package auth

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SecurityHeadersMiddleware 添加安全响应头
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

type loginAttempt struct {
	count     int
	firstTime time.Time
	lockedAt  time.Time
}

// LoginRateLimiter 速率限制器（用于登录与 OTP 校验防暴力破解）
type LoginRateLimiter struct {
	mu          sync.RWMutex
	attempts    map[string]*loginAttempt
	maxAttempts int
	window      time.Duration
	lockout     time.Duration
}

var (
	loginLimiter = newRateLimiter(5, 5*time.Minute, 15*time.Minute)
	otpLimiter   = newRateLimiter(8, 5*time.Minute, 15*time.Minute)
)

func newRateLimiter(maxAttempts int, window, lockout time.Duration) *LoginRateLimiter {
	rl := &LoginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
	}
	go rl.cleanup()
	return rl
}

func GetLoginRateLimiter() *LoginRateLimiter {
	return loginLimiter
}

func GetOTPRateLimiter() *LoginRateLimiter {
	return otpLimiter
}

func (l *LoginRateLimiter) IsBlocked(key string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	attempt, exists := l.attempts[key]
	if !exists {
		return false
	}

	if !attempt.lockedAt.IsZero() {
		if time.Since(attempt.lockedAt) < l.lockout {
			return true
		}
	}

	return false
}

func (l *LoginRateLimiter) RecordFailure(key string) (blocked bool, remaining int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	attempt, exists := l.attempts[key]
	if !exists {
		l.attempts[key] = &loginAttempt{count: 1, firstTime: now}
		return false, l.maxAttempts - 1
	}

	if !attempt.lockedAt.IsZero() && now.Sub(attempt.lockedAt) >= l.lockout {
		attempt.count = 1
		attempt.firstTime = now
		attempt.lockedAt = time.Time{}
		return false, l.maxAttempts - 1
	}

	if now.Sub(attempt.firstTime) > l.window {
		attempt.count = 1
		attempt.firstTime = now
		attempt.lockedAt = time.Time{}
		return false, l.maxAttempts - 1
	}

	attempt.count++
	if attempt.count >= l.maxAttempts {
		attempt.lockedAt = now
		return true, 0
	}

	return false, l.maxAttempts - attempt.count
}

func (l *LoginRateLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (l *LoginRateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for k, attempt := range l.attempts {
			if now.Sub(attempt.firstTime) > l.window+l.lockout {
				delete(l.attempts, k)
			}
		}
		l.mu.Unlock()
	}
}

// CheckWebSocketOrigin 验证 WebSocket 请求的 Origin 头，防御 CSWSH 攻击
func CheckWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	host := r.Host
	if host == "" {
		host = r.Header.Get("Host")
	}
	if host == "" {
		return false
	}

	originHost := extractHostname(extractHost(origin))
	requestHost := extractHostname(host)
	if originHost == "" || requestHost == "" {
		return false
	}

	return strings.EqualFold(originHost, requestHost)
}

func extractHost(origin string) string {
	origin = strings.TrimSpace(origin)
	if idx := strings.Index(origin, "://"); idx >= 0 {
		origin = origin[idx+3:]
	}
	if idx := strings.Index(origin, "/"); idx >= 0 {
		origin = origin[:idx]
	}
	return origin
}

func extractHostname(hostPort string) string {
	if strings.HasPrefix(hostPort, "[") {
		if idx := strings.Index(hostPort, "]"); idx >= 0 {
			return hostPort[1:idx]
		}
	}
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		return hostPort[:idx]
	}
	return hostPort
}

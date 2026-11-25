package auth

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
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
	loginLimiter   = newRateLimiter(5, 5*time.Minute, 15*time.Minute)
	loginIPLimiter = newRateLimiter(60, 5*time.Minute, 15*time.Minute)
	otpLimiter     = newRateLimiter(8, 5*time.Minute, 15*time.Minute)
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

func GetLoginIPRateLimiter() *LoginRateLimiter {
	return loginIPLimiter
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

	if !attempt.lockedAt.IsZero() {
		if now.Sub(attempt.lockedAt) < l.lockout {
			return true, 0
		}
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
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil || u.User != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	originHost := strings.TrimSpace(u.Hostname())
	if originHost == "" {
		return false
	}

	reqHost := strings.TrimSpace(r.Host)
	if reqHost == "" {
		reqHost = strings.TrimSpace(r.Header.Get("Host"))
	}
	if reqHost == "" {
		return false
	}

	reqHostName, reqPort := splitRequestHostPort(reqHost)
	if reqHostName == "" {
		return false
	}

	if strings.EqualFold(originHost, reqHostName) {
		if !IsAuthEnabled() {
			return normalizeSchemePort(u.Port(), scheme) == normalizeSchemePort(reqPort, scheme)
		}
		return true
	}

	if allowedDomains := strings.TrimSpace(conf.EnvCfg.TWOFA_ALLOWED_DOMAINS); allowedDomains != "" {
		for _, allowed := range strings.Split(allowedDomains, ",") {
			candidate := strings.TrimSpace(allowed)
			if candidate == "" {
				continue
			}
			if strings.EqualFold(candidate, originHost) || strings.EqualFold(candidate, u.Host) {
				return true
			}
		}
	}

	return false
}

func splitRequestHostPort(hostPort string) (host, port string) {
	hostPort = strings.TrimSpace(hostPort)
	if h, p, err := net.SplitHostPort(hostPort); err == nil {
		return h, p
	}
	if strings.HasPrefix(hostPort, "[") && strings.HasSuffix(hostPort, "]") {
		return hostPort[1 : len(hostPort)-1], ""
	}
	return hostPort, ""
}

func normalizeSchemePort(port, scheme string) string {
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return ""
	}
	return port
}

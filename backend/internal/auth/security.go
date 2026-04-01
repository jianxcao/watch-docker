package auth

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SecurityHeadersMiddleware 添加安全响应头，防御 XSS、点击劫持等攻击
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

// loginAttempt 记录一次登录尝试
type loginAttempt struct {
	count     int
	firstTime time.Time
	lockedAt  time.Time
}

// LoginRateLimiter 登录速率限制器
type LoginRateLimiter struct {
	mu       sync.RWMutex
	attempts map[string]*loginAttempt
	maxAttempts int
	window      time.Duration
	lockout     time.Duration
}

var loginLimiter = &LoginRateLimiter{
	attempts:    make(map[string]*loginAttempt),
	maxAttempts: 5,
	window:      5 * time.Minute,
	lockout:     15 * time.Minute,
}

func init() {
	go loginLimiter.cleanup()
}

// GetLoginRateLimiter 获取登录速率限制器
func GetLoginRateLimiter() *LoginRateLimiter {
	return loginLimiter
}

// IsBlocked 检查 IP 是否被暂时锁定
func (l *LoginRateLimiter) IsBlocked(ip string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	attempt, exists := l.attempts[ip]
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

// RecordFailure 记录一次失败的登录尝试，返回是否应锁定
func (l *LoginRateLimiter) RecordFailure(ip string) (blocked bool, remaining int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	attempt, exists := l.attempts[ip]
	if !exists {
		l.attempts[ip] = &loginAttempt{count: 1, firstTime: now}
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

// RecordSuccess 登录成功后清除记录
func (l *LoginRateLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// cleanup 定时清理过期记录
func (l *LoginRateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for ip, attempt := range l.attempts {
			if now.Sub(attempt.firstTime) > l.window+l.lockout {
				delete(l.attempts, ip)
			}
		}
		l.mu.Unlock()
	}
}

// CheckWebSocketOrigin 验证 WebSocket 请求的 Origin 头
// 防止 Cross-Site WebSocket Hijacking (CSWSH) 攻击
// 只比较主机名（不含端口），以兼容反向代理场景（如 Vite dev proxy）
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

// extractHost 从 Origin URL 中提取主机部分（含端口）
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

// extractHostname 从 host:port 中提取纯主机名（去掉端口）
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

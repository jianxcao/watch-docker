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

// SanitizeHostHeader 规范化 Host / X-Forwarded-Host 值，去除协议前缀与路径后缀。
func SanitizeHostHeader(raw string) string {
	host := strings.TrimSpace(raw)
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "http://") {
		host = host[len("http://"):]
	} else if strings.HasPrefix(lower, "https://") {
		host = host[len("https://"):]
	}
	if idx := strings.IndexByte(host, '/'); idx >= 0 {
		host = host[:idx]
	}
	return strings.TrimSpace(host)
}

func parseRemoteAddrIP(remoteAddr string) net.IP {
	raw := strings.TrimSpace(remoteAddr)
	if raw == "" {
		return nil
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	} else {
		host = strings.Trim(raw, "[]")
	}
	if idx := strings.IndexByte(host, '%'); idx >= 0 {
		host = host[:idx]
	}
	return net.ParseIP(host)
}

// IsTrustedProxyRequest 判断请求直接来源 IP 是否命中 TRUSTED_PROXIES 配置的 IP/CIDR。
func IsTrustedProxyRequest(r *http.Request) bool {
	if r == nil || strings.TrimSpace(conf.EnvCfg.TRUSTED_PROXIES) == "" {
		return false
	}
	peerIP := parseRemoteAddrIP(r.RemoteAddr)
	if peerIP == nil {
		return false
	}
	for _, raw := range strings.Split(conf.EnvCfg.TRUSTED_PROXIES, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, ipNet, err := net.ParseCIDR(entry); err == nil && ipNet.Contains(peerIP) {
				return true
			}
			continue
		}
		if trustedIP := net.ParseIP(strings.Trim(entry, "[]")); trustedIP != nil && trustedIP.Equal(peerIP) {
			return true
		}
	}
	return false
}

// EffectiveRequestHost 解析请求有效 Host（仅对来自 TRUSTED_PROXIES 的请求采信 X-Forwarded-Host）。
func EffectiveRequestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := SanitizeHostHeader(r.Host)
	if host == "" {
		host = SanitizeHostHeader(r.Header.Get("Host"))
	}
	if IsTrustedProxyRequest(r) {
		if xfh := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xfh != "" {
			first := SanitizeHostHeader(strings.Split(xfh, ",")[0])
			if h, _ := splitRequestHostPort(first); h != "" {
				host = first
			}
		}
	}
	return host
}

// EffectiveRequestScheme 解析请求有效协议（仅对来自 TRUSTED_PROXIES 的请求采信 X-Forwarded-Proto）。
func EffectiveRequestScheme(r *http.Request) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	if IsTrustedProxyRequest(r) {
		xfp := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
		if strings.EqualFold(xfp, "https") {
			return "https"
		}
		if strings.EqualFold(xfp, "http") {
			return "http"
		}
	}
	return "http"
}

// ParseTrustedProxies 解析逗号分隔的 TRUSTED_PROXIES 配置，分别返回合法 IP/CIDR（兼容带方括号 IPv6）与被忽略的非法项。
func ParseTrustedProxies(raw string) (valid []string, invalid []string) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	for _, item := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(item)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, ipNet, err := net.ParseCIDR(entry); err == nil && ipNet != nil {
				valid = append(valid, ipNet.String())
			} else {
				invalid = append(invalid, entry)
			}
			continue
		}
		if ip := net.ParseIP(strings.Trim(entry, "[]")); ip != nil {
			valid = append(valid, ip.String())
		} else {
			invalid = append(invalid, entry)
		}
	}
	return valid, invalid
}

// MatchAllowedDomain 按 host、host:port 或 scheme://host[:port] 规则匹配 TWOFA_ALLOWED_DOMAINS 白名单。
func MatchAllowedDomain(allowedDomainsStr, candidateHostWithPort, candidateHostOnly, scheme string) bool {
	if strings.TrimSpace(allowedDomainsStr) == "" {
		return true
	}
	if candidateHostOnly == "" {
		return false
	}
	_, candPort := splitRequestHostPort(candidateHostWithPort)
	candEffectivePort := effectiveSchemePort(candPort, scheme)

	for _, raw := range strings.Split(allowedDomainsStr, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "://") {
			if eu, err := url.Parse(entry); err == nil && eu != nil && eu.User == nil &&
				eu.Hostname() != "" && (strings.EqualFold(eu.Scheme, "http") || strings.EqualFold(eu.Scheme, "https")) {
				if scheme == "" || !strings.EqualFold(eu.Scheme, scheme) {
					continue
				}
				entryEffectivePort := effectiveSchemePort(eu.Port(), eu.Scheme)
				if strings.EqualFold(eu.Hostname(), candidateHostOnly) && candEffectivePort != "" && entryEffectivePort == candEffectivePort {
					return true
				}
			}
			continue
		}
		if entryHost, entryPort, err := net.SplitHostPort(entry); err == nil {
			if entryHost == "" || entryPort == "" {
				continue
			}
			if (entryPort == "443" && scheme != "" && !strings.EqualFold(scheme, "https")) ||
				(entryPort == "80" && scheme != "" && !strings.EqualFold(scheme, "http")) {
				continue
			}
			if strings.EqualFold(entryHost, candidateHostOnly) && candEffectivePort != "" && entryPort == candEffectivePort {
				return true
			}
			continue
		}
		entryHost, _ := splitRequestHostPort(entry)
		if entryHost != "" && !strings.ContainsAny(entryHost, "/ \t@") && strings.EqualFold(entryHost, candidateHostOnly) {
			return true
		}
	}
	return false
}

// MatchAllowedRPID 按 hostname 维度校验 RPID 是否符合 TWOFA_ALLOWED_DOMAINS（支持 host、host:port 与 scheme://host[:port]）。
func MatchAllowedRPID(allowedDomainsStr, rpid string) bool {
	if strings.TrimSpace(allowedDomainsStr) == "" {
		return true
	}
	rpid = strings.TrimSpace(rpid)
	if rpid == "" {
		return false
	}
	for _, raw := range strings.Split(allowedDomainsStr, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "://") {
			if eu, err := url.Parse(entry); err == nil && eu != nil && eu.User == nil &&
				(strings.EqualFold(eu.Scheme, "http") || strings.EqualFold(eu.Scheme, "https")) &&
				eu.Hostname() != "" && strings.EqualFold(eu.Hostname(), rpid) {
				return true
			}
			continue
		}
		if entryHost, entryPort, err := net.SplitHostPort(entry); err == nil {
			if entryHost != "" && entryPort != "" && strings.EqualFold(entryHost, rpid) {
				return true
			}
			continue
		}
		entryHost, _ := splitRequestHostPort(entry)
		if entryHost != "" && !strings.ContainsAny(entryHost, "/ \t@") && strings.EqualFold(entryHost, rpid) {
			return true
		}
	}
	return false
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

	reqHost := EffectiveRequestHost(r)
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
		if MatchAllowedDomain(allowedDomains, u.Host, originHost, scheme) {
			return true
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

func effectiveSchemePort(port, scheme string) string {
	if port != "" {
		return port
	}
	switch strings.ToLower(scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/conf"
)

func TestCheckWebSocketOrigin(t *testing.T) {
	origEnv := conf.EnvCfg
	t.Cleanup(func() {
		conf.EnvCfg = origEnv
	})

	makeReq := func(host, origin string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
		req.Host = host
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		return req
	}

	// 无登录认证模式下要求同 host 且同端口
	conf.EnvCfg.USER_NAME = ""
	conf.EnvCfg.USER_PASSWORD = ""
	conf.EnvCfg.TWOFA_ALLOWED_DOMAINS = ""

	if !CheckWebSocketOrigin(makeReq("localhost:8080", "")) {
		t.Fatal("empty Origin should be allowed")
	}
	if !CheckWebSocketOrigin(makeReq("localhost:8080", "http://localhost:8080")) {
		t.Fatal("exact same host and port should be allowed")
	}
	if CheckWebSocketOrigin(makeReq("localhost:8080", "http://localhost:3000")) {
		t.Fatal("different port should be rejected when auth is disabled")
	}
	if CheckWebSocketOrigin(makeReq("localhost:8080", "http://user@localhost:8080")) {
		t.Fatal("userinfo in Origin must be rejected")
	}
	if CheckWebSocketOrigin(makeReq("localhost:8080", "javascript://localhost:8080")) {
		t.Fatal("non-http/https scheme must be rejected")
	}
	if !CheckWebSocketOrigin(makeReq("[::1]:8080", "http://[::1]:8080")) {
		t.Fatal("IPv6 matching host:port should be allowed")
	}
	if !CheckWebSocketOrigin(makeReq("example.com", "https://example.com:443")) {
		t.Fatal("normalized HTTPS default port 443 should match")
	}

	// 开启认证时允许同 hostname 不同反代端口以及 TWOFA_ALLOWED_DOMAINS 白名单
	conf.EnvCfg.USER_NAME = "admin"
	conf.EnvCfg.USER_PASSWORD = "secret"
	conf.EnvCfg.TWOFA_ALLOWED_DOMAINS = "trusted.example.com"

	if !CheckWebSocketOrigin(makeReq("localhost:8080", "http://localhost:5173")) {
		t.Fatal("same hostname different port should be allowed when auth is enabled")
	}
	if !CheckWebSocketOrigin(makeReq("localhost:8080", "https://trusted.example.com")) {
		t.Fatal("TWOFA_ALLOWED_DOMAINS should be allowed")
	}
	if CheckWebSocketOrigin(makeReq("localhost:8080", "https://evil.example.com")) {
		t.Fatal("untrusted origin should be rejected")
	}

	// 受信任代理转发 X-Forwarded-Host（且 TWOFA_ALLOWED_DOMAINS 为空时）应放行，非受信任来源必须拒绝
	conf.EnvCfg.TWOFA_ALLOWED_DOMAINS = ""
	conf.EnvCfg.TRUSTED_PROXIES = "127.0.0.1"
	trustedWSReq := makeReq("127.0.0.1:8080", "https://docker.example.com:8443")
	trustedWSReq.RemoteAddr = "127.0.0.1:50000"
	trustedWSReq.Header.Set("X-Forwarded-Host", "docker.example.com:8443")
	if !CheckWebSocketOrigin(trustedWSReq) {
		t.Fatal("trusted proxy X-Forwarded-Host should be accepted by CheckWebSocketOrigin")
	}

	untrustedWSReq := makeReq("127.0.0.1:8080", "https://docker.example.com:8443")
	untrustedWSReq.RemoteAddr = "198.51.100.42:54321"
	untrustedWSReq.Header.Set("X-Forwarded-Host", "docker.example.com:8443")
	if CheckWebSocketOrigin(untrustedWSReq) {
		t.Fatal("untrusted peer X-Forwarded-Host must be rejected")
	}

	// 带方括号 IPv6 白名单应匹配同主机加端口的 Origin
	conf.EnvCfg.TRUSTED_PROXIES = ""
	conf.EnvCfg.TWOFA_ALLOWED_DOMAINS = "[::1]"
	if !CheckWebSocketOrigin(makeReq("127.0.0.1:8080", "http://[::1]:8080")) {
		t.Fatal("bracketed IPv6 in TWOFA_ALLOWED_DOMAINS should allow http://[::1]:8080")
	}

	// MatchAllowedDomain 默认端口 (80/443) 与完整 scheme://host 端口约束
	if !MatchAllowedDomain("https://docker.example.com:443", "docker.example.com", "docker.example.com", "https") {
		t.Fatal("expected https://docker.example.com:443 to match implicit HTTPS default port")
	}
	if !MatchAllowedDomain("https://docker.example.com:443", "docker.example.com:443", "docker.example.com", "https") {
		t.Fatal("expected https://docker.example.com:443 to match explicit :443")
	}
	if !MatchAllowedDomain("docker.example.com:443", "docker.example.com", "docker.example.com", "https") {
		t.Fatal("expected docker.example.com:443 to match implicit https")
	}
	if MatchAllowedDomain("docker.example.com:443", "docker.example.com:443", "docker.example.com", "http") {
		t.Fatal("expected docker.example.com:443 to reject cleartext http scheme")
	}
	if MatchAllowedDomain("https://docker.example.com", "docker.example.com:9443", "docker.example.com", "https") {
		t.Fatal("expected https://docker.example.com to reject non-default port 9443")
	}

	// ParseTrustedProxies 清洗带方括号 IPv6 并剔除非法项
	validProxies, invalidProxies := ParseTrustedProxies("[::1],invalid-entry,127.0.0.1,10.0.0.0/8")
	if len(validProxies) != 3 || validProxies[0] != "::1" || validProxies[1] != "127.0.0.1" || validProxies[2] != "10.0.0.0/8" || len(invalidProxies) != 1 || invalidProxies[0] != "invalid-entry" {
		t.Fatalf("unexpected ParseTrustedProxies output: valid=%#v invalid=%#v", validProxies, invalidProxies)
	}
}

func TestLoginRateLimiterKeepsActiveLockoutAcrossWindowBoundary(t *testing.T) {
	limiter := &LoginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 2,
		window:      50 * time.Millisecond,
		lockout:     300 * time.Millisecond,
	}
	key := "127.0.0.1|alice"

	if blocked, _ := limiter.RecordFailure(key); blocked {
		t.Fatal("first failure should not block")
	}
	if blocked, _ := limiter.RecordFailure(key); !blocked {
		t.Fatal("second failure should trigger lockout")
	}

	// 推进超过 window (50ms) 但仍在 lockout (300ms) 期内
	limiter.mu.Lock()
	limiter.attempts[key].firstTime = time.Now().Add(-100 * time.Millisecond)
	limiter.attempts[key].lockedAt = time.Now().Add(-100 * time.Millisecond)
	limiter.mu.Unlock()

	if blocked, remaining := limiter.RecordFailure(key); !blocked || remaining != 0 {
		t.Fatalf("expected active lockout preserved across window boundary, got blocked=%v remaining=%d", blocked, remaining)
	}
}

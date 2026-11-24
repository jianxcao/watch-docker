package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
}

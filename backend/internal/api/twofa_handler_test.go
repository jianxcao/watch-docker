package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
)

func setupTwoFATestServer(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	origEnv := conf.EnvCfg
	origCfg := config.Get()
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	conf.EnvCfg.CONFIG_PATH = dir
	conf.EnvCfg.CONFIG_FILE = "config.yaml"
	conf.EnvCfg.USER_NAME = "tester"
	conf.EnvCfg.USER_PASSWORD = "secret-password"
	conf.EnvCfg.IS_SECONDARY_VERIFICATION = true

	if _, err := config.Load(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("load temp config: %v", err)
	}

	t.Cleanup(func() {
		conf.EnvCfg = origEnv
		if origCfg != nil {
			config.SetGlobal(origCfg)
		}
	})

	s := &Server{logger: zap.NewNop()}
	r := gin.New()
	twofaGroup := r.Group("/api/v1/2fa")
	twofaGroup.Use(auth.TempTokenMiddleware())
	{
		twofaGroup.POST("/setup/otp/init", s.handleOTPSetupInit())
		twofaGroup.POST("/setup/otp/verify", s.handleOTPSetupVerify())
		twofaGroup.POST("/setup/webauthn/begin", s.handleWebAuthnRegisterBegin())
	}
	return r, "tester"
}

func doTwoFARequest(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "localhost:8080"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestTwoFASetupAndTempTokenOverwriteProtection(t *testing.T) {
	r, username := setupTwoFATestServer(t)

	tempToken, err := auth.GenerateTempToken(username)
	if err != nil {
		t.Fatalf("generate temp token: %v", err)
	}

	// 1. Verify OTP setup using an uninitialized secret is rejected
	badVerify := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/verify", tempToken, map[string]string{
		"secret": "JBSWY3DPEHPK3PXP",
		"code":   "000000",
	})
	if badVerify.Code != http.StatusOK {
		t.Fatalf("expected 200 error response, got %d", badVerify.Code)
	}
	var badRes BaseRes
	if err := json.Unmarshal(badVerify.Body.Bytes(), &badRes); err != nil {
		t.Fatalf("decode bad verify: %v", err)
	}
	if badRes.Code != CodeBadRequest {
		t.Fatalf("expected CodeBadRequest for uninitialized secret, got %d", badRes.Code)
	}

	// 2. First-time OTP setup via tempToken succeeds
	initRec := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/init", tempToken, nil)
	if initRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on initial setup/otp/init, got %d (%s)", initRec.Code, initRec.Body.String())
	}
	var initResp struct {
		Code int `json:"code"`
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(initRec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("decode init resp: %v", err)
	}
	if initResp.Code != SUCCESS || initResp.Data.Secret == "" {
		t.Fatalf("expected otp init success with secret, got %+v", initResp)
	}

	otpCode, err := totp.GenerateCode(initResp.Data.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}

	verifyRec := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/verify", tempToken, map[string]string{
		"secret": initResp.Data.Secret,
		"code":   otpCode,
	})
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on verify, got %d", verifyRec.Code)
	}
	var verifyResp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("decode verify resp: %v", err)
	}
	if verifyResp.Code != SUCCESS || verifyResp.Data.Token == "" {
		t.Fatalf("expected full access token after setup verify, got %+v", verifyResp)
	}

	// 3. Once 2FA is configured, a second setup attempt using a tempToken MUST be blocked
	secondTemp, err := auth.GenerateTempToken(username)
	if err != nil {
		t.Fatalf("generate second temp token: %v", err)
	}
	blockedInit := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/init", secondTemp, nil)
	if blockedInit.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when re-initializing OTP with tempToken after setup, got %d (%s)", blockedInit.Code, blockedInit.Body.String())
	}

	blockedVerify := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/verify", secondTemp, map[string]string{
		"secret": initResp.Data.Secret,
		"code":   otpCode,
	})
	if blockedVerify.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when verifying OTP setup with tempToken after setup, got %d", blockedVerify.Code)
	}

	blockedWebAuthn := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/webauthn/begin", secondTemp, nil)
	if blockedWebAuthn.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when beginning WebAuthn setup with tempToken after setup, got %d", blockedWebAuthn.Code)
	}

	// 4. Full-access JWT is allowed to reconfigure 2FA
	fullInit := doTwoFARequest(r, http.MethodPost, "/api/v1/2fa/setup/otp/init", verifyResp.Data.Token, nil)
	if fullInit.Code != http.StatusOK {
		t.Fatalf("expected 200 when reconfiguring OTP with full token, got %d (%s)", fullInit.Code, fullInit.Body.String())
	}
}

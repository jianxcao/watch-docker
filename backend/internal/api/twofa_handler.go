package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/twofa"
	"go.uber.org/zap"
)

type webauthnSessionEntry struct {
	Username  string
	Flow      string // "register" | "login"
	RPID      string
	Session   webauthn.SessionData
	ExpiresAt time.Time
}

type otpSetupEntry struct {
	Username  string
	Secret    string
	ExpiresAt time.Time
}

var (
	twofaSessionsMu  sync.Mutex
	webauthnSessions = make(map[string]webauthnSessionEntry)
	otpSetupSessions = make(map[string]otpSetupEntry)
)

func randomSessionID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func storeWebAuthnSession(username, flow, rpid string, session *webauthn.SessionData) string {
	id := randomSessionID()
	now := time.Now()
	twofaSessionsMu.Lock()
	for k, v := range webauthnSessions {
		if now.After(v.ExpiresAt) {
			delete(webauthnSessions, k)
		}
	}
	webauthnSessions[id] = webauthnSessionEntry{
		Username:  username,
		Flow:      flow,
		RPID:      rpid,
		Session:   *session,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	twofaSessionsMu.Unlock()
	return id
}

func consumeWebAuthnSession(id, username, flow, rpid string) (webauthn.SessionData, bool) {
	now := time.Now()
	twofaSessionsMu.Lock()
	defer twofaSessionsMu.Unlock()

	entry, ok := webauthnSessions[id]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(webauthnSessions, id)
	if now.After(entry.ExpiresAt) {
		return webauthn.SessionData{}, false
	}
	if entry.Username != username || entry.Flow != flow || entry.RPID != rpid {
		return webauthn.SessionData{}, false
	}
	return entry.Session, true
}

func storeOTPSetupSecret(username, secret string) {
	now := time.Now()
	twofaSessionsMu.Lock()
	for k, v := range otpSetupSessions {
		if now.After(v.ExpiresAt) {
			delete(otpSetupSessions, k)
		}
	}
	otpSetupSessions[username] = otpSetupEntry{
		Username:  username,
		Secret:    secret,
		ExpiresAt: now.Add(10 * time.Minute),
	}
	twofaSessionsMu.Unlock()
}

func consumeOTPSetupSecret(username, secret string) bool {
	now := time.Now()
	twofaSessionsMu.Lock()
	defer twofaSessionsMu.Unlock()

	entry, ok := otpSetupSessions[username]
	if !ok {
		// 若直接传入未初始化过的密钥，仅允许在服务端已有匹配会话时通过
		return false
	}
	if now.After(entry.ExpiresAt) || entry.Secret != secret {
		return false
	}
	delete(otpSetupSessions, username)
	return true
}

// extractRPIDAndOrigin 从请求提取 RPID 和 Origin；仅当未设置 Origin 时根据 Host 推导，不盲信外部伪造头。
func extractRPIDAndOrigin(c *gin.Context) (rpid string, origin string) {
	origin = strings.TrimSpace(c.GetHeader("Origin"))
	host := strings.TrimSpace(c.Request.Host)

	if origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Hostname() != "" {
			rpid = u.Hostname()
			if host == "" {
				host = u.Host
			}
			// 若请求 Host 存在，要求 Origin 主机与请求 Host 主机一致
			reqHostOnly := host
			if h, _, err := net.SplitHostPort(host); err == nil {
				reqHostOnly = h
			}
			if reqHostOnly != "" && !strings.EqualFold(reqHostOnly, rpid) {
				rpid = reqHostOnly
			}
			return rpid, origin
		}
	}

	if host == "" {
		host = strings.TrimSpace(c.GetHeader("Host"))
	}
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}

	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	origin = scheme + "://" + host

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	rpid = host
	return rpid, origin
}

func isAllowedDomain(domain string) bool {
	allowedDomainsStr := conf.EnvCfg.TWOFA_ALLOWED_DOMAINS
	if allowedDomainsStr == "" {
		return true
	}

	allowedDomains := strings.Split(allowedDomainsStr, ",")
	for _, allowed := range allowedDomains {
		if strings.EqualFold(strings.TrimSpace(allowed), domain) {
			return true
		}
	}
	return false
}

func upgradeTokenFromContext(c *gin.Context) (string, error) {
	if raw, ok := c.Get("rawToken"); ok {
		if tokenStr, ok := raw.(string); ok && tokenStr != "" {
			return auth.UpgradeTempToken(tokenStr)
		}
	}
	tokenStr, err := auth.ExtractTokenFromRequest(c)
	if err != nil {
		return "", err
	}
	return auth.UpgradeTempToken(tokenStr)
}

// isTempTokenBlockedForAlreadySetup 阻止已启用 2FA 的账户仅凭临时令牌重新绑定或覆盖密钥（防止 2FA 绕过）。
func isTempTokenBlockedForAlreadySetup(c *gin.Context, username string) (bool, error) {
	isTemp, _ := c.Get("isTempToken")
	isTempBool, _ := isTemp.(bool)
	if !isTempBool {
		return false, nil
	}
	return twofa.IsUserSetup(username)
}

func (s *Server) handleTwoFAStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		rpid, _ := extractRPIDAndOrigin(c)
		isSetup, err := twofa.IsUserSetupForMethod(username.(string), userConfig.Method, rpid)
		if err != nil {
			s.logger.Error("check user setup status failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查设置状态失败"))
			return
		}

		envCfg := conf.EnvCfg
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"enabled": envCfg.IS_SECONDARY_VERIFICATION,
			"isSetup": isSetup,
			"method":  userConfig.Method,
		}))
	}
}

func (s *Server) handleOTPSetupInit() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		if blocked, err := isTempTokenBlockedForAlreadySetup(c, usernameStr); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查 2FA 状态失败"))
			return
		} else if blocked {
			c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "已启用二次验证，禁止使用临时令牌重置"))
			return
		}

		secret, err := twofa.GenerateOTPSecret()
		if err != nil {
			s.logger.Error("generate otp secret failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成密钥失败"))
			return
		}

		qrCodeURL, err := twofa.GenerateQRCodeURL(secret, usernameStr, "Watch Docker")
		if err != nil {
			s.logger.Error("generate qr code url failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成二维码失败"))
			return
		}

		storeOTPSetupSecret(usernameStr, secret)

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"secret":    secret,
			"qrCodeURL": qrCodeURL,
		}))
	}
}

func (s *Server) handleOTPSetupVerify() gin.HandlerFunc {
	type Request struct {
		Code   string `json:"code" binding:"required"`
		Secret string `json:"secret" binding:"required"`
	}

	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		if blocked, err := isTempTokenBlockedForAlreadySetup(c, usernameStr); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查 2FA 状态失败"))
			return
		} else if blocked {
			s.logger.Warn("blocked attempt to overwrite existing 2FA using temp token", zap.String("username", usernameStr))
			c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "已启用二次验证，请先验证原有 2FA 后再修改"))
			return
		}

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		if !consumeOTPSetupSecret(usernameStr, req.Secret) {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "OTP 初始化会话无效或已过期，请重新生成二维码"))
			return
		}

		if !twofa.ValidateOTPCode(req.Secret, req.Code) {
			// 允许重试时保留会话直到过期或成功
			storeOTPSetupSecret(usernameStr, req.Secret)
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "验证码错误"))
			return
		}

		userConfig := &twofa.UserTwoFAConfig{
			Method:    twofa.MethodOTP,
			OTPSecret: req.Secret,
			IsSetup:   true,
		}

		if err := twofa.SaveUserConfig(usernameStr, userConfig); err != nil {
			s.logger.Error("save user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "保存配置失败"))
			return
		}

		isTemp, _ := c.Get("isTempToken")
		var fullToken string
		if isTempBool, _ := isTemp.(bool); isTempBool {
			var err error
			fullToken, err = upgradeTokenFromContext(c)
			if err != nil {
				s.logger.Error("upgrade temp token failed", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
				return
			}
		} else {
			var err error
			fullToken, err = auth.GenerateToken(usernameStr)
			if err != nil {
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成token失败"))
				return
			}
		}

		s.logger.Info("user setup otp successfully", zap.String("username", usernameStr))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

func (s *Server) handleVerifyOTP() gin.HandlerFunc {
	type Request struct {
		Code string `json:"code" binding:"required"`
	}

	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		rateKey := c.ClientIP() + "|" + usernameStr
		limiter := auth.GetOTPRateLimiter()
		if limiter.IsBlocked(rateKey) {
			s.logger.Warn("otp verification blocked due to rate limit", zap.String("username", usernameStr), zap.String("ip", c.ClientIP()))
			c.JSON(http.StatusTooManyRequests, NewErrorResCode(CodeBadRequest, "验证码错误次数过多，请15分钟后再试"))
			return
		}

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		userConfig, err := twofa.GetUserConfig(usernameStr)
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		if userConfig.Method != twofa.MethodOTP {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "未设置OTP"))
			return
		}

		if userConfig.OTPSecret == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "OTP密钥未设置"))
			return
		}

		if !twofa.ValidateOTPCode(userConfig.OTPSecret, req.Code) {
			blocked, _ := limiter.RecordFailure(rateKey)
			if blocked {
				c.JSON(http.StatusTooManyRequests, NewErrorResCode(CodeBadRequest, "验证码错误次数过多，请15分钟后再试"))
			} else {
				c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "验证码错误"))
			}
			return
		}
		limiter.RecordSuccess(rateKey)

		fullToken, err := upgradeTokenFromContext(c)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user verify otp successfully", zap.String("username", usernameStr))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

func (s *Server) handleWebAuthnRegisterBegin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		if blocked, err := isTempTokenBlockedForAlreadySetup(c, usernameStr); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查 2FA 状态失败"))
			return
		} else if blocked {
			c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "已启用二次验证，禁止使用临时令牌添加凭据"))
			return
		}

		rpid, origin := extractRPIDAndOrigin(c)
		if !isAllowedDomain(rpid) {
			s.logger.Warn("domain not in whitelist", zap.String("domain", rpid))
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "域名不在白名单中"))
			return
		}

		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", rpid, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		credentials, err := twofa.GetUserCredentialsForRPID(usernameStr, rpid)
		if err != nil {
			s.logger.Error("get user credentials for rpid failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取凭据失败"))
			return
		}

		options, session, err := webAuthnService.BeginRegistration(usernameStr, credentials)
		if err != nil {
			s.logger.Error("begin webauthn registration failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "开始注册失败"))
			return
		}

		sessionID := storeWebAuthnSession(usernameStr, "register", rpid, session)

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"options":     options,
			"sessionData": sessionID,
		}))
	}
}

func (s *Server) handleWebAuthnRegisterFinish() gin.HandlerFunc {
	type Request struct {
		SessionData string          `json:"sessionData" binding:"required"`
		Response    json.RawMessage `json:"response" binding:"required"`
	}

	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		if blocked, err := isTempTokenBlockedForAlreadySetup(c, usernameStr); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查 2FA 状态失败"))
			return
		} else if blocked {
			c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "已启用二次验证，禁止使用临时令牌添加凭据"))
			return
		}

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("bind json failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		rpid, origin := extractRPIDAndOrigin(c)
		if !isAllowedDomain(rpid) {
			s.logger.Warn("domain not in whitelist", zap.String("domain", rpid))
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "域名不在白名单中"))
			return
		}

		sessionData, ok := consumeWebAuthnSession(req.SessionData, usernameStr, "register", rpid)
		if !ok {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "注册会话无效或已过期"))
			return
		}

		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", rpid, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(req.Response))
		if err != nil {
			s.logger.Error("parse credential creation response failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "响应数据错误"))
			return
		}

		credentials, err := twofa.GetUserCredentialsForRPID(usernameStr, rpid)
		if err != nil {
			s.logger.Error("get user credentials for rpid failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取凭据失败"))
			return
		}

		credential, err := webAuthnService.FinishRegistration(usernameStr, credentials, sessionData, parsedResponse)
		if err != nil {
			s.logger.Error("finish webauthn registration failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "完成注册失败"))
			return
		}

		userConfig, err := twofa.GetUserConfig(usernameStr)
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		userConfig.Method = twofa.MethodWebAuthn
		userConfig.WebAuthnCredentials = append(userConfig.WebAuthnCredentials, twofa.WebAuthnCredentialWithRPID{
			Credential: *credential,
			RPID:       rpid,
		})
		userConfig.IsSetup = true

		if err := twofa.SaveUserConfig(usernameStr, userConfig); err != nil {
			s.logger.Error("save user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "保存配置失败"))
			return
		}

		isTemp, _ := c.Get("isTempToken")
		var fullToken string
		if isTempBool, _ := isTemp.(bool); isTempBool {
			fullToken, err = upgradeTokenFromContext(c)
			if err != nil {
				s.logger.Error("upgrade temp token failed", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
				return
			}
		} else {
			fullToken, err = auth.GenerateToken(usernameStr)
			if err != nil {
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成token失败"))
				return
			}
		}

		s.logger.Info("user setup webauthn successfully", zap.String("username", usernameStr))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

func (s *Server) handleWebAuthnLoginBegin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		rpid, origin := extractRPIDAndOrigin(c)
		if !isAllowedDomain(rpid) {
			s.logger.Warn("domain not in whitelist", zap.String("domain", rpid))
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "域名不在白名单中"))
			return
		}

		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", rpid, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		userConfig, err := twofa.GetUserConfig(usernameStr)
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		if userConfig.Method != twofa.MethodWebAuthn {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "未设置WebAuthn"))
			return
		}

		credentials, err := twofa.GetUserCredentialsForRPID(usernameStr, rpid)
		if err != nil {
			s.logger.Error("get user credentials for rpid failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取凭据失败"))
			return
		}

		if len(credentials) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "当前域名未注册WebAuthn凭据"))
			return
		}

		options, session, err := webAuthnService.BeginLogin(usernameStr, credentials)
		if err != nil {
			s.logger.Error("begin webauthn login failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "开始验证失败"))
			return
		}

		sessionID := storeWebAuthnSession(usernameStr, "login", rpid, session)

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"options":     options,
			"sessionData": sessionID,
		}))
	}
}

func (s *Server) handleWebAuthnLoginFinish() gin.HandlerFunc {
	type Request struct {
		SessionData string          `json:"sessionData" binding:"required"`
		Response    json.RawMessage `json:"response" binding:"required"`
	}

	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}
		usernameStr := username.(string)

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		rpid, origin := extractRPIDAndOrigin(c)
		if !isAllowedDomain(rpid) {
			s.logger.Warn("domain not in whitelist", zap.String("domain", rpid))
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "域名不在白名单中"))
			return
		}

		sessionData, ok := consumeWebAuthnSession(req.SessionData, usernameStr, "login", rpid)
		if !ok {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "验证会话无效或已过期"))
			return
		}

		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", rpid, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(req.Response))
		if err != nil {
			s.logger.Error("parse credential request response failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "响应数据错误"))
			return
		}

		credentials, err := twofa.GetUserCredentialsForRPID(usernameStr, rpid)
		if err != nil {
			s.logger.Error("get user credentials for rpid failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取凭据失败"))
			return
		}

		if len(credentials) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "当前域名未注册WebAuthn凭据"))
			return
		}

		_, err = webAuthnService.FinishLogin(usernameStr, credentials, sessionData, parsedResponse)
		if err != nil {
			s.logger.Error("finish webauthn login failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "验证失败"))
			return
		}

		fullToken, err := upgradeTokenFromContext(c)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user verify webauthn successfully", zap.String("username", usernameStr))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

func (s *Server) handleDisableTwoFA() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		isTempToken, _ := c.Get("isTempToken")
		if isTemp, _ := isTempToken.(bool); isTemp {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "需要完整验证"))
			return
		}

		userConfig := &twofa.UserTwoFAConfig{
			IsSetup: false,
		}

		if err := twofa.SaveUserConfig(username.(string), userConfig); err != nil {
			s.logger.Error("save user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "保存配置失败"))
			return
		}

		s.logger.Info("user disabled twofa", zap.String("username", username.(string)))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"message": "二次验证已禁用",
		}))
	}
}

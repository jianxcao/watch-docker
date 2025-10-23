package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/twofa"
	"go.uber.org/zap"
)

// handleTwoFAStatus 获取二次验证状态
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

		// 二次验证是否启用由 IS_SECONDARY_VERIFICATION 环境变量控制
		envCfg := conf.EnvCfg
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"enabled": envCfg.IS_SECONDARY_VERIFICATION,
			"isSetup": userConfig.IsSetup,
			"method":  userConfig.Method,
		}))
	}
}

// handleOTPSetupInit 初始化 OTP 设置
func (s *Server) handleOTPSetupInit() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		// 生成 OTP 密钥
		secret, err := twofa.GenerateOTPSecret()
		if err != nil {
			s.logger.Error("generate otp secret failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成密钥失败"))
			return
		}

		// 生成二维码 URL
		qrCodeURL, err := twofa.GenerateQRCodeURL(secret, username.(string), "Watch Docker")
		if err != nil {
			s.logger.Error("generate qr code url failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成二维码失败"))
			return
		}

		// 将密钥临时存储到 session 中（使用 gin context）
		c.Set("otp_secret", secret)

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"secret":    secret,
			"qrCodeURL": qrCodeURL,
		}))
	}
}

// handleOTPSetupVerify 验证并启用 OTP
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

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		// 验证 OTP 代码
		if !twofa.ValidateOTPCode(req.Secret, req.Code) {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "验证码错误"))
			return
		}

		// 保存配置
		userConfig := &twofa.UserTwoFAConfig{
			Method:    twofa.MethodOTP,
			OTPSecret: req.Secret,
			IsSetup:   true,
		}

		if err := twofa.SaveUserConfig(username.(string), userConfig); err != nil {
			s.logger.Error("save user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "保存配置失败"))
			return
		}

		// 升级临时 token 为完整 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未找到token"))
			return
		}

		tempToken := authHeader[7:] // 去掉 "Bearer "
		fullToken, err := auth.UpgradeTempToken(tempToken)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user setup otp successfully", zap.String("username", username.(string)))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

// handleVerifyOTP 验证 OTP
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

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		// 获取用户配置
		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		if !userConfig.IsSetup || userConfig.Method != twofa.MethodOTP {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "未设置OTP"))
			return
		}

		// 验证 OTP 代码
		if !twofa.ValidateOTPCode(userConfig.OTPSecret, req.Code) {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "验证码错误"))
			return
		}

		// 升级临时 token 为完整 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未找到token"))
			return
		}

		tempToken := authHeader[7:] // 去掉 "Bearer "
		fullToken, err := auth.UpgradeTempToken(tempToken)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user verify otp successfully", zap.String("username", username.(string)))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

// handleWebAuthnRegisterBegin 开始 WebAuthn 注册
func (s *Server) handleWebAuthnRegisterBegin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		// 从请求头获取前端的 origin
		origin := c.GetHeader("Origin")
		if origin == "" {
			// 回退方案：使用 Referer 或构造默认值
			origin = "https://" + c.Request.Host
		}

		// 提取主机名（去掉端口）作为 RPID
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// 获取 WebAuthn 服务
		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", host, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		// 获取用户现有凭据
		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		// 开始注册
		options, session, err := webAuthnService.BeginRegistration(username.(string), userConfig.WebAuthnCredentials)
		if err != nil {
			s.logger.Error("begin webauthn registration failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "开始注册失败"))
			return
		}

		// 将 session 数据存储到临时存储（这里使用内存，生产环境应使用 Redis）
		sessionJSON, _ := json.Marshal(session)
		c.Set("webauthn_session", string(sessionJSON))

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"options":     options,
			"sessionData": string(sessionJSON),
		}))
	}
}

// handleWebAuthnRegisterFinish 完成 WebAuthn 注册
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
		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("bind json failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		// 从请求头获取前端的 origin
		origin := c.GetHeader("Origin")
		if origin == "" {
			// 回退方案：使用 Referer 或构造默认值
			origin = "https://" + c.Request.Host
		}

		// 提取主机名（去掉端口）作为 RPID
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// 获取 WebAuthn 服务
		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", host, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		// 解析 session 数据
		var sessionData webauthn.SessionData
		if err := json.Unmarshal([]byte(req.SessionData), &sessionData); err != nil {
			s.logger.Error("unmarshal session data failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "session数据错误"))
			return
		}

		// 解析响应（使用已读取的 req.Response）
		parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(req.Response))
		if err != nil {
			s.logger.Error("parse credential creation response failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "响应数据错误"))
			return
		}

		// 获取用户现有凭据
		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		// 完成注册
		credential, err := webAuthnService.FinishRegistration(username.(string), userConfig.WebAuthnCredentials, sessionData, parsedResponse)
		if err != nil {
			s.logger.Error("finish webauthn registration failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "完成注册失败"))
			return
		}

		// 保存凭据
		userConfig.Method = twofa.MethodWebAuthn
		userConfig.WebAuthnCredentials = append(userConfig.WebAuthnCredentials, *credential)
		userConfig.IsSetup = true

		if err := twofa.SaveUserConfig(username.(string), userConfig); err != nil {
			s.logger.Error("save user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "保存配置失败"))
			return
		}

		// 升级临时 token 为完整 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未找到token"))
			return
		}

		tempToken := authHeader[7:] // 去掉 "Bearer "
		fullToken, err := auth.UpgradeTempToken(tempToken)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user setup webauthn successfully", zap.String("username", username.(string)))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

// handleWebAuthnLoginBegin 开始 WebAuthn 验证
func (s *Server) handleWebAuthnLoginBegin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		// 从请求头获取前端的 origin
		origin := c.GetHeader("Origin")
		if origin == "" {
			// 回退方案：使用 Referer 或构造默认值
			origin = "https://" + c.Request.Host
		}

		// 提取主机名（去掉端口）作为 RPID
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// 获取 WebAuthn 服务
		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", host, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		// 获取用户配置
		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		if !userConfig.IsSetup || userConfig.Method != twofa.MethodWebAuthn {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "未设置WebAuthn"))
			return
		}

		// 开始验证
		options, session, err := webAuthnService.BeginLogin(username.(string), userConfig.WebAuthnCredentials)
		if err != nil {
			s.logger.Error("begin webauthn login failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "开始验证失败"))
			return
		}

		// 将 session 数据存储到临时存储
		sessionJSON, _ := json.Marshal(session)

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"options":     options,
			"sessionData": string(sessionJSON),
		}))
	}
}

// handleWebAuthnLoginFinish 完成 WebAuthn 验证
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

		var req Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "参数错误"))
			return
		}

		// 从请求头获取前端的 origin
		origin := c.GetHeader("Origin")
		if origin == "" {
			// 回退方案：使用 Referer 或构造默认值
			origin = "https://" + c.Request.Host
		}

		// 提取主机名（去掉端口）作为 RPID
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// 获取 WebAuthn 服务
		webAuthnService, err := twofa.NewWebAuthnService("Watch Docker", host, origin)
		if err != nil {
			s.logger.Error("create webauthn service failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "创建WebAuthn服务失败"))
			return
		}

		// 解析 session 数据
		var sessionData webauthn.SessionData
		if err := json.Unmarshal([]byte(req.SessionData), &sessionData); err != nil {
			s.logger.Error("unmarshal session data failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "session数据错误"))
			return
		}

		// 解析响应（使用已读取的 req.Response）
		parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(req.Response))
		if err != nil {
			s.logger.Error("parse credential request response failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "响应数据错误"))
			return
		}

		// 获取用户配置
		userConfig, err := twofa.GetUserConfig(username.(string))
		if err != nil {
			s.logger.Error("get user twofa config failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
			return
		}

		// 完成验证
		_, err = webAuthnService.FinishLogin(username.(string), userConfig.WebAuthnCredentials, sessionData, parsedResponse)
		if err != nil {
			s.logger.Error("finish webauthn login failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "验证失败"))
			return
		}

		// 升级临时 token 为完整 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未找到token"))
			return
		}

		tempToken := authHeader[7:] // 去掉 "Bearer "
		fullToken, err := auth.UpgradeTempToken(tempToken)
		if err != nil {
			s.logger.Error("upgrade temp token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "升级token失败"))
			return
		}

		s.logger.Info("user verify webauthn successfully", zap.String("username", username.(string)))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token": fullToken,
		}))
	}
}

// handleDisableTwoFA 禁用二次验证
func (s *Server) handleDisableTwoFA() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "未登录"))
			return
		}

		// 检查是否为临时 token
		isTempToken, _ := c.Get("isTempToken")
		if isTempToken.(bool) {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUnauthorized, "需要完整验证"))
			return
		}

		// 清除配置（将 IsSetup 设为 false，清空凭据）
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

package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/registry"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/scheduler"
	"github.com/jianxcao/watch-docker/backend/internal/twofa"
	"github.com/jianxcao/watch-docker/backend/internal/updater"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Server struct {
	logger              *zap.Logger
	docker              *dockercli.Client
	registry            *registry.Client
	scanner             *scanner.Scanner
	updater             *updater.Updater
	scheduler           *scheduler.Scheduler
	wsStatsManager      *StatsWebSocketManager
	composeClient       *composecli.Client
	streamManagerString *wsstream.StreamManager[string] // 用于 container stats (JSON 文本)
	streamManagerBytes  *wsstream.StreamManager[[]byte] // 用于 compose logs (二进制流)
}

func NewRouter(logger *zap.Logger, docker *dockercli.Client, reg *registry.Client, sc *scanner.Scanner, sch *scheduler.Scheduler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	// r.Use(ginzap(logger))

	// 创建 WebSocket 管理器（string 类型用于 JSON 文本）
	streamManagerString := wsstream.NewStreamManager[string]()

	// 创建 WebSocket 管理器（[]byte 类型用于日志流）
	streamManagerBytes := wsstream.NewStreamManager[[]byte]()

	// 创建 WebSocket 管理器（使用 wsstream 框架）
	wsStatsManager := NewStatsWebSocketManager(docker, sc, streamManagerString)

	// 创建 Compose 客户端
	cfg := config.Get()
	var composeClient *composecli.Client
	if cfg.Compose.Enabled {
		composeClient = composecli.NewClient(docker.GetDockerClient())
	}

	s := &Server{
		logger:              logger,
		docker:              docker,
		registry:            reg,
		scanner:             sc,
		updater:             updater.New(docker),
		scheduler:           sch,
		wsStatsManager:      wsStatsManager,
		composeClient:       composeClient,
		streamManagerString: streamManagerString,
		streamManagerBytes:  streamManagerBytes,
	}

	api := r.Group("/api/v1")
	{
		// 公开接口（不需要身份验证）
		api.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, NewSuccessRes(nil)) })
		api.POST("/login", s.handleLogin())
		api.POST("/logout", s.handleLogout())
		api.GET("/auth/status", s.handleAuthStatus())
		api.GET("/info", s.handleGetInfo())
	}

	// 二次验证相关路由（允许临时 token）
	twofa := api.Group("/2fa")
	twofa.Use(auth.TempTokenMiddleware())
	{
		twofa.GET("/status", s.handleTwoFAStatus())
		twofa.POST("/setup/otp/init", s.handleOTPSetupInit())
		twofa.POST("/setup/otp/verify", s.handleOTPSetupVerify())
		twofa.POST("/setup/webauthn/begin", s.handleWebAuthnRegisterBegin())
		twofa.POST("/setup/webauthn/finish", s.handleWebAuthnRegisterFinish())
		twofa.POST("/verify/otp", s.handleVerifyOTP())
		twofa.POST("/verify/webauthn/begin", s.handleWebAuthnLoginBegin())
		twofa.POST("/verify/webauthn/finish", s.handleWebAuthnLoginFinish())
		twofa.POST("/disable", s.handleDisableTwoFA())
	}

	// 需要身份验证的接口
	protected := api.Group("")
	protected.Use(auth.AuthMiddleware())
	{
		// 设置容器相关路由
		s.setupContainerRoutes(protected)

		// 设置镜像相关路由
		s.setupImageRoutes(protected)

		// 设置 Compose 相关路由
		if s.composeClient != nil {
			s.setupComposeRoutes(protected)
		}

		// 设置 Volume 相关路由
		s.setupVolumeRoutes(protected)

		// 设置网络相关路由
		s.setupNetworkRoutes(protected)

		// 其他路由
		protected.GET("/config", s.handleGetConfig())
		protected.POST("/config", s.handleSaveConfig())
		protected.GET("/logs", s.handleLogStream)

		// Shell WebSocket
		protected.GET("/shell", s.handleShellWebSocket())
	}
	s.setupStaticRoutes(r)

	return r
}

// handleGetInfo 获取系统信息
func (s *Server) handleGetInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取 Docker 版本信息
		dockerVersion, err := s.docker.GetVersion(c.Request.Context())
		if err != nil {
			s.logger.Error("get docker version", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "无法获取Docker版本信息"))
			return
		}

		// 获取当前应用版本
		envCfg := conf.EnvCfg

		info := gin.H{
			"dockerVersion":                  dockerVersion.Version,
			"dockerAPIVersion":               dockerVersion.APIVersion,
			"dockerPlatform":                 dockerVersion.Platform,
			"dockerGitCommit":                dockerVersion.GitCommit,
			"dockerGoVersion":                dockerVersion.GoVersion,
			"dockerBuildTime":                dockerVersion.BuildTime,
			"version":                        envCfg.VERSION_WATCH_DOCKER,
			"appPath":                        envCfg.APP_PATH,
			"isOpenDockerShell":              conf.EnvCfg.IS_OPEN_DOCKER_SHELL,
			"isSecondaryVerificationEnabled": conf.EnvCfg.IS_SECONDARY_VERIFICATION,
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"info": info}))
	}
}

// handleGetConfig 获取当前配置
func (s *Server) handleGetConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"config": cfg}))
	}
}

// handleSaveConfig 保存配置并使其生效
func (s *Server) handleSaveConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg config.Config
		oldCfg := config.Get()
		if err := c.ShouldBindJSON(&cfg); err != nil {
			s.logger.Error("invalid config format", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid config format"))
			return
		}

		if oldCfg.Logging.Level != cfg.Logging.Level {
			s.logger.Info("log level changed", zap.String("old_level", oldCfg.Logging.Level), zap.String("new_level", cfg.Logging.Level))
			if err := logger.SetLogLevel(cfg.Logging.Level); err != nil {
				s.logger.Error("设置日志出错， 请重启容器", zap.String("level", cfg.Logging.Level), zap.Error(err))
			}
		}

		// 设置为全局配置（这会触发保存到文件）
		config.SetGlobal(&cfg)

		// 动态更新 registry 客户端的认证凭据
		if s.scanner != nil {
			s.scanner.GetRegistryClient().UpdateManifestCredentials()
			s.logger.Info("registry credentials updated")
		}

		// 重启调度器以应用新的配置
		if s.scheduler != nil {
			s.logger.Info("restarting scheduler to apply new configuration")
			s.scheduler.Stop()
			s.scheduler.Start()
		}

		s.logger.Info("config updated successfully")
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handleLogin 登录接口
func (s *Server) handleLogin() gin.HandlerFunc {
	type LoginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "用户名和密码不能为空"))
			return
		}

		// 验证用户凭据
		if !auth.ValidateCredentials(req.Username, req.Password) {
			s.logger.Warn("login failed", zap.String("username", req.Username))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "用户名或密码错误"))
			return
		}

		// 检查是否启用二次验证
		envCfg := conf.EnvCfg
		if envCfg.IS_SECONDARY_VERIFICATION {
			// 检查用户是否已设置二次验证
			userConfig, err := twofa.GetUserConfig(req.Username)
			if err != nil {
				s.logger.Error("get user twofa config failed", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "获取配置失败"))
				return
			}

			// 提取 RPID（用于 WebAuthn 检查）
			rpid, _ := extractRPIDAndOrigin(c)

			// 检查当前域名/方法是否已设置
			isSetup, err := twofa.IsUserSetupForMethod(req.Username, userConfig.Method, rpid)
			if err != nil {
				s.logger.Error("check user setup status failed", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "检查设置状态失败"))
				return
			}

			// 生成临时 token
			tempToken, err := auth.GenerateTempToken(req.Username)
			if err != nil {
				s.logger.Error("generate temp token failed", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成token失败"))
				return
			}

			s.logger.Info("user login, need 2fa", zap.String("username", req.Username), zap.Bool("isSetup", isSetup), zap.String("method", string(userConfig.Method)), zap.String("rpid", rpid))
			c.JSON(http.StatusOK, NewSuccessRes(gin.H{
				"needTwoFA": true,
				"isSetup":   isSetup,
				"method":    userConfig.Method,
				"tempToken": tempToken,
				"username":  req.Username,
			}))
			return
		}

		// 未启用二次验证，直接生成完整 token
		token, err := auth.GenerateToken(req.Username)
		if err != nil {
			s.logger.Error("generate token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "生成token失败"))
			return
		}

		s.logger.Info("user logged in", zap.String("username", req.Username))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token":    token,
			"username": req.Username,
		}))
	}
}

// handleLogout 登出接口
func (s *Server) handleLogout() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 简单的登出响应，客户端需要删除本地存储的token
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"message": "登出成功"}))
	}
}

// handleAuthStatus 检查身份验证状态
func (s *Server) handleAuthStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"authEnabled": auth.IsAuthEnabled(),
		}))
	}
}

// handleLogStream 处理日志 SSE 流
func (s *Server) handleLogStream(c *gin.Context) {
	// 设置SSE相关头部
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	// 订阅日志
	ch := logger.Subscribe()
	defer logger.Unsubscribe(ch)

	// 将请求上下文用于取消
	ctx := c.Request.Context()

	// 简单心跳，防止某些代理超时
	c.Writer.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// 发送事件
			c.SSEvent("message", msg)
			// 手动刷新
			if f, ok := c.Writer.(http.Flusher); ok {
				f.Flush()
			}
		case <-ctx.Done():
			return
		}
	}
}

// setupStaticRoutes 设置静态文件路由 (前端资源)
func (s *Server) setupStaticRoutes(r *gin.Engine) {
	// 静态文件目录
	staticDir := conf.EnvCfg.STATIC_DIR

	// 检查静态文件目录是否存在
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		logger.Logger.Warn(fmt.Sprintf("静态文件目录不存在，跳过前端资源服务: %s", staticDir))
		return
	}

	logger.Logger.Info(fmt.Sprintf("启用前端静态文件服务, dir=%s", staticDir))

	// 根路径重定向到index.html
	r.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(staticDir, "index.html"))
	})
	r.HEAD("/", func(c *gin.Context) {
		// HEAD请求只返回头部，不需要文件内容
		c.Status(http.StatusOK)
	})

	// 处理所有非API路径的静态文件服务
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 如果是API请求，返回404
		if path == "/api" || strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		fs := gin.Dir(staticDir, false)
		fileServerStatic := http.StripPrefix("/", http.FileServer(fs))
		file, err := fs.Open(c.Request.URL.Path)
		if err != nil {
			c.File(filepath.Join(staticDir, "index.html"))
			return
		} else {
			fileServerStatic.ServeHTTP(c.Writer, c.Request)
		}
		defer file.Close()
	})
}

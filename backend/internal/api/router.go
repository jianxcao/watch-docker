package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/registry"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/scheduler"
	"github.com/jianxcao/watch-docker/backend/internal/updater"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Server struct {
	logger    *zap.Logger
	docker    *dockercli.Client
	registry  *registry.Client
	scanner   *scanner.Scanner
	updater   *updater.Updater
	scheduler *scheduler.Scheduler
}

func NewRouter(logger *zap.Logger, docker *dockercli.Client, reg *registry.Client, sc *scanner.Scanner, sch *scheduler.Scheduler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	// r.Use(ginzap(logger))

	s := &Server{logger: logger, docker: docker, registry: reg, scanner: sc, updater: updater.New(docker), scheduler: sch}

	api := r.Group("/api/v1")
	{
		// 公开接口（不需要身份验证）
		api.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, NewSuccessRes(nil)) })
		api.POST("/login", s.handleLogin())
		api.POST("/logout", s.handleLogout())
		api.GET("/auth/status", s.handleAuthStatus())
		api.GET("/info", s.handleGetInfo())
	}

	// 需要身份验证的接口
	protected := api.Group("")
	protected.Use(auth.AuthMiddleware())
	{

		protected.GET("/containers", s.handleListContainers())
		protected.POST("/containers/:id/update", s.handleUpdateContainer())
		protected.POST("/updates/run", s.handleBatchUpdate())
		protected.POST("/containers/:id/stop", s.handleStopContainer())
		protected.POST("/containers/:id/start", s.handleStartContainer())
		protected.DELETE("/containers/:id", s.handleDeleteContainer())
		protected.GET("/images", s.handleListImages())
		protected.DELETE("/images", s.handleDeleteImage())
		protected.GET("/config", s.handleGetConfig())
		protected.POST("/config", s.handleSaveConfig())
		protected.GET("/logs", s.handleLogStream)
	}
	s.setupStaticRoutes(r)
	return r
}

// simple logging middleware using zap
// func ginzap(logger *zap.Logger) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		path := c.Request.URL.Path
// 		method := c.Request.Method
// 		c.Next()
// 		status := c.Writer.Status()
// 		logger.Info("页面请求", zap.String("method", method), zap.String("path", path), zap.Int("status", status))
// 	}
// }

func (s *Server) handleUpdateContainer() gin.HandlerFunc {
	type reqBody struct {
		Image string `json:"image"`
	}
	return func(c *gin.Context) {
		id := c.Param("id")
		var body reqBody
		_ = c.ShouldBindJSON(&body)
		if body.Image == "" {
			// try inspect to get current image
			info, err := s.docker.InspectContainer(c.Request.Context(), id)
			if err != nil {
				s.logger.Error("inspect", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeImageRequired, "image required"))
				return
			}
			body.Image = info.Config.Image
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		if err := s.updater.UpdateContainer(ctx, id, body.Image); err != nil {
			s.logger.Error("update container", zap.String("container", id), zap.String("image", body.Image), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(codeForUpdateErr(err), err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handleBatchUpdate 触发一次批量更新：
// 1) 扫描当前状态
// 2) 对需要更新的容器逐个执行更新（串行）
func (s *Server) handleBatchUpdate() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()

		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, true)
		if err != nil {
			s.logger.Error("batch scan failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "scan failed"))
			return
		}

		updated := make([]string, 0)
		failed := make(map[string]string)
		failedCodes := make(map[string]int)
		for _, st := range statuses {
			if st.Skipped || st.Status != "UpdateAvailable" {
				continue
			}
			if !st.Running && !cfg.Docker.IncludeStopped {
				continue
			}
			uctx, cancelOne := context.WithTimeout(ctx, 5*time.Minute)
			if err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
				failed[st.Name] = err.Error()
				failedCodes[st.Name] = codeForUpdateErr(err)
				s.logger.Error("auto update failed", zap.String("container", st.Name), zap.String("image", st.Image), zap.Error(err))
			} else {
				updated = append(updated, st.Name)
			}
			cancelOne()
		}
		if len(failed) > 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUpdateFailed, "some containers failed to update"))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"updated": updated, "failed": failed, "failedCodes": failedCodes}))
	}
}

// codeForUpdateErr 根据错误来源映射错误码
func codeForUpdateErr(err error) int {
	if err == nil {
		return SUCCESS
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "pull:"):
		return CodeRegistryError
	case strings.HasPrefix(msg, "inspect:"), strings.HasPrefix(msg, "stop:"), strings.HasPrefix(msg, "create:"), strings.HasPrefix(msg, "start new:"), strings.HasPrefix(msg, "remove:"):
		return CodeDockerError
	default:
		return CodeUpdateFailed
	}
}

func (s *Server) handleListContainers() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		isUserCache := c.Query("isUserCache") == "true"
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, isUserCache)
		if err != nil {
			s.logger.Error("scan failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "scan failed"))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"containers": statuses}))
	}
}

func (s *Server) handleStopContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.docker.StopContainer(c.Request.Context(), id, 20); err != nil {
			s.logger.Error("stop container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleStartContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.docker.StartContainer(c.Request.Context(), id); err != nil {
			s.logger.Error("start container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleDeleteContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.docker.RemoveContainer(c.Request.Context(), id, true); err != nil {
			s.logger.Error("delete container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		// 删除成功后，立即获取更新后的容器列表返回给前端
		cfg := config.Get()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, true)
		if err != nil {
			s.logger.Error("scan after delete failed", zap.Error(err))
			// 即使扫描失败，也返回删除成功的响应
			c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true, "containers": statuses}))
	}
}

func (s *Server) handleListImages() gin.HandlerFunc {
	return func(c *gin.Context) {
		imgs, err := s.docker.ListImages(c.Request.Context())
		if err != nil {
			s.logger.Error("list images", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"images": imgs}))
	}
}

// handleDeleteImage 删除未使用的镜像（需前端确认未被使用）
// body: { "ref": "imageID or repo:tag", "force": false }
func (s *Server) handleDeleteImage() gin.HandlerFunc {
	type req struct {
		Ref   string `json:"ref"`
		Force bool   `json:"force"`
	}
	return func(c *gin.Context) {
		var r req
		if err := c.ShouldBindJSON(&r); err != nil || r.Ref == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "ref required"))
			return
		}
		if err := s.docker.RemoveImage(c.Request.Context(), r.Ref, r.Force, true); err != nil {
			s.logger.Error("remove image", zap.String("ref", r.Ref), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
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
			"dockerVersion":    dockerVersion.Version,
			"dockerAPIVersion": dockerVersion.APIVersion,
			"dockerPlatform":   dockerVersion.Platform,
			"dockerGitCommit":  dockerVersion.GitCommit,
			"dockerGoVersion":  dockerVersion.GoVersion,
			"dockerBuildTime":  dockerVersion.BuildTime,
			"version":          envCfg.VERSION_WATCH_DOCKER,
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

		// 生成 JWT token
		token, err := auth.GenerateToken(req.Username)
		if err != nil {
			s.logger.Error("generate token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorRes("生成token失败"))
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

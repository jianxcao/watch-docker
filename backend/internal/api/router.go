package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
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
	r.Use(ginzap(logger))

	s := &Server{logger: logger, docker: docker, registry: reg, scanner: sc, updater: updater.New(docker), scheduler: sch}

	api := r.Group("/api/v1")
	{
		api.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, NewSuccessRes(nil)) })
		api.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, NewSuccessRes(nil)) })
		api.GET("/containers", s.handleListContainers())
		api.POST("/containers/:id/update", s.handleUpdateContainer())
		api.POST("/updates/run", s.handleBatchUpdate())
		api.POST("/containers/:id/stop", s.handleStopContainer())
		api.POST("/containers/:id/start", s.handleStartContainer())
		api.DELETE("/containers/:id", s.handleDeleteContainer())
		api.GET("/images", s.handleListImages())
		api.DELETE("/images", s.handleDeleteImage())
		api.GET("/config", s.handleGetConfig())
		api.POST("/config", s.handleSaveConfig())
	}
	return r
}

// simple logging middleware using zap
func ginzap(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		status := c.Writer.Status()
		logger.Info("http", zap.String("method", method), zap.String("path", path), zap.Int("status", status))
	}
}

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

		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency)
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency)
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
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency)
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
		if err := c.ShouldBindJSON(&cfg); err != nil {
			s.logger.Error("invalid config format", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid config format"))
			return
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

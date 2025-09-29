package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jianxcao/watch-docker/backend/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// setupContainerRoutes 设置容器相关的路由
func (s *Server) setupContainerRoutes(protected *gin.RouterGroup) {
	protected.GET("/containers", s.handleListContainers())
	protected.POST("/containers/stats", s.handleGetContainersStats())
	protected.GET("/containers/stats/ws", s.handleStatsWebSocket())
	protected.POST("/containers/:id/update", s.handleUpdateContainer())
	protected.POST("/updates/run", s.handleBatchUpdate())
	protected.POST("/containers/:id/stop", s.handleStopContainer())
	protected.POST("/containers/:id/start", s.handleStartContainer())
	protected.DELETE("/containers/:id", s.handleDeleteContainer())
	protected.GET("/containers/:id/export", s.handleExportContainer())
	protected.POST("/system/prune", s.handlePruneSystem())
	protected.GET("/update/all", s.handleUpdateAll())
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

		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, true, true)
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
		isHaveUpdate := c.Query("isHaveUpdate") == "true"
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, isUserCache, isHaveUpdate)
		if err != nil {
			s.logger.Error("scan failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "scan failed"))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"containers": statuses}))
	}
}

func (s *Server) handleGetContainersStats() gin.HandlerFunc {
	type reqBody struct {
		ContainerIDs []string `json:"containerIds" binding:"required"`
	}

	return func(c *gin.Context) {
		var body reqBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request body"))
			return
		}

		if len(body.ContainerIDs) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "container ids required"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// 获取容器统计信息
		statsMap, err := s.docker.GetContainersStats(ctx, body.ContainerIDs)
		if err != nil {
			s.logger.Error("get containers stats failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取容器统计信息失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"stats": statsMap}))
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
		force := c.Query("force") == "true"
		if err := s.docker.RemoveContainer(c.Request.Context(), id, force); err != nil {
			s.logger.Error("delete container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		// 删除成功后，立即获取更新后的容器列表返回给前端
		cfg := config.Get()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, true, true)
		if err != nil {
			s.logger.Error("scan after delete failed", zap.Error(err))
			// 即使扫描失败，也返回删除成功的响应
			c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true, "containers": statuses}))
	}
}

// handleExportContainer 处理容器导出
func (s *Server) handleExportContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "container id required"))
			return
		}

		ctx := c.Request.Context()

		// 获取容器信息以生成文件名
		containerInfo, err := s.docker.InspectContainer(ctx, containerID)
		if err != nil {
			s.logger.Error("inspect container for export", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "容器不存在: "+containerID))
			return
		}

		s.logger.Info("exporting container", zap.String("containerID", containerID), zap.String("containerName", containerInfo.Name))

		// 导出容器
		reader, err := s.docker.ExportContainer(ctx, containerID)
		if err != nil {
			s.logger.Error("export container failed", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "导出容器失败: "+err.Error()))
			return
		}
		defer reader.Close()

		// 生成文件名
		filename := generateContainerFileName(&containerInfo)

		s.logger.Info("starting container export", zap.String("containerID", containerID), zap.String("filename", filename))

		// 设置响应头
		c.Header("Content-Type", "application/x-tar")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Header("Content-Transfer-Encoding", "binary")

		// 流式传输文件
		_, err = io.Copy(c.Writer, reader)
		if err != nil {
			s.logger.Error("copy container tar stream", zap.String("containerID", containerID), zap.Error(err))
			return
		}
		s.logger.Info("container export completed", zap.String("containerID", containerID))
	}
}

// handleStatsWebSocket 处理容器统计 WebSocket 连接
func (s *Server) handleStatsWebSocket() gin.HandlerFunc {
	return s.wsStatsManager.HandleWebSocket
}

func (s *Server) handleUpdateAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.scheduler.RunScanAndUpdate(c.Request.Context())
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// generateContainerFileName 根据容器信息生成导出文件名
func generateContainerFileName(container *container.InspectResponse) string {
	// 获取容器名称（去掉前缀斜杠）
	containerName := strings.TrimPrefix(container.Name, "/")

	if containerName != "" {
		// 替换不合法的文件名字符
		filename := strings.ReplaceAll(containerName, ":", "_")
		filename = strings.ReplaceAll(filename, "/", "_")
		filename = strings.ReplaceAll(filename, " ", "_")
		return fmt.Sprintf("container_%s.tar", filename)
	}

	// 如果没有名称，使用容器 ID 的前12位
	shortID := container.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	return fmt.Sprintf("container_%s.tar", shortID)
}

// handlePruneSystem 清理悬挂的文件系统、网络、镜像等
func (s *Server) handlePruneSystem() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()

		s.logger.Info("starting system prune")

		if err := s.docker.PruneSystem(ctx); err != nil {
			s.logger.Error("prune system failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "系统清理失败: "+err.Error()))
			return
		}

		s.logger.Info("system prune completed successfully")
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true, "message": "系统清理完成"}))
	}
}

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"go.uber.org/zap"
)

// setupVolumeRoutes 设置Volume相关路由
func (s *Server) setupVolumeRoutes(rg *gin.RouterGroup) {
	volumes := rg.Group("/volumes")
	{
		volumes.GET("", s.handleListVolumes())
		volumes.GET("/:name", s.handleGetVolume())
		volumes.POST("", s.handleCreateVolume())
		volumes.DELETE("/:name", s.handleDeleteVolume())
		volumes.POST("/prune", s.handlePruneVolumes())
	}
}

// handleListVolumes 获取Volume列表
func (s *Server) handleListVolumes() gin.HandlerFunc {
	return func(c *gin.Context) {
		response, err := s.docker.ListVolumes(c.Request.Context())
		if err != nil {
			s.logger.Error("list volumes failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取Volume列表失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}

// handleGetVolume 获取Volume详情
func (s *Server) handleGetVolume() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "Volume名称不能为空"))
			return
		}

		response, err := s.docker.GetVolume(c.Request.Context(), name)
		if err != nil {
			s.logger.Error("get volume failed", zap.String("name", name), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取Volume详情失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}

// handleCreateVolume 创建Volume
func (s *Server) handleCreateVolume() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dockercli.VolumeCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("invalid request", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "请求参数无效"))
			return
		}

		if req.Name == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "Volume名称不能为空"))
			return
		}

		volume, err := s.docker.CreateVolume(c.Request.Context(), &req)
		if err != nil {
			s.logger.Error("create volume failed", zap.String("name", req.Name), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "创建Volume失败"))
			return
		}

		s.logger.Info("volume created", zap.String("name", req.Name))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"volume": volume}))
	}
}

// handleDeleteVolume 删除Volume
func (s *Server) handleDeleteVolume() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "Volume名称不能为空"))
			return
		}

		// 获取force参数
		force := c.DefaultQuery("force", "false") == "true"

		err := s.docker.RemoveVolume(c.Request.Context(), name, force)
		if err != nil {
			s.logger.Error("delete volume failed", zap.String("name", name), zap.Bool("force", force), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "删除Volume失败"))
			return
		}

		s.logger.Info("volume deleted", zap.String("name", name))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handlePruneVolumes 清理未使用的Volume
func (s *Server) handlePruneVolumes() gin.HandlerFunc {
	return func(c *gin.Context) {
		response, err := s.docker.PruneVolumes(c.Request.Context())
		if err != nil {
			s.logger.Error("prune volumes failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "清理Volume失败"))
			return
		}

		s.logger.Info("volumes pruned",
			zap.Int("count", len(response.VolumesDeleted)),
			zap.Int64("spaceReclaimed", response.SpaceReclaimed))

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}


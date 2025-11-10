package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"go.uber.org/zap"
)

// setupNetworkRoutes 设置网络相关路由
func (s *Server) setupNetworkRoutes(rg *gin.RouterGroup) {
	networks := rg.Group("/networks")
	{
		networks.GET("", s.handleListNetworks())
		networks.GET("/:id", s.handleGetNetwork())
		networks.POST("", s.handleCreateNetwork())
		networks.DELETE("/:id", s.handleDeleteNetwork())
		networks.POST("/prune", s.handlePruneNetworks())
		networks.POST("/:id/connect", s.handleConnectContainer())
		networks.POST("/:id/disconnect", s.handleDisconnectContainer())
	}
}

// handleListNetworks 获取网络列表
func (s *Server) handleListNetworks() gin.HandlerFunc {
	return func(c *gin.Context) {
		response, err := s.docker.ListNetworks(c.Request.Context())
		if err != nil {
			s.logger.Error("list networks failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取网络列表失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}

// handleGetNetwork 获取网络详情
func (s *Server) handleGetNetwork() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络ID不能为空"))
			return
		}

		response, err := s.docker.GetNetwork(c.Request.Context(), id)
		if err != nil {
			s.logger.Error("get network failed", zap.String("id", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取网络详情失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}

// handleCreateNetwork 创建网络
func (s *Server) handleCreateNetwork() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dockercli.NetworkCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("invalid request", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "请求参数无效"))
			return
		}

		if req.Name == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络名称不能为空"))
			return
		}

		// 设置默认驱动
		if req.Driver == "" {
			req.Driver = "bridge"
		}

		network, err := s.docker.CreateNetwork(c.Request.Context(), &req)
		if err != nil {
			s.logger.Error("create network failed", zap.String("name", req.Name), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "创建网络失败"))
			return
		}

		s.logger.Info("network created",
			zap.String("name", req.Name),
			zap.String("id", network.ID),
			zap.String("driver", network.Driver))

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"network": network}))
	}
}

// handleDeleteNetwork 删除网络
func (s *Server) handleDeleteNetwork() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络ID不能为空"))
			return
		}

		err := s.docker.DeleteNetwork(c.Request.Context(), id)
		if err != nil {
			s.logger.Error("delete network failed", zap.String("id", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		s.logger.Info("network deleted", zap.String("id", id))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handlePruneNetworks 清理未使用的网络
func (s *Server) handlePruneNetworks() gin.HandlerFunc {
	return func(c *gin.Context) {
		response, err := s.docker.PruneNetworks(c.Request.Context())
		if err != nil {
			s.logger.Error("prune networks failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "清理网络失败"))
			return
		}

		s.logger.Info("networks pruned", zap.Int("count", len(response.NetworksDeleted)))

		c.JSON(http.StatusOK, NewSuccessRes(response))
	}
}

// handleConnectContainer 将容器连接到网络
func (s *Server) handleConnectContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络ID不能为空"))
			return
		}

		var req dockercli.NetworkConnectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("invalid request", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "请求参数无效"))
			return
		}

		if req.Container == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "容器ID不能为空"))
			return
		}

		err := s.docker.ConnectContainer(c.Request.Context(), id, &req)
		if err != nil {
			s.logger.Error("connect container failed",
				zap.String("networkId", id),
				zap.String("container", req.Container),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "连接容器到网络失败"))
			return
		}

		s.logger.Info("container connected to network",
			zap.String("networkId", id),
			zap.String("container", req.Container))

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// handleDisconnectContainer 从网络断开容器
func (s *Server) handleDisconnectContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络ID不能为空"))
			return
		}

		var req dockercli.NetworkDisconnectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("invalid request", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "请求参数无效"))
			return
		}

		if req.Container == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "容器ID不能为空"))
			return
		}

		err := s.docker.DisconnectContainer(c.Request.Context(), id, &req)
		if err != nil {
			s.logger.Error("disconnect container failed",
				zap.String("networkId", id),
				zap.String("container", req.Container),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "从网络断开容器失败"))
			return
		}

		s.logger.Info("container disconnected from network",
			zap.String("networkId", id),
			zap.String("container", req.Container))

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

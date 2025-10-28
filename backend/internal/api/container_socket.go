package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
	"go.uber.org/zap"
)

// StatsWebSocketManager WebSocket 连接管理器（使用 wsstream 框架）
type StatsWebSocketManager struct {
	docker        *dockercli.Client
	scanner       *scanner.Scanner
	streamManager *wsstream.StreamManager[string]
}

// NewStatsWebSocketManager 创建新的 WebSocket 管理器
func NewStatsWebSocketManager(docker *dockercli.Client, scanner *scanner.Scanner, streamManager *wsstream.StreamManager[string]) *StatsWebSocketManager {
	return &StatsWebSocketManager{
		docker:        docker,
		scanner:       scanner,
		streamManager: streamManager,
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (manager *StatsWebSocketManager) HandleStatsWebSocket(c *gin.Context) {
	// 使用 wsstream 框架处理 WebSocket 连接
	// 所有客户端共享同一个数据源（container-stats）
	manager.streamManager.HandleWebSocket(c, "container-stats", func() wsstream.StreamSource[string] {
		return wsstream.NewContainerStatsSource(wsstream.ContainerStatsSourceOptions{
			Docker:   manager.docker,
			Scanner:  manager.scanner,
			Interval: 2 * time.Second, // 每 2 秒推送一次
		})
	})
}

// handleContainerLogsWebSocket 处理容器日志的 WebSocket 连接
func (s *Server) handleContainerLogsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从路径参数获取容器 ID
		containerID := c.Param("containerID")
		// 从查询参数获取容器名称（用于日志显示）
		containerName := c.Query("projectName")

		if containerID == "" {
			logger.Logger.Error("Missing containerID parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing containerID parameter"})
			return
		}

		logger.Logger.Info("Container logs WebSocket connection request",
			zap.String("containerID", containerID),
			zap.String("containerName", containerName))

		// 使用 StreamManager 处理 WebSocket 连接
		// 每个容器的日志流是独立的，使用 containerID 作为唯一标识
		key := fmt.Sprintf("container-logs-%s", containerID)
		s.streamManagerBytes.HandleWebSocket(c, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewContainerLogsSource(wsstream.ContainerLogsSourceOptions{
				ContainerID:   containerID,
				ContainerName: containerName,
				DockerClient:  s.docker,
				Key:           key,
			})
		})
	}
}

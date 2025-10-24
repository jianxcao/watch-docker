package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
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
func (manager *StatsWebSocketManager) HandleWebSocket(c *gin.Context) {
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

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client WebSocket 客户端连接
type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *StatsWebSocketManager
}

// StatsWebSocketManager WebSocket 连接管理器
type StatsWebSocketManager struct {
	docker      *dockercli.Client
	scanner     *scanner.Scanner
	clients     map[*Client]bool
	latestStats []byte // 保存最新的统计数据
	register    chan *Client
	unregister  chan *Client
	mu          sync.RWMutex
}

// NewStatsWebSocketManager 创建新的 WebSocket 管理器
func NewStatsWebSocketManager(docker *dockercli.Client, scanner *scanner.Scanner) *StatsWebSocketManager {
	return &StatsWebSocketManager{
		docker:     docker,
		scanner:    scanner,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run 启动 WebSocket 管理器
func (manager *StatsWebSocketManager) Run(ctx context.Context) {
	ticker := time.NewTicker(1100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case client := <-manager.register:
			manager.mu.Lock()
			manager.clients[client] = true
			manager.mu.Unlock()

			// 通知 Docker 客户端有新连接
			manager.docker.AddStatsConnection(ctx)
			logger.Logger.Info(fmt.Sprintf("WebSocket 客户端已连接，当前连接数: %d", len(manager.clients)))

			// 如果有最新统计数据，立即发送给新客户端
			if manager.latestStats != nil {
				select {
				case client.send <- manager.latestStats:
				default:
					// 客户端发送通道满了，忽略
				}
			}

		case client := <-manager.unregister:
			manager.mu.Lock()
			if _, ok := manager.clients[client]; ok {
				delete(manager.clients, client)
				close(client.send)
			}
			manager.mu.Unlock()

			// 通知 Docker 客户端连接已断开
			manager.docker.RemoveStatsConnection()
			logger.Logger.Info(fmt.Sprintf("WebSocket 客户端已断开，当前连接数: %d", len(manager.clients)))

		case <-ticker.C:
			// 定期推送统计数据
			manager.mu.RLock()
			clientCount := len(manager.clients)
			manager.mu.RUnlock()

			if clientCount > 0 {
				manager.broadcastStats(ctx)
			}

		case <-ctx.Done():
			logger.Logger.Info("WebSocket 管理器停止")
			return
		}
	}
}

// broadcastStats 广播容器状态和统计数据到所有连接
func (manager *StatsWebSocketManager) broadcastStats(ctx context.Context) {
	// 获取配置
	cfg := config.Get()
	// 使用scanner获取完整的容器状态信息
	containerStatuses, err := manager.scanner.ScanOnce(ctx, cfg.Docker.IncludeStopped, cfg.Scan.Concurrency, true, false)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("获取容器状态失败: %v", err))
		return
	}
	if len(containerStatuses) == 0 {
		// 发送空数据
		response := map[string]interface{}{
			"type": "containers",
			"data": map[string]interface{}{
				"containers": []scanner.ContainerStatus{},
			},
			"timestamp": time.Now().Unix(),
		}

		message, err := json.Marshal(response)
		if err != nil {
			logger.Logger.Error(fmt.Sprintf("序列化空容器数据失败: %v", err))
			return
		}

		manager.broadcastMessage(message)
		return
	}

	// 收集运行中容器的ID
	runningContainerIDs := make([]string, 0)
	for _, cs := range containerStatuses {
		if cs.Running {
			runningContainerIDs = append(runningContainerIDs, cs.ID)
		}
	}

	// 获取运行中容器的统计数据
	var statsMap map[string]*dockercli.ContainerStats
	if len(runningContainerIDs) > 0 {
		statsMap, err = manager.docker.GetContainersStats(ctx, runningContainerIDs)
		if err != nil {
			logger.Logger.Error(fmt.Sprintf("获取容器统计失败: %v", err))
			// 即使获取统计失败，也要发送容器状态信息
			statsMap = make(map[string]*dockercli.ContainerStats)
		}
	} else {
		statsMap = make(map[string]*dockercli.ContainerStats)
	}

	// 将统计数据合并到容器状态中
	for i := range containerStatuses {
		// 添加统计数据（如果容器正在运行且有统计数据）
		if containerStatuses[i].Running && statsMap[containerStatuses[i].ID] != nil {
			containerStatuses[i].Stats = statsMap[containerStatuses[i].ID]
		}
	}

	// 构建响应数据
	response := map[string]interface{}{
		"type": "containers",
		"data": map[string]interface{}{
			"containers": containerStatuses,
		},
		"timestamp": time.Now().Unix(),
	}

	// 序列化为 JSON
	message, err := json.Marshal(response)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("序列化容器数据失败: %v", err))
		return
	}

	manager.broadcastMessage(message)
}

// broadcastMessage 广播消息到所有连接的客户端
func (manager *StatsWebSocketManager) broadcastMessage(message []byte) {
	// 保存最新数据
	manager.mu.Lock()
	manager.latestStats = message
	clients := make([]*Client, 0, len(manager.clients))
	for client := range manager.clients {
		clients = append(clients, client)
	}
	manager.mu.Unlock()

	// 直接发送给所有客户端
	for _, client := range clients {
		select {
		case client.send <- message:
			// 发送成功
		default:
			// 客户端发送通道满了，该客户端可能已经断开或处理太慢
			// 这里可以选择断开该客户端连接
			// go func(c *Client) {
			// 	manager.unregister <- c
			// }(client)
		}
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (manager *StatsWebSocketManager) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Logger.Error(fmt.Sprintf("WebSocket 升级失败: %v", err))
		return
	}

	// 创建客户端
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  manager,
	}

	// 注册客户端
	manager.register <- client

	// 启动写入和读取协程
	go client.writePump()
	go client.readPump()
}

// writePump 处理向 WebSocket 连接写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logger.Logger.Error(fmt.Sprintf("WebSocket 写入失败: %v", err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 处理从 WebSocket 连接读取消息
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Logger.Error(fmt.Sprintf("WebSocket 读取错误: %v", err))
			}
			break
		}
	}
}

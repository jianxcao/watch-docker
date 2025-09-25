package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
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
	docker     *dockercli.Client
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewStatsWebSocketManager 创建新的 WebSocket 管理器
func NewStatsWebSocketManager(docker *dockercli.Client) *StatsWebSocketManager {
	return &StatsWebSocketManager{
		docker:     docker,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
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
			log.Printf("WebSocket 客户端已连接，当前连接数: %d", len(manager.clients))

		case client := <-manager.unregister:
			manager.mu.Lock()
			if _, ok := manager.clients[client]; ok {
				delete(manager.clients, client)
				close(client.send)
			}
			manager.mu.Unlock()

			// 通知 Docker 客户端连接已断开
			manager.docker.RemoveStatsConnection()
			log.Printf("WebSocket 客户端已断开，当前连接数: %d", len(manager.clients))

		case message := <-manager.broadcast:
			manager.mu.RLock()
			for client := range manager.clients {
				select {
				case client.send <- message:
				default:
					delete(manager.clients, client)
					close(client.send)
				}
			}
			manager.mu.RUnlock()

		case <-ticker.C:
			// 定期推送统计数据
			if len(manager.clients) > 0 {
				manager.broadcastStats(ctx)
			}

		case <-ctx.Done():
			log.Println("WebSocket 管理器停止")
			return
		}
	}
}

// broadcastStats 广播统计数据到所有连接
func (manager *StatsWebSocketManager) broadcastStats(ctx context.Context) {
	// 获取所有运行中的容器
	containers, err := manager.docker.ListContainers(ctx, false)
	if err != nil {
		log.Printf("获取容器列表失败: %v", err)
		return
	}

	if len(containers) == 0 {
		return
	}

	// 获取容器 ID 列表
	containerIDs := make([]string, 0, len(containers))
	for _, c := range containers {
		containerIDs = append(containerIDs, c.ID)
	}

	// 获取统计数据
	statsMap, err := manager.docker.GetContainersStats(ctx, containerIDs)
	if err != nil {
		log.Printf("获取容器统计失败: %v", err)
		return
	}

	// 构建响应数据
	response := map[string]interface{}{
		"type": "stats",
		"data": map[string]interface{}{
			"stats": statsMap,
		},
		"timestamp": time.Now().Unix(),
	}

	// 序列化为 JSON
	message, err := json.Marshal(response)
	if err != nil {
		log.Printf("序列化统计数据失败: %v", err)
		return
	}

	// 广播到所有连接
	select {
	case manager.broadcast <- message:
	default:
		// 广播频道满了，跳过这次广播
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (manager *StatsWebSocketManager) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
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
				log.Printf("WebSocket 写入失败: %v", err)
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
				log.Printf("WebSocket 读取错误: %v", err)
			}
			break
		}
	}
}

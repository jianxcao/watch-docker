package wsstream

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

// StreamManager 管理所有的 StreamHub（泛型版本）
type StreamManager[T MessageType] struct {
	// key -> StreamHub 映射
	hubs map[string]*StreamHub[T]

	// 保护 hubs 映射的互斥锁
	mu sync.RWMutex

	// 客户端计数器（用于生成唯一 ID）
	clientCounter uint64
	counterMu     sync.Mutex
}

// NewStreamManager 创建新的 StreamManager
func NewStreamManager[T MessageType]() *StreamManager[T] {
	return &StreamManager[T]{
		hubs:          make(map[string]*StreamHub[T]),
		clientCounter: 0,
	}
}

// GetOrCreateHub 获取或创建指定 key 的 Hub
func (m *StreamManager[T]) GetOrCreateHub(key string, sourceFactory func() StreamSource[T]) *StreamHub[T] {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if hub, exists := m.hubs[key]; exists {
		logger.Logger.Debug("复用现有 StreamHub", zap.String("key", key))
		return hub
	}

	// 创建新的 Hub
	source := sourceFactory()
	hub := NewStreamHub(source, m)
	m.hubs[key] = hub

	// 启动 Hub
	go hub.Run()

	logger.Logger.Debug("创建新的 StreamHub", zap.String("key", key))
	return hub
}

// RemoveHub 移除指定 key 的 Hub
func (m *StreamManager[T]) RemoveHub(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hub, exists := m.hubs[key]; exists {
		delete(m.hubs, key)
		logger.Logger.Debug("移除 StreamHub", zap.String("key", key))
		// 不需要调用 hub.Close()，因为 Hub 已经在自己的 Run() 中处理了清理
		_ = hub
	}
}

func (m *StreamManager[T]) UpgradeWebSocket(c *gin.Context) (*websocket.Conn, error) {
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

func (m *StreamManager[T]) StartHub(conn *websocket.Conn, key string, sourceFactory func() StreamSource[T]) {

	// 获取或创建 Hub
	hub := m.GetOrCreateHub(key, sourceFactory)

	// 生成客户端 ID
	clientID := m.GenerateClientID()

	// 创建客户端
	client := NewClient(conn, hub, clientID)

	logger.Logger.Debug("新的 WebSocket 连接",
		zap.String("key", key),
		zap.String("clientId", clientID))

	// 注册客户端到 Hub
	hub.RegisterClient(client)

	// 启动客户端的读写协程
	go client.WritePump()
	go client.ReadPump()
}

// HandleWebSocket 处理 WebSocket 连接请求
func (m *StreamManager[T]) HandleWebSocket(c *gin.Context, key string, sourceFactory func() StreamSource[T]) {
	// 升级为 WebSocket 连接
	conn, err := m.UpgradeWebSocket(c)
	if err != nil {
		logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}
	m.StartHub(conn, key, sourceFactory)
}

// generateClientID 生成唯一的客户端 ID（内部使用）
func (m *StreamManager[T]) GenerateClientID() string {
	m.counterMu.Lock()
	defer m.counterMu.Unlock()
	m.clientCounter++
	return fmt.Sprintf("client-%d", m.clientCounter)
}

// Close 关闭所有 Hub
func (m *StreamManager[T]) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Logger.Debug("关闭 StreamManager", zap.Int("hubCount", len(m.hubs)))

	for _, hub := range m.hubs {
		// logger.Logger.Info("关闭 StreamHub", zap.String("key", key))
		hub.Close()
	}

	m.hubs = make(map[string]*StreamHub[T])
}

// GetHubCount 获取当前 Hub 数量（用于监控）
func (m *StreamManager[T]) GetHubCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hubs)
}

// GetClientCount 获取指定 Hub 的客户端数量
func (m *StreamManager[T]) GetClientCount(key string) int {
	m.mu.RLock()
	hub, exists := m.hubs[key]
	m.mu.RUnlock()

	if !exists {
		return 0
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

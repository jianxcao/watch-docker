package wsstream

import (
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:       auth.CheckWebSocketOrigin,
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

// StreamManager 管理所有的 StreamHub（泛型版本）
type StreamManager[T MessageType] struct {
	hubs          map[string]*StreamHub[T]
	mu            sync.RWMutex
	clientCounter uint64
	counterMu     sync.Mutex
}

// NewStreamManager 创建新的 StreamManager
func NewStreamManager[T MessageType]() *StreamManager[T] {
	return &StreamManager[T]{
		hubs: make(map[string]*StreamHub[T]),
	}
}

// GetOrCreateHub 获取或创建指定 key 的活跃 Hub
func (m *StreamManager[T]) GetOrCreateHub(key string, sourceFactory func() StreamSource[T]) *StreamHub[T] {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hub, exists := m.hubs[key]; exists && !hub.IsClosed() {
		logger.Logger.Debug("复用现有 StreamHub", zap.String("key", key))
		return hub
	}

	source := sourceFactory()
	hub := NewStreamHub(source, m)
	m.hubs[key] = hub
	go hub.Run()

	logger.Logger.Debug("创建新的 StreamHub", zap.String("key", key))
	return hub
}

// RemoveHub 移除指定 key 的 Hub
func (m *StreamManager[T]) RemoveHub(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.hubs[key]; exists {
		delete(m.hubs, key)
		logger.Logger.Debug("移除 StreamHub", zap.String("key", key))
	}
}

// RemoveHubIfMatch 仅当映射中的 Hub 与调用方一致时移除，避免退出中的旧 Hub 误删新创建的同 key Hub
func (m *StreamManager[T]) RemoveHubIfMatch(key string, target *StreamHub[T]) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hub, exists := m.hubs[key]; exists && hub == target {
		delete(m.hubs, key)
		logger.Logger.Debug("移除 StreamHub", zap.String("key", key))
	}
}

func (m *StreamManager[T]) UpgradeWebSocket(c *gin.Context) (*websocket.Conn, error) {
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

func (m *StreamManager[T]) StartHub(conn *websocket.Conn, key string, sourceFactory func() StreamSource[T]) {
	clientID := m.GenerateClientID()

	// 若现有 Hub 正处于退出瞬间导致 RegisterClient 返回 false，则移除并重试新建一次
	for attempt := 0; attempt < 2; attempt++ {
		hub := m.GetOrCreateHub(key, sourceFactory)
		client := NewClient(conn, hub, clientID)
		if hub.RegisterClient(client) {
			logger.Logger.Debug("新的 WebSocket 连接",
				zap.String("key", key),
				zap.String("clientId", clientID))
			go client.WritePump()
			go client.ReadPump()
			return
		}
		m.RemoveHubIfMatch(key, hub)
	}

	_ = conn.Close()
}

// HandleWebSocket 处理 WebSocket 连接请求
func (m *StreamManager[T]) HandleWebSocket(c *gin.Context, key string, sourceFactory func() StreamSource[T]) {
	conn, err := m.UpgradeWebSocket(c)
	if err != nil {
		logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}
	m.StartHub(conn, key, sourceFactory)
}

func (m *StreamManager[T]) GenerateClientID() string {
	m.counterMu.Lock()
	defer m.counterMu.Unlock()
	m.clientCounter++
	return fmt.Sprintf("client-%d", m.clientCounter)
}

// Close 关闭所有 Hub
func (m *StreamManager[T]) Close() {
	m.mu.Lock()
	hubsToClose := make([]*StreamHub[T], 0, len(m.hubs))
	for _, hub := range m.hubs {
		hubsToClose = append(hubsToClose, hub)
	}
	m.hubs = make(map[string]*StreamHub[T])
	m.mu.Unlock()

	logger.Logger.Debug("关闭 StreamManager", zap.Int("hubCount", len(hubsToClose)))
	for _, hub := range hubsToClose {
		hub.Close()
	}
}

func (m *StreamManager[T]) GetHubCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hubs)
}

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

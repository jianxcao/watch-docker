package wsstream

import (
	"errors"
	"fmt"
	"net/http"
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
	closed        bool
	pumpsWG       sync.WaitGroup
	clientCounter uint64
	counterMu     sync.Mutex
}

// NewStreamManager 创建新的 StreamManager
func NewStreamManager[T MessageType]() *StreamManager[T] {
	return &StreamManager[T]{
		hubs: make(map[string]*StreamHub[T]),
	}
}

// IsClosed 检查 StreamManager 是否已开始停机关闭
func (m *StreamManager[T]) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// GetOrCreateHub 获取或创建指定 key 的活跃 Hub（管理器已关闭时返回 nil）
func (m *StreamManager[T]) GetOrCreateHub(key string, sourceFactory func() StreamSource[T]) *StreamHub[T] {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

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
	if m.IsClosed() {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "服务正在关闭"})
		return nil, errors.New("stream manager closed")
	}
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

func (m *StreamManager[T]) StartHub(conn *websocket.Conn, key string, sourceFactory func() StreamSource[T]) {
	_ = m.StartHubWithResult(conn, key, sourceFactory)
}

// StartHubWithResult attaches conn to an active Hub or creates one, returning true if started.
// When returning false, conn has already been closed.
func (m *StreamManager[T]) StartHubWithResult(conn *websocket.Conn, key string, sourceFactory func() StreamSource[T]) bool {
	if m.IsClosed() {
		_ = conn.Close()
		return false
	}
	clientID := m.GenerateClientID()

	// 若现有 Hub 正处于退出瞬间导致 RegisterClient 返回 false，则移除并重试新建一次
	for attempt := 0; attempt < 2; attempt++ {
		hub := m.GetOrCreateHub(key, sourceFactory)
		if hub == nil {
			break
		}
		client := NewClient(conn, hub, clientID)
		if hub.RegisterClient(client) {
			m.mu.RLock()
			if m.closed {
				m.mu.RUnlock()
				hub.UnregisterClient(client)
				_ = conn.Close()
				return false
			}
			m.pumpsWG.Add(2)
			m.mu.RUnlock()

			logger.Logger.Debug("新的 WebSocket 连接",
				zap.String("key", key),
				zap.String("clientId", clientID))
			go func() {
				defer m.pumpsWG.Done()
				client.WritePump()
			}()
			go func() {
				defer m.pumpsWG.Done()
				client.ReadPump()
			}()
			return true
		}
		m.RemoveHubIfMatch(key, hub)
	}

	_ = conn.Close()
	return false
}

// AttachToHub attaches conn to an existing active Hub for key.
// If no active Hub exists or registration fails before shutdown, it returns false without closing conn.
func (m *StreamManager[T]) AttachToHub(conn *websocket.Conn, key string) bool {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return false
	}
	hub, exists := m.hubs[key]
	m.mu.RUnlock()
	if !exists || hub.IsClosed() {
		return false
	}

	clientID := m.GenerateClientID()
	client := NewClient(conn, hub, clientID)
	if !hub.RegisterClient(client) {
		return false
	}

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		hub.UnregisterClient(client)
		return false
	}
	m.pumpsWG.Add(2)
	m.mu.RUnlock()

	logger.Logger.Debug("加入现有 WebSocket 流",
		zap.String("key", key),
		zap.String("clientId", clientID))
	go func() {
		defer m.pumpsWG.Done()
		client.WritePump()
	}()
	go func() {
		defer m.pumpsWG.Done()
		client.ReadPump()
	}()
	return true
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

// Close 关闭所有 Hub 并拒绝后续新流创建
func (m *StreamManager[T]) Close() {
	m.mu.Lock()
	m.closed = true
	hubsToClose := make([]*StreamHub[T], 0, len(m.hubs))
	for _, hub := range m.hubs {
		hub.closing.Store(true)
		hub.cancel()
		hubsToClose = append(hubsToClose, hub)
	}
	m.hubs = make(map[string]*StreamHub[T])
	m.mu.Unlock()

	logger.Logger.Debug("关闭 StreamManager", zap.Int("hubCount", len(hubsToClose)))
	for _, hub := range hubsToClose {
		hub.Close()
	}
	m.pumpsWG.Wait()
}

func (m *StreamManager[T]) GetHubCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hubs)
}

// HasActiveHub reports whether an open Hub currently exists for key.
func (m *StreamManager[T]) HasActiveHub(key string) bool {
	m.mu.RLock()
	hub, exists := m.hubs[key]
	m.mu.RUnlock()
	return exists && !hub.IsClosed()
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

package wsstream

import (
	"context"
	"io"
	"sync"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// StreamHub 管理单个数据源的多个客户端连接
type StreamHub struct {
	// 数据源
	source StreamSource

	// 注册的客户端
	clients map[*Client]bool

	// 注册请求通道
	register chan *Client

	// 注销请求通道
	unregister chan *Client

	// 用于保护客户端映射的互斥锁
	mu sync.RWMutex

	// 数据源上下文和取消函数
	ctx    context.Context
	cancel context.CancelFunc

	// 数据源是否已启动
	sourceStarted bool

	// Hub 关闭通道
	done chan struct{}

	// Hub 的唯一标识
	key string

	// 父管理器的引用（用于通知清理）
	manager *StreamManager
}

// NewStreamHub 创建新的 StreamHub
func NewStreamHub(source StreamSource, manager *StreamManager) *StreamHub {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamHub{
		source:        source,
		clients:       make(map[*Client]bool),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		ctx:           ctx,
		cancel:        cancel,
		sourceStarted: false,
		done:          make(chan struct{}),
		key:           source.GetKey(),
		manager:       manager,
	}
}

// Run 启动 Hub 的主循环
func (h *StreamHub) Run() {
	defer func() {
		close(h.done)
		logger.Logger.Info("StreamHub 退出", zap.String("key", h.key))
	}()

	logger.Logger.Info("StreamHub 启动", zap.String("key", h.key))

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mu.Unlock()

			logger.Logger.Info("客户端已注册",
				zap.String("key", h.key),
				zap.String("clientId", client.id),
				zap.Int("totalClients", clientCount))

			// 如果这是第一个客户端，启动数据源
			if clientCount == 1 && !h.sourceStarted {
				go h.startStreamSource()
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			clientCount := len(h.clients)
			h.mu.Unlock()

			logger.Logger.Info("客户端已注销",
				zap.String("key", h.key),
				zap.String("clientId", client.id),
				zap.Int("remainingClients", clientCount))

			// 如果没有客户端了，停止数据源并通知管理器清理
			if clientCount == 0 {
				logger.Logger.Info("所有客户端已断开，停止数据源",
					zap.String("key", h.key))
				h.stopStreamSource()
				// 通知管理器移除此 Hub
				if h.manager != nil {
					h.manager.RemoveHub(h.key)
				}
				return
			}

		case <-h.ctx.Done():
			// 外部取消，关闭所有客户端
			logger.Logger.Info("StreamHub 被取消", zap.String("key", h.key))
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				client.conn.Close()
			}
			h.clients = make(map[*Client]bool)
			h.mu.Unlock()
			h.stopStreamSource()
			return
		}
	}
}

// startStreamSource 启动数据源并读取数据
func (h *StreamHub) startStreamSource() {
	h.mu.Lock()
	if h.sourceStarted {
		h.mu.Unlock()
		return
	}
	h.sourceStarted = true
	h.mu.Unlock()

	logger.Logger.Info("启动数据源", zap.String("key", h.key))

	reader, err := h.source.Start(h.ctx)
	if err != nil {
		logger.Logger.Error("启动数据源失败",
			zap.String("key", h.key),
			zap.Error(err))
		h.broadcastError("启动数据源失败: " + err.Error())
		return
	}
	defer reader.Close()

	// 发送欢迎消息
	h.broadcast([]byte("\x1b[32m=== 已连接到数据流 ===\x1b[0m\r\n"))

	// 读取数据流并广播
	buffer := make([]byte, 4096)
	for {
		select {
		case <-h.ctx.Done():
			logger.Logger.Info("数据源读取被取消", zap.String("key", h.key))
			return
		default:
			n, err := reader.Read(buffer)
			if n > 0 {
				// 复制数据并广播
				data := make([]byte, n)
				copy(data, buffer[:n])
				h.broadcast(data)
			}

			if err != nil {
				// 检查是否是 context 取消导致的
				select {
				case <-h.ctx.Done():
					// Context 已取消，这是正常的停止流程
					logger.Logger.Info("数据源读取被取消（检测到 context 取消）",
						zap.String("key", h.key))
					return
				default:
					// Context 未取消，这是真正的错误
					if err == io.EOF {
						logger.Logger.Info("数据源已结束",
							zap.String("key", h.key))
						h.broadcast([]byte("\r\n\x1b[33m=== 数据流已结束 ===\x1b[0m\r\n"))
					} else {
						logger.Logger.Error("读取数据源出错",
							zap.String("key", h.key),
							zap.Error(err))
						h.broadcastError("读取数据流出错: " + err.Error())
					}
					return
				}
			}
		}
	}
}

// stopStreamSource 停止数据源
func (h *StreamHub) stopStreamSource() {
	h.mu.Lock()
	if !h.sourceStarted {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	logger.Logger.Info("停止数据源", zap.String("key", h.key))

	// 取消上下文会停止数据源的读取
	h.cancel()

	// 调用数据源的 Stop 方法
	if err := h.source.Stop(); err != nil {
		logger.Logger.Warn("停止数据源时出错",
			zap.String("key", h.key),
			zap.Error(err))
	}
}

// broadcast 广播消息到所有客户端
func (h *StreamHub) broadcast(message []byte) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.send <- message:
			// 发送成功
		default:
			// 客户端发送通道满了，说明客户端处理太慢或已断开
			logger.Logger.Warn("客户端发送通道满",
				zap.String("key", h.key),
				zap.String("clientId", client.id))
		}
	}
}

// broadcastError 广播错误消息到所有客户端
func (h *StreamHub) broadcastError(errMsg string) {
	formattedMsg := []byte("\r\n\x1b[31m错误: " + errMsg + "\x1b[0m\r\n")
	h.broadcast(formattedMsg)
}

// RegisterClient 注册客户端
func (h *StreamHub) RegisterClient(client *Client) {
	h.register <- client
}

// UnregisterClient 注销客户端（通常由 client 自己调用）
func (h *StreamHub) UnregisterClient(client *Client) {
	h.unregister <- client
}

// Close 关闭 Hub
func (h *StreamHub) Close() {
	h.cancel()
	<-h.done
}

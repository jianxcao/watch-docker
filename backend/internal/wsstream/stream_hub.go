package wsstream

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// isNormalCloseError 判断错误是否是正常关闭相关的错误
func isNormalCloseError(err error) bool {
	if err == nil {
		return false
	}

	// 1. 检查 context 取消（最常见的正常关闭原因）
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// 2. 检查系统调用错误（使用类型断言，更精确）
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EPIPE: // Broken pipe
		case syscall.ECONNRESET: // Connection reset by peer
		case syscall.ECONNABORTED: // Connection aborted
		case syscall.ENOTCONN: // Socket is not connected
		case syscall.ESHUTDOWN: // Cannot send after transport endpoint shutdown
			return true
		}
	}

	// 3. 检查网络操作错误
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// 检查是否是关闭操作导致的错误
		if errors.Is(opErr.Err, net.ErrClosed) {
			return true
		}
		// 递归检查内部错误
		if isNormalCloseError(opErr.Err) {
			return true
		}
	}

	// 4. 检查路径错误（用于文件/管道操作）
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		// 递归检查内部错误
		if isNormalCloseError(pathErr.Err) {
			return true
		}
	}

	// 5. 检查特定的标准错误
	if errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrClosed) {
		return true
	}

	// 6. 最后才用字符串匹配作为兜底（用于一些包装过的错误）
	errMsg := err.Error()
	normalErrorStrings := []string{
		"use of closed network connection",
		"read/write on closed pipe",
	}

	for _, normalErrStr := range normalErrorStrings {
		if strings.Contains(errMsg, normalErrStr) {
			return true
		}
	}

	return false
}

// StreamHub 管理单个数据源的多个客户端连接（泛型版本）
type StreamHub[T MessageType] struct {
	// 数据源
	source StreamSource[T]

	// 注册的客户端
	clients map[*Client[T]]bool

	// 注册请求通道
	register chan *Client[T]

	// 注销请求通道
	unregister chan *Client[T]

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
	manager *StreamManager[T]
}

// NewStreamHub 创建新的 StreamHub
func NewStreamHub[T MessageType](source StreamSource[T], manager *StreamManager[T]) *StreamHub[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamHub[T]{
		source:        source,
		clients:       make(map[*Client[T]]bool),
		register:      make(chan *Client[T]),
		unregister:    make(chan *Client[T]),
		ctx:           ctx,
		cancel:        cancel,
		sourceStarted: false,
		done:          make(chan struct{}),
		key:           source.GetKey(),
		manager:       manager,
	}
}

// Run 启动 Hub 的主循环
func (h *StreamHub[T]) Run() {
	defer func() {
		close(h.done)
		logger.Logger.Debug("StreamHub 退出", zap.String("key", h.key))
	}()

	logger.Logger.Debug("StreamHub 启动", zap.String("key", h.key))

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mu.Unlock()

			logger.Logger.Debug("客户端已注册",
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

			logger.Logger.Debug("客户端已注销",
				zap.String("key", h.key),
				zap.String("clientId", client.id),
				zap.Int("remainingClients", clientCount))

			// 如果没有客户端了，停止数据源并通知管理器清理
			if clientCount == 0 {
				logger.Logger.Debug("所有客户端已断开，停止数据源",
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
			logger.Logger.Debug("StreamHub 被取消", zap.String("key", h.key))
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				client.conn.Close()
			}
			h.clients = make(map[*Client[T]]bool)
			h.mu.Unlock()
			h.stopStreamSource()
			return
		}
	}
}

// startStreamSource 启动数据源并读取数据
func (h *StreamHub[T]) startStreamSource() {
	h.mu.Lock()
	if h.sourceStarted {
		h.mu.Unlock()
		return
	}
	h.sourceStarted = true
	h.mu.Unlock()

	logger.Logger.Debug("启动数据源", zap.String("key", h.key))

	reader, err := h.source.Start(h.ctx)
	if err != nil {
		logger.Logger.Error("启动数据源失败",
			zap.String("key", h.key),
			zap.Error(err))
		h.broadcastError("启动数据源失败: " + err.Error())
		return
	}
	defer reader.Close()

	// 简单的读取循环
	for {
		select {
		case <-h.ctx.Done():
			logger.Logger.Debug("数据源读取被取消", zap.String("key", h.key))
			return
		default:
			message, err := reader.Read(h.ctx)
			if err != nil {
				// 检查是否是正常的关闭错误
				if isNormalCloseError(err) {
					logger.Logger.Debug("数据源读取被正常关闭",
						zap.String("key", h.key),
						zap.String("reason", err.Error()))
					h.closeAllClients()
					return
				}

				// EOF 是数据流正常结束
				if err == io.EOF {
					logger.Logger.Debug("数据源已结束",
						zap.String("key", h.key))
				} else {
					// 其他错误才是真正的错误
					logger.Logger.Error("读取数据源出错",
						zap.String("key", h.key),
						zap.Error(err))
					h.broadcastError("读取数据流出错: " + err.Error())
				}
				h.closeAllClients()
				return
			}

			// 直接广播消息（已经是完整的消息）
			h.broadcast(message)
		}
	}
}

// stopStreamSource 停止数据源
func (h *StreamHub[T]) stopStreamSource() {
	h.mu.Lock()
	if !h.sourceStarted {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	logger.Logger.Debug("停止数据源", zap.String("key", h.key))

	// 取消上下文会停止数据源的读取
	h.cancel()

	// 调用数据源的 Stop 方法
	if err := h.source.Stop(); err != nil {
		logger.Logger.Warn("停止数据源时出错",
			zap.String("key", h.key),
			zap.Error(err))
	}
}

// broadcast 广播泛型消息到所有客户端
func (h *StreamHub[T]) broadcast(message T) {
	h.mu.RLock()
	clients := make([]*Client[T], 0, len(h.clients))
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

// broadcastRaw 广播原始字节（用于欢迎消息等）
func (h *StreamHub[T]) broadcastRaw(data []byte) {
	// 将 []byte 转换为 T 类型
	var sample T
	var message T
	switch any(sample).(type) {
	case string:
		message = any(string(data)).(T)
	case []byte:
		message = any(data).(T)
	default:
		message = any(data).(T)
	}
	h.broadcast(message)
}

// broadcastError 广播错误消息到所有客户端
func (h *StreamHub[T]) broadcastError(errMsg string) {
	formattedMsg := []byte("\r\n\x1b[31m错误: " + errMsg + "\x1b[0m\r\n")
	h.broadcastRaw(formattedMsg)
}

// closeAllClients 关闭所有客户端连接（数据源结束或出错时调用）
func (h *StreamHub[T]) closeAllClients() {
	h.mu.Lock()
	clientCount := len(h.clients)
	clients := make([]*Client[T], 0, clientCount)
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()

	logger.Logger.Debug("关闭所有客户端连接",
		zap.String("key", h.key),
		zap.Int("clientCount", clientCount))

	// 给所有客户端一点时间接收最后的消息
	// 然后关闭连接
	go func() {
		// 等待 100ms 让最后的消息发送出去
		time.Sleep(100 * time.Millisecond)

		for _, client := range clients {
			// 直接关闭连接，conn.Close() 是幂等的，内部有互斥锁保护
			// 即使在 writePump/readPump 中也调用了 Close()，也不会有问题
			client.conn.Close()
		}
	}()
}

// RegisterClient 注册客户端
func (h *StreamHub[T]) RegisterClient(client *Client[T]) {
	h.register <- client
}

// UnregisterClient 注销客户端（通常由 client 自己调用）
func (h *StreamHub[T]) UnregisterClient(client *Client[T]) {
	h.unregister <- client
}

// Close 关闭 Hub
func (h *StreamHub[T]) Close() {
	h.cancel()
	<-h.done
}

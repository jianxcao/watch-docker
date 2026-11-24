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

func isNormalCloseError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EPIPE, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.ENOTCONN, syscall.ESHUTDOWN:
			return true
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, net.ErrClosed) || isNormalCloseError(opErr.Err) {
			return true
		}
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if isNormalCloseError(pathErr.Err) {
			return true
		}
	}

	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
		return true
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "use of closed network connection") ||
		strings.Contains(errMsg, "read/write on closed pipe")
}

// StreamHub 管理单个数据源的多个客户端连接（泛型版本）
type StreamHub[T MessageType] struct {
	source        StreamSource[T]
	clients       map[*Client[T]]bool
	register      chan *Client[T]
	unregister    chan *Client[T]
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	sourceStarted bool
	done          chan struct{}
	key           string
	manager       *StreamManager[T]
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
		if h.manager != nil {
			h.manager.RemoveHubIfMatch(h.key, h)
		}
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
			alreadyStarted := h.sourceStarted
			h.mu.Unlock()

			logger.Logger.Debug("客户端已注册",
				zap.String("key", h.key),
				zap.String("clientId", client.id),
				zap.Int("totalClients", clientCount))

			if clientCount == 1 && !alreadyStarted {
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

			if clientCount == 0 {
				logger.Logger.Debug("所有客户端已断开，停止数据源", zap.String("key", h.key))
				h.stopStreamSource()
				return
			}

		case <-h.ctx.Done():
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
		h.closeAllClients()
		return
	}
	defer reader.Close()

	for {
		select {
		case <-h.ctx.Done():
			logger.Logger.Debug("数据源读取被取消", zap.String("key", h.key))
			return
		default:
			message, err := reader.Read(h.ctx)
			if err != nil {
				if isNormalCloseError(err) {
					logger.Logger.Debug("数据源读取被正常关闭",
						zap.String("key", h.key),
						zap.String("reason", err.Error()))
					h.closeAllClients()
					return
				}

				if err == io.EOF {
					logger.Logger.Debug("数据源已结束", zap.String("key", h.key))
				} else {
					logger.Logger.Error("读取数据源出错",
						zap.String("key", h.key),
						zap.Error(err))
					h.broadcastError("读取数据流出错: " + err.Error())
				}
				h.closeAllClients()
				return
			}

			h.broadcast(message)
		}
	}
}

func (h *StreamHub[T]) stopStreamSource() {
	h.mu.Lock()
	started := h.sourceStarted
	h.mu.Unlock()
	if !started {
		h.cancel()
		return
	}

	logger.Logger.Debug("停止数据源", zap.String("key", h.key))
	h.cancel()

	if err := h.source.Stop(); err != nil {
		logger.Logger.Warn("停止数据源时出错",
			zap.String("key", h.key),
			zap.Error(err))
	}
}

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
		default:
			logger.Logger.Warn("客户端发送通道满",
				zap.String("key", h.key),
				zap.String("clientId", client.id))
		}
	}
}

func (h *StreamHub[T]) broadcastRaw(data []byte) {
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

func (h *StreamHub[T]) broadcastError(errMsg string) {
	formattedMsg := []byte("\r\n\x1b[31m错误: " + errMsg + "\x1b[0m\r\n")
	h.broadcastRaw(formattedMsg)
}

func (h *StreamHub[T]) closeAllClients() {
	h.mu.RLock()
	clientCount := len(h.clients)
	clients := make([]*Client[T], 0, clientCount)
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	logger.Logger.Debug("关闭所有客户端连接",
		zap.String("key", h.key),
		zap.Int("clientCount", clientCount))

	go func() {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-h.done:
		}
		for _, client := range clients {
			client.conn.Close()
		}
	}()
}

// RegisterClient 安全注册客户端（若 Hub 已退出则返回 false，避免永久阻塞）
func (h *StreamHub[T]) RegisterClient(client *Client[T]) bool {
	select {
	case h.register <- client:
		return true
	case <-h.ctx.Done():
		return false
	case <-h.done:
		return false
	}
}

// UnregisterClient 安全注销客户端（若 Hub 已退出则直接退出，避免 goroutine 泄漏）
func (h *StreamHub[T]) UnregisterClient(client *Client[T]) {
	select {
	case h.unregister <- client:
	case <-h.ctx.Done():
	case <-h.done:
	}
}

// IsClosed 检查 Hub 是否已处于停止状态
func (h *StreamHub[T]) IsClosed() bool {
	select {
	case <-h.ctx.Done():
		return true
	case <-h.done:
		return true
	default:
		return false
	}
}

// Close 关闭 Hub
func (h *StreamHub[T]) Close() {
	h.cancel()
	<-h.done
}

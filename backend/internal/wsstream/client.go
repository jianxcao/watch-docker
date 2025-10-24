package wsstream

import (
	"time"

	"github.com/gorilla/websocket"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// Client 代表一个 WebSocket 客户端连接（泛型版本）
type Client[T MessageType] struct {
	// WebSocket 连接
	conn *websocket.Conn

	// 发送消息的缓冲通道
	send chan T

	// 所属的 Hub
	hub *StreamHub[T]

	// 客户端 ID（用于日志）
	id string
}

// NewClient 创建新的 WebSocket 客户端
func NewClient[T MessageType](conn *websocket.Conn, hub *StreamHub[T], id string) *Client[T] {
	return &Client[T]{
		conn: conn,
		send: make(chan T, 256),
		hub:  hub,
		id:   id,
	}
}

// WritePump 处理向 WebSocket 连接写入消息
func (c *Client[T]) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		logger.Logger.Debug("客户端 writePump 退出", zap.String("clientId", c.id))
	}()

	// 确定消息类型
	messageType := c.getMessageType()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if !ok {
				// Hub 关闭了发送通道
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 根据泛型类型发送消息
			if err := c.writeMessage(messageType, message); err != nil {
				logger.Logger.Warn("WebSocket 写入失败",
					zap.String("clientId", c.id),
					zap.Error(err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			// 发送 Ping 消息保持连接
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Logger.Warn("WebSocket Ping 发送失败",
					zap.String("clientId", c.id),
					zap.Error(err))
				return
			}
		}
	}
}

// getMessageType 根据泛型类型确定 WebSocket 消息类型
func (c *Client[T]) getMessageType() int {
	var sample T
	switch any(sample).(type) {
	case string:
		return websocket.TextMessage
	case []byte:
		return websocket.BinaryMessage
	default:
		return websocket.BinaryMessage
	}
}

// writeMessage 根据消息类型写入 WebSocket
func (c *Client[T]) writeMessage(messageType int, message T) error {
	switch messageType {
	case websocket.TextMessage:
		// string 类型
		return c.conn.WriteMessage(messageType, []byte(any(message).(string)))
	case websocket.BinaryMessage:
		// []byte 类型
		return c.conn.WriteMessage(messageType, any(message).([]byte))
	default:
		return c.conn.WriteMessage(messageType, any(message).([]byte))
	}
}

// ReadPump 处理从 WebSocket 连接读取消息
// 主要用于检测客户端断开连接和处理 Pong 响应
func (c *Client[T]) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		logger.Logger.Debug("客户端 readPump 退出", zap.String("clientId", c.id))
	}()

	c.conn.SetReadLimit(1024 * 1024) // 1MB
	c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure) {
				logger.Logger.Info("WebSocket 异常关闭",
					zap.String("clientId", c.id),
					zap.Error(err))
			} else {
				logger.Logger.Debug("WebSocket 正常关闭",
					zap.String("clientId", c.id))
			}
			break
		}
		// 收到任何消息都重置读取超时
		c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	}
}

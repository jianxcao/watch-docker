package wsstream

import (
	"time"

	"github.com/gorilla/websocket"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// Client 代表一个 WebSocket 客户端连接
type Client struct {
	// WebSocket 连接
	conn *websocket.Conn

	// 发送消息的缓冲通道
	send chan []byte

	// 所属的 Hub
	hub *StreamHub

	// 客户端 ID（用于日志）
	id string
}

// NewClient 创建新的 WebSocket 客户端
func NewClient(conn *websocket.Conn, hub *StreamHub, id string) *Client {
	return &Client{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  hub,
		id:   id,
	}
}

// writePump 处理向 WebSocket 连接写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		logger.Logger.Debug("客户端 writePump 退出", zap.String("clientId", c.id))
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if !ok {
				// Hub 关闭了发送通道
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 使用 BinaryMessage 避免 UTF-8 验证问题
			if err := c.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
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

// readPump 处理从 WebSocket 连接读取消息
// 主要用于检测客户端断开连接和处理 Pong 响应
func (c *Client) readPump() {
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

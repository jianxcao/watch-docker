package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// handleComposeLogsWebSocket 处理 Compose 项目日志的 WebSocket 连接
func (s *Server) handleComposeLogsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从查询参数获取项目信息
		composeFile := c.Query("composeFile")
		projectName := c.Query("projectName")

		if composeFile == "" {
			logger.Logger.Error("Missing composeFile parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing composeFile parameter"})
			return
		}

		logger.Logger.Info("Compose logs WebSocket connection request",
			zap.String("composeFile", composeFile),
			zap.String("projectName", projectName))

		// 升级为 WebSocket 连接
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Logger.Error("Failed to upgrade WebSocket", zap.Error(err))
			return
		}
		defer conn.Close()

		// 设置连接参数
		conn.SetReadLimit(1024 * 1024)
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			return nil
		})

		// 创建上下文，用于控制日志流
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		// 启动心跳检测
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						logger.Logger.Warn("WebSocket Ping failed", zap.Error(err))
						cancel()
						return
					}
				}
			}
		}()

		// 监听客户端消息（主要用于检测断开连接）
		go func() {
			defer cancel()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
						logger.Logger.Warn("WebSocket read error", zap.Error(err))
					}
					return
				}
			}
		}()

		// 获取项目路径
		projectPath := path.Dir(composeFile)

		// 执行 docker compose logs 命令，使用流式输出
		result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
			ExecPath:      projectPath,
			Args:          []string{"logs", "--follow", "--timestamps", "--tail=500"},
			OperationName: "compose logs",
		})

		if result.Error != nil {
			logger.Logger.Error("Failed to start compose logs stream", zap.Error(result.Error))
			errMsg := fmt.Sprintf("启动日志流失败: %v\n", result.Error)
			conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			conn.WriteMessage(websocket.TextMessage, []byte(errMsg))
			return
		}
		defer result.Reader.Close()

		// 发送欢迎消息
		welcomeMsg := fmt.Sprintf("\x1b[32m=== 连接到项目 %s 的日志流 ===\x1b[0m\r\n", projectName)
		conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(welcomeMsg)); err != nil {
			logger.Logger.Error("Failed to send welcome message", zap.Error(err))
			return
		}

		// 读取日志流并发送到 WebSocket
		reader := bufio.NewReader(result.Reader)
		for {
			select {
			case <-ctx.Done():
				logger.Logger.Info("Compose logs stream context cancelled")
				return
			default:
				// 读取一行日志
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						logger.Logger.Info("Compose logs stream ended")
						return
					}
					logger.Logger.Error("Error reading compose logs", zap.Error(err))
					return
				}

				// 发送日志到 WebSocket
				conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, line); err != nil {
					logger.Logger.Error("Failed to write message to WebSocket", zap.Error(err))
					return
				}
			}
		}
	}
}

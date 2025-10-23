package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// TerminalMessage 终端消息结构
type TerminalMessage struct {
	Type string `json:"type"` // "input", "resize"
	Data string `json:"data"` // 输入数据
	Rows uint16 `json:"rows"` // 终端行数
	Cols uint16 `json:"cols"` // 终端列数
}

// handleShellWebSocket 处理 Shell WebSocket 连接
func (s *Server) handleShellWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Logger.Info("Shell WebSocket connection request")
		if !auth.IsAuthEnabled() {
			logger.Logger.Warn("开启shell必须设置登录，否则非常危险")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "开启shell必须设置登录，否则非常危险"})
			return
		}
		if !conf.EnvCfg.IS_OPEN_DOCKER_SHELL {
			logger.Logger.Warn("未开启shell功能，请在配置文件中开启,此操作非常危险")
			c.JSON(http.StatusForbidden, gin.H{"error": "未开启shell功能，请在配置文件中开启,此操作非常危险"})
			return
		} else {
			logger.Logger.Warn("开启shell非常危险，请谨慎使用")
		}
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

		// 创建上下文，用于控制 shell 会话
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		// 启动 shell
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}

		// 创建命令
		cmd := exec.CommandContext(ctx, shell)
		// 设置环境变量，支持中文显示
		cmd.Env = append(os.Environ(),
			"TERM=xterm-256color",
			"LANG=zh_CN.UTF-8",
			"LC_ALL=zh_CN.UTF-8",
		)

		// 启动 PTY
		ptmx, err := pty.Start(cmd)
		if err != nil {
			logger.Logger.Error("Failed to start PTY", zap.Error(err))
			errMsg := fmt.Sprintf("启动终端失败: %v\r\n", err)
			conn.WriteMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}
		defer func() {
			ptmx.Close()
			cmd.Process.Kill()
		}()

		// 设置初始终端大小
		pty.Setsize(ptmx, &pty.Winsize{
			Rows: 24,
			Cols: 80,
		})

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

		// 读取 PTY 输出并发送到 WebSocket
		go func() {
			defer cancel()
			buf := make([]byte, 1024)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					n, err := ptmx.Read(buf)
					if err != nil {
						if err != io.EOF {
							logger.Logger.Error("Error reading from PTY", zap.Error(err))
						}
						return
					}

					if n > 0 {
						conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
						if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
							logger.Logger.Error("Failed to write message to WebSocket", zap.Error(err))
							return
						}
					}
				}
			}
		}()

		// 读取 WebSocket 消息并写入 PTY
		for {
			select {
			case <-ctx.Done():
				logger.Logger.Info("Shell WebSocket context cancelled")
				return
			default:
				messageType, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
						logger.Logger.Warn("WebSocket read error", zap.Error(err))
					}
					return
				}

				switch messageType {
				case websocket.TextMessage:
					// 处理文本消息（用户输入）
					if _, err := ptmx.Write(message); err != nil {
						logger.Logger.Error("Failed to write to PTY", zap.Error(err))
						return
					}
				case websocket.BinaryMessage:
					// 处理二进制消息（也作为用户输入）
					if _, err := ptmx.Write(message); err != nil {
						logger.Logger.Error("Failed to write to PTY", zap.Error(err))
						return
					}
				}
			}
		}
	}
}

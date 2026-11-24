package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

var containerShellUpgrader = websocket.Upgrader{
	CheckOrigin:       auth.CheckWebSocketOrigin,
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

type ContainerShellMessage struct {
	Type string `json:"type"` // "input", "resize"
	Data string `json:"data"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

var allowedContainerShells = map[string]struct{}{
	"sh":            {},
	"bash":          {},
	"zsh":           {},
	"ash":           {},
	"/bin/sh":       {},
	"/bin/bash":     {},
	"/bin/zsh":      {},
	"/bin/ash":      {},
	"/usr/bin/bash": {},
	"/usr/bin/zsh":  {},
}

// handleContainerShellWebSocket 处理容器 Shell WebSocket 连接
func (s *Server) handleContainerShellWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := validateShellAccess(); err != nil {
			logger.Logger.Warn("container shell access denied", zap.Error(err))
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		containerID := c.Param("id")
		if containerID == "" {
			logger.Logger.Error("Missing containerID parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing containerID parameter"})
			return
		}

		shellType := strings.TrimSpace(c.DefaultQuery("shell", "sh"))
		if _, ok := allowedContainerShells[shellType]; !ok {
			logger.Logger.Warn("disallowed container shell requested", zap.String("shell", shellType))
			c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的 shell 类型，仅允许 sh/bash/zsh/ash"})
			return
		}

		logger.Logger.Info("Container Shell WebSocket connection request",
			zap.String("containerID", containerID),
			zap.String("shell", shellType))

		ctx := c.Request.Context()
		containerInfo, err := s.docker.InspectContainer(ctx, containerID)
		if err != nil {
			logger.Logger.Error("Failed to inspect container", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "容器不存在: " + containerID})
			return
		}

		if containerInfo.State == nil || !containerInfo.State.Running {
			logger.Logger.Warn("Container is not running", zap.String("containerID", containerID))
			c.JSON(http.StatusBadRequest, gin.H{"error": "容器未运行"})
			return
		}

		conn, err := containerShellUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Logger.Error("Failed to upgrade WebSocket", zap.Error(err))
			return
		}
		defer conn.Close()
		writer := &synchronizedWebSocketWriter{conn: conn}

		conn.SetReadLimit(1024 * 1024)
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		})

		execCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		execConfig := client.ExecCreateOptions{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			TTY:          true,
			Cmd:          []string{shellType},
		}

		execID, err := s.docker.ContainerExecCreate(execCtx, containerID, execConfig)
		if err != nil {
			logger.Logger.Error("Failed to create exec", zap.String("containerID", containerID), zap.Error(err))
			errMsg := fmt.Sprintf("创建终端失败: %v\r\n", err)
			_ = writer.writeMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}

		execAttach, err := s.docker.ContainerExecAttach(execCtx, execID.ID, client.ExecAttachOptions{
			TTY: true,
		})
		if err != nil {
			logger.Logger.Error("Failed to attach exec", zap.String("execID", execID.ID), zap.Error(err))
			errMsg := fmt.Sprintf("附加终端失败: %v\r\n", err)
			_ = writer.writeMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}
		defer func() {
			logger.Logger.Info("Container shell exec attached closed")
			cancel()
			execAttach.Close()
		}()

		logger.Logger.Info("Container shell exec attached",
			zap.String("containerID", containerID),
			zap.String("execID", execID.ID),
			zap.String("shell", shellType))

		// 心跳检测（使用 synchronizedWebSocketWriter 防止并发写 panic）
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-execCtx.Done():
					return
				case <-ticker.C:
					if err := writer.writeMessage(websocket.PingMessage, nil); err != nil {
						logger.Logger.Warn("WebSocket Ping failed", zap.Error(err))
						cancel()
						_ = conn.Close()
						return
					}
				}
			}
		}()

		// 读取 exec 输出并发送到 WebSocket；退出时关闭 conn 使主读取循环及时返回
		go func() {
			defer func() {
				cancel()
				_ = conn.Close()
			}()
			buf := make([]byte, 4096)
			for {
				select {
				case <-execCtx.Done():
					return
				default:
					n, err := execAttach.Reader.Read(buf)
					if err != nil {
						if err != io.EOF && execCtx.Err() == nil {
							logger.Logger.Error("Error reading from exec", zap.Error(err))
						}
						return
					}

					if n > 0 {
						if err := writer.writeMessage(websocket.BinaryMessage, buf[:n]); err != nil {
							logger.Logger.Debug("Failed to write message to WebSocket", zap.Error(err))
							return
						}
					}
				}
			}
		}()

		// 读取 WebSocket 消息并写入 exec
		for {
			select {
			case <-execCtx.Done():
				logger.Logger.Info("Container shell WebSocket context cancelled")
				return
			default:
				messageType, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
						logger.Logger.Warn("WebSocket read error", zap.Error(err))
					}
					return
				}
				_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))

				switch messageType {
				case websocket.TextMessage:
					var msg ContainerShellMessage
					if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" && msg.Rows > 0 && msg.Cols > 0 {
						resizeErr := s.docker.ContainerExecResize(execCtx, execID.ID, client.ExecResizeOptions{
							Height: uint(msg.Rows),
							Width:  uint(msg.Cols),
						})
						if resizeErr != nil {
							logger.Logger.Warn("Failed to resize terminal",
								zap.String("execID", execID.ID),
								zap.Uint16("rows", msg.Rows),
								zap.Uint16("cols", msg.Cols),
								zap.Error(resizeErr))
						}
					} else if err == nil && msg.Type == "input" {
						if _, err := execAttach.Conn.Write([]byte(msg.Data)); err != nil {
							return
						}
					} else {
						if _, err := execAttach.Conn.Write(message); err != nil {
							logger.Logger.Error("Failed to write to exec", zap.Error(err))
							return
						}
					}
				case websocket.BinaryMessage:
					if _, err := execAttach.Conn.Write(message); err != nil {
						logger.Logger.Error("Failed to write to exec", zap.Error(err))
						return
					}
				}
			}
		}
	}
}

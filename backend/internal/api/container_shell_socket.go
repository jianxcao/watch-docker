package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// containerShellUpgrader WebSocket 升级器（用于容器 Shell）
var containerShellUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

// ContainerShellMessage 容器终端消息结构
type ContainerShellMessage struct {
	Type string `json:"type"` // "input", "resize"
	Data string `json:"data"` // 输入数据
	Rows uint16 `json:"rows"` // 终端行数
	Cols uint16 `json:"cols"` // 终端列数
}

// handleContainerShellWebSocket 处理容器 Shell WebSocket 连接
func (s *Server) handleContainerShellWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		if containerID == "" {
			logger.Logger.Error("Missing containerID parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing containerID parameter"})
			return
		}

		// 获取 shell 类型，默认为 sh
		shellType := c.DefaultQuery("shell", "sh")
		logger.Logger.Info("Container Shell WebSocket connection request",
			zap.String("containerID", containerID),
			zap.String("shell", shellType))

		// 检查容器是否在运行
		ctx := c.Request.Context()
		containerInfo, err := s.docker.InspectContainer(ctx, containerID)
		if err != nil {
			logger.Logger.Error("Failed to inspect container", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "容器不存在: " + containerID})
			return
		}

		if !containerInfo.State.Running {
			logger.Logger.Warn("Container is not running", zap.String("containerID", containerID))
			c.JSON(http.StatusBadRequest, gin.H{"error": "容器未运行"})
			return
		}

		// 升级为 WebSocket 连接
		conn, err := containerShellUpgrader.Upgrade(c.Writer, c.Request, nil)
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

		// 创建上下文，用于控制 exec 会话
		execCtx, cancel := context.WithCancel(ctx)

		// 创建 docker exec 配置
		execConfig := container.ExecOptions{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          true,
			Cmd:          []string{shellType},
		}

		// 创建 exec 实例
		execID, err := s.docker.ContainerExecCreate(execCtx, containerID, execConfig)
		if err != nil {
			logger.Logger.Error("Failed to create exec", zap.String("containerID", containerID), zap.Error(err))
			errMsg := fmt.Sprintf("创建终端失败: %v\r\n", err)
			conn.WriteMessage(websocket.BinaryMessage, []byte(errMsg))
			cancel()
			return
		}

		// 附加到 exec 实例
		execAttach, err := s.docker.ContainerExecAttach(execCtx, execID.ID, container.ExecStartOptions{
			Tty: true,
		})

		defer func() {
			logger.Logger.Info("Container shell exec attached closed")
			cancel()
			execAttach.Close()
		}()

		if err != nil {
			logger.Logger.Error("Failed to attach exec", zap.String("execID", execID.ID), zap.Error(err))
			errMsg := fmt.Sprintf("附加终端失败: %v\r\n", err)
			conn.WriteMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}

		logger.Logger.Info("Container shell exec attached",
			zap.String("containerID", containerID),
			zap.String("execID", execID.ID),
			zap.String("shell", shellType))

		// 启动心跳检测
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-execCtx.Done():
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

		// 读取 exec 输出并发送到 WebSocket
		go func() {
			defer cancel()
			buf := make([]byte, 4096)
			for {
				select {
				case <-execCtx.Done():
					return
				default:
					if execCtx.Err() != nil {
						return
					}
					n, err := execAttach.Reader.Read(buf)
					if err != nil {
						// 检查是否是正常关闭（context 已取消）
						if err != io.EOF && execCtx.Err() == nil {
							// 只有在 context 未取消时才记录错误，避免正常关闭时的误报
							logger.Logger.Error("Error reading from exec", zap.Error(err))
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

				switch messageType {
				case websocket.TextMessage:
					// 尝试解析为 JSON 消息（支持 resize）
					var msg ContainerShellMessage
					if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
						// 调整终端大小
						resizeErr := s.docker.ContainerExecResize(execCtx, execID.ID, container.ResizeOptions{
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
					} else {
						// 普通文本输入
						if _, err := execAttach.Conn.Write(message); err != nil {
							logger.Logger.Error("Failed to write to exec", zap.Error(err))
							return
						}
					}
				case websocket.BinaryMessage:
					// 二进制消息作为用户输入
					if _, err := execAttach.Conn.Write(message); err != nil {
						logger.Logger.Error("Failed to write to exec", zap.Error(err))
						return
					}
				}
			}
		}
	}
}

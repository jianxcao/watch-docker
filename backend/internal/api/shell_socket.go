package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

var shellUpgrader = websocket.Upgrader{
	CheckOrigin:       auth.CheckWebSocketOrigin,
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

type TerminalMessage struct {
	Type string `json:"type"` // "input", "resize"
	Data string `json:"data"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type webSocketMessageWriter interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
}

type synchronizedWebSocketWriter struct {
	mu   sync.Mutex
	conn webSocketMessageWriter
}

func (w *synchronizedWebSocketWriter) writeMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	return w.conn.WriteMessage(messageType, data)
}

func (s *Server) handleShellOptions() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := validateShellAccess(); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, err.Error()))
			return
		}
		options, err := currentShellOptions()
		if err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(options))
	}
}

func validateShellAccess() error {
	if !auth.IsAuthEnabled() {
		return fmt.Errorf("开启 Shell 必须设置登录，否则非常危险")
	}
	if !conf.EnvCfg.IS_OPEN_DOCKER_SHELL {
		return fmt.Errorf("未开启 Shell 功能，请设置 IS_OPEN_DOCKER_SHELL=true")
	}
	return nil
}

func currentShellOptions() (ShellOptions, error) {
	target, err := configuredTerminalTarget()
	if err != nil {
		return ShellOptions{}, err
	}
	root := "/"
	targetUID := os.Geteuid()
	if target == terminalTargetHost {
		if err := validateHostRoot("/", hostTargetRoot); err != nil {
			return ShellOptions{}, err
		}
		if _, err := exec.LookPath("nsenter"); err != nil {
			return ShellOptions{}, fmt.Errorf("未找到 nsenter，请在镜像中安装 util-linux")
		}
		root = hostTargetRoot
		targetUID = 0
	}
	options := discoverShells(root, targetUID, os.Getenv("SHELL"))
	options.Target = target
	if len(options.Shells) == 0 {
		return ShellOptions{}, fmt.Errorf("%s 目标中没有可用的 Shell", target)
	}
	return options, nil
}

// handleShellWebSocket 处理 Shell WebSocket 连接
func (s *Server) handleShellWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Logger.Info("Shell WebSocket connection request")
		if err := validateShellAccess(); err != nil {
			logger.Logger.Warn(err.Error())
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		logger.Logger.Warn("开启 Shell 非常危险，请谨慎使用")

		options, err := currentShellOptions()
		if err != nil {
			logger.Logger.Error("resolve shell options", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		selectedShell := strings.TrimSpace(c.Query("shell"))
		if selectedShell == "" {
			selectedShell = options.DefaultShell
		}
		nsenterPath := ""
		if options.Target == terminalTargetHost {
			nsenterPath, err = exec.LookPath("nsenter")
			if err != nil {
				logger.Logger.Error("find nsenter", zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{"error": "未找到 nsenter，请在镜像中安装 util-linux"})
				return
			}
		}
		command, err := resolveShellCommand(options.Target, selectedShell, options, nsenterPath)
		if err != nil {
			logger.Logger.Warn("invalid shell selection", zap.String("shell", selectedShell), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		conn, err := shellUpgrader.Upgrade(c.Writer, c.Request, nil)
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

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		cmd := exec.CommandContext(ctx, command.Path, command.Args...)
		env := append(os.Environ(),
			"TERM=xterm-256color",
			"LANG=zh_CN.UTF-8",
			"LC_ALL=zh_CN.UTF-8",
		)
		if options.Target == terminalTargetHost {
			// 容器内运行用户 HOME=/home/watch-docker，进入宿主机 root 命名空间时重置为宿主机 root 环境
			env = append(env, "HOME=/root", "USER=root", "LOGNAME=root")
		}
		cmd.Env = env

		ptmx, err := pty.Start(cmd)
		if err != nil {
			logger.Logger.Error("Failed to start PTY", zap.Error(err))
			errMsg := fmt.Sprintf("启动终端失败: %v\r\n", err)
			_ = writer.writeMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}
		defer func() {
			cancel()
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
		}()

		_ = pty.Setsize(ptmx, &pty.Winsize{
			Rows: 24,
			Cols: 80,
		})

		// 心跳检测
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
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

		// 读取 PTY 输出并发送到 WebSocket
		go func() {
			defer func() {
				cancel()
				_ = conn.Close()
			}()
			buf := make([]byte, 4096)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					n, err := ptmx.Read(buf)
					if err != nil {
						if err != io.EOF {
							logger.Logger.Debug("Error reading from PTY", zap.Error(err))
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

		// 读取 WebSocket 消息：支持 JSON resize 控制帧与终端键盘输入
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
				_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))

				switch messageType {
				case websocket.TextMessage:
					var msg TerminalMessage
					if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" && msg.Rows > 0 && msg.Cols > 0 {
						if resizeErr := pty.Setsize(ptmx, &pty.Winsize{
							Rows: msg.Rows,
							Cols: msg.Cols,
						}); resizeErr != nil {
							logger.Logger.Warn("Failed to resize PTY", zap.Error(resizeErr))
						}
						continue
					} else if err == nil && msg.Type == "input" {
						if _, err := ptmx.Write([]byte(msg.Data)); err != nil {
							return
						}
						continue
					}
					if _, err := ptmx.Write(message); err != nil {
						logger.Logger.Error("Failed to write to PTY", zap.Error(err))
						return
					}
				case websocket.BinaryMessage:
					if _, err := ptmx.Write(message); err != nil {
						logger.Logger.Error("Failed to write to PTY", zap.Error(err))
						return
					}
				}
			}
		}
	}
}

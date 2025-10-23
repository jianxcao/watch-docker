package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// handleComposeCreateAndUpWebSocket 处理创建并启动 Compose 项目的 WebSocket 连接
func (s *Server) handleComposeCreateAndUpWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 升级为 WebSocket 连接
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Logger.Error("Failed to upgrade WebSocket", zap.Error(err))
			return
		}
		defer conn.Close()

		// 设置连接参数
		conn.SetReadLimit(1024 * 1024)
		conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分钟超时

		// 创建上下文
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()

		appPath := conf.EnvCfg.APP_PATH
		if appPath == "" {
			logger.Logger.Error("APP_PATH 未设置，无法创建项目")
			sendWSMessage(conn, "ERROR", "APP_PATH 未设置，无法创建项目")
			return
		}

		// 读取客户端发送的创建请求
		var req struct {
			Name        string `json:"name"`
			YamlContent string `json:"yamlContent"`
			Force       bool   `json:"force"`
		}

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if err := conn.ReadJSON(&req); err != nil {
			logger.Logger.Error("Failed to read create request", zap.Error(err))
			sendWSMessage(conn, "ERROR", "读取请求失败: "+err.Error())
			return
		}

		if req.Name == "" && req.YamlContent == "" {
			logger.Logger.Error("Invalid create request", zap.String("name", req.Name), zap.String("yamlContent", req.YamlContent))
			sendWSMessage(conn, "ERROR", "请求参数错误")
			return
		}
		logger.Logger.Info("Compose create and up request",
			zap.String("name", req.Name))
		composeDir := filepath.Join(appPath, req.Name)
		if req.YamlContent != "" {
			// 1. 创建项目文件
			sendWSMessage(conn, "INFO", fmt.Sprintf("正在创建或更新项目 %s...\r\n", req.Name))
			_, err := s.composeClient.SaveNewProject(ctx, req.Name, req.YamlContent, req.Force)
			if err != nil {
				logger.Logger.Error("Failed to create project", zap.Error(err))
				sendWSMessage(conn, "ERROR", "创建或更新项目文件失败: "+err.Error()+"\r\n")
				return
			}
			sendWSMessage(conn, "SUCCESS", fmt.Sprintf("项目文件创建或更新成: %s\r\n", composeDir))
		}
		sendWSMessage(conn, "INFO", "正在启动项目...\r\n")

		result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
			ExecPath:      composeDir,
			Args:          []string{"--ansi", "always", "up", "-d", "--remove-orphans", "--force-recreate"},
			OperationName: "compose up",
		})

		hasError := false
		if result.Error != nil {
			logger.Logger.Error("Failed to start compose up", zap.Error(result.Error))
			sendWSMessage(conn, "ERROR", "启动项目命令执行失败: "+result.Error.Error()+"\r\n")
			hasError = true
		}

		// 即使有错误，也尝试读取输出（可能包含详细错误信息）
		if result.Reader != nil {
			defer result.Reader.Close()

			logger.Logger.Info("开始读取 compose up 命令输出")

			// 使用字节块读取，保留原始格式（包括 \r 进度条）
			buffer := make([]byte, 4096) // 4KB 缓冲区
			for {
				select {
				case <-ctx.Done():
					logger.Logger.Warn("读取 compose up 输出时上下文取消")
					sendWSMessage(conn, "ERROR", "\r\n操作超时或被取消\r\n")
					return
				default:
					n, err := result.Reader.Read(buffer)
					if n > 0 {
						// 发送读取到的数据到前端（保留所有 \r 和 ANSI 转义序列）
						output := string(buffer[:n])
						sendWSMessage(conn, "LOG", output)
					}

					if err != nil {
						if err == io.EOF {
							// ✅ 命令执行完成，输出流自然结束
							logger.Logger.Info("compose up 命令输出读取完成（EOF）")
							goto endOfRead
						}
						// ❌ 读取过程中发生错误
						logger.Logger.Error("读取 compose up 输出时出错", zap.Error(err))
						goto endOfRead
					}
				}
			}
		endOfRead:
			logger.Logger.Info("结束读取 compose up 命令输出")
		}

		// 等待命令退出码
		// 注意：此时输出已经读取完毕（EOF），命令应该很快就会完成
		// 设置一个较短的超时作为保护，正常情况下应该立即收到退出码
		select {
		case exitCode, ok := <-result.ExitCode:
			if !ok {
				// 通道已关闭但没有收到数据，这不应该发生
				logger.Logger.Error("退出码通道异常关闭")
				hasError = true
			} else {
				logger.Logger.Info("收到 compose up 退出码", zap.Int("exitCode", exitCode))
				if exitCode != 0 {
					hasError = true
					logger.Logger.Error("compose up 执行失败", zap.Int("exitCode", exitCode))
				}
			}
		case <-time.After(10 * time.Second):
			// 输出已读完，但等待退出码超时
			// 这种情况很少见，可能是命令进程僵死
			logger.Logger.Warn("输出已读完但等待退出码超时（10秒），可能进程异常")
			hasError = true
		case <-ctx.Done():
			logger.Logger.Warn("等待退出码时上下文取消")
			sendWSMessage(conn, "ERROR", "\r\n操作被取消\r\n")
			return
		}

		// 如果启动失败，发送错误并结束
		if hasError {
			sendWSMessage(conn, "ERROR", "\r\n✗ 项目启动失败，请检查配置和日志\r\n")
			return
		}

		// 3. 获取项目状态
		sendWSMessage(conn, "INFO", "\r\n正在获取项目状态...\r\n")
		statusResult := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
			ExecPath:      composeDir,
			Args:          []string{"ps"},
			OperationName: "compose ps",
		})

		if statusResult.Error == nil {
			defer statusResult.Reader.Close()
			buffer := make([]byte, 4096)
			for {
				n, err := statusResult.Reader.Read(buffer)
				if n > 0 {
					sendWSMessage(conn, "LOG", string(buffer[:n]))
				}
				if err != nil {
					break
				}
			}
		}

		// 完成
		sendWSMessage(conn, "COMPLETE", composeDir)
	}
}

// sendWSMessage 发送 WebSocket 消息的辅助函数
// 返回 error 以便调用者决定如何处理
func sendWSMessage(conn *websocket.Conn, msgType, message string) error {
	conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	err := conn.WriteJSON(map[string]string{
		"type":    msgType,
		"message": message,
	})
	if err != nil {
		// 区分连接关闭错误和其他写入错误
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			logger.Logger.Info("WebSocket connection already closed", zap.String("msgType", msgType))
		} else if errors.Is(err, websocket.ErrCloseSent) {
			logger.Logger.Info("WebSocket connection closed", zap.Error(err))
		} else {
			logger.Logger.Error("Failed to send WebSocket message", zap.String("msgType", msgType), zap.Error(err))
		}
	}
	return err
}

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
			// 创建错误通道
			readErr := make(chan error, 1)

			// 启动读取 goroutine
			go func() {
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						logger.Logger.Info("WebSocket ReadMessage returned error",
							zap.Error(err),
							zap.Bool("isCloseError", websocket.IsCloseError(err,
								websocket.CloseNormalClosure,
								websocket.CloseGoingAway,
								websocket.CloseAbnormalClosure)))
						readErr <- err
						return
					}
				}
			}()

			// 等待 ctx 取消或读取错误
			select {
			case <-ctx.Done():
				logger.Logger.Info("WebSocket read goroutine: context cancelled")
				return
			case err := <-readErr:
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
					logger.Logger.Warn("WebSocket unexpected close error", zap.Error(err))
				} else {
					logger.Logger.Info("WebSocket closed normally", zap.Error(err))
				}
				return
			}
		}()

		// 获取项目路径
		projectPath := path.Dir(composeFile)

		// 执行 docker compose logs 命令，使用流式输出
		result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
			ExecPath:      projectPath,
			Args:          []string{"--ansi", "always", "logs", "--follow", "--timestamps", "--tail=500"},
			OperationName: "compose logs",
		})

		if result.Error != nil {
			logger.Logger.Error("Failed to start compose logs stream", zap.Error(result.Error))
			errMsg := fmt.Sprintf("启动日志流失败: %v\n", result.Error)
			conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			conn.WriteMessage(websocket.BinaryMessage, []byte(errMsg))
			return
		}
		defer result.Reader.Close()
		// 发送欢迎消息
		welcomeMsg := fmt.Sprintf("\x1b[32m=== 连接到项目 %s 的日志流 ===\x1b[0m\r\n", projectName)
		conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(welcomeMsg)); err != nil {
			logger.Logger.Error("Failed to send welcome message", zap.Error(err))
			return
		}

		// 读取日志流并发送到 WebSocket
		// 使用字节块读取，保留 ANSI 颜色和控制字符
		buffer := make([]byte, 4096) // 4KB 缓冲区
		for {
			// 读取日志块
			// 注意：Read 会阻塞，但当 ctx 取消时，底层的 docker compose 进程会终止
			// 导致 Read 返回 EOF 或其他错误，从而退出循环
			n, err := result.Reader.Read(buffer)
			if n > 0 {
				// 发送日志到 WebSocket（使用 BinaryMessage 避免 UTF-8 验证问题）
				conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					// 判断是否是连接关闭错误
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						logger.Logger.Info("WebSocket connection closed by client", zap.Error(err))
					} else if errors.Is(err, websocket.ErrCloseSent) {
						logger.Logger.Info("WebSocket connection closed", zap.Error(err))
					} else {
						logger.Logger.Error("Failed to write message to WebSocket", zap.Error(err))
					}
					return
				}
			}

			if err != nil {
				if err == io.EOF {
					logger.Logger.Info("Compose logs stream ended")
					return
				}
				logger.Logger.Error("Error reading compose logs", zap.Error(err))
				return
			}
		}
	}
}

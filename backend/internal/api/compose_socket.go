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
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
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
			sendWSMessage(conn, "ERROR", "APP_PATH 未设置，无法创建项目, APP_PATH是docker安装根目录，需要映射")
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

// handleComposeLogsWebSocketV2 处理 Compose 项目日志的 WebSocket 连接（使用新的流管理器）
func (s *Server) handleComposeLogsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从路径参数获取项目名称
		projectName := c.Param("projectName")
		// 从查询参数获取 composeFile（兼容旧逻辑）
		composeFile := c.Query("composeFile")

		if projectName == "" {
			logger.Logger.Error("Missing projectName parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing projectName parameter"})
			return
		}

		if composeFile == "" {
			logger.Logger.Error("Missing composeFile parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing composeFile parameter"})
			return
		}

		logger.Logger.Info("Compose logs WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("composeFile", composeFile))

		// 获取项目路径
		projectPath := path.Dir(composeFile)

		// 使用 StreamManager 处理 WebSocket 连接
		// 相同 projectName 的客户端会共享同一个日志流
		s.streamManager.HandleWebSocket(c, projectName, func() wsstream.StreamSource {
			return wsstream.NewComposeLogsSource(projectPath, projectName)
		})
	}
}

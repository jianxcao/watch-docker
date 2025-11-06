package api

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
	"go.uber.org/zap"
)

// handleComposeCreateAndUpWebSocket 处理创建并启动 Compose 项目的 WebSocket 连接
func (s *Server) handleComposeCreateAndUpWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		appPath := conf.EnvCfg.APP_PATH
		if appPath == "" {
			logger.Logger.Error("APP_PATH 未设置，无法创建项目")
			c.JSON(http.StatusBadRequest, gin.H{"error": "APP_PATH 未设置"})
			return
		}

		// 使用自定义的 WebSocket 处理，先读取参数再创建 Source
		s.handleComposeCreateAndUpWebSocketCustom(c, appPath)
	}
}

// handleComposeCreateAndUpWebSocketCustom 自定义 WebSocket 处理（先读取参数）
func (s *Server) handleComposeCreateAndUpWebSocketCustom(c *gin.Context, appPath string) {
	// 升级为 WebSocket 连接
	conn, err := s.streamManagerBytes.UpgradeWebSocket(c)
	if err != nil {
		logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}

	// 读取客户端发送的请求参数
	var req struct {
		Name        string `json:"name"`
		YamlContent string `json:"yamlContent"`
		Force       bool   `json:"force"`
	}

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := conn.ReadJSON(&req); err != nil {
		logger.Logger.Error("Failed to read create request", zap.Error(err))
		conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m读取请求失败: "+err.Error()+"\x1b[0m\r\n"))
		conn.Close()
		return
	}

	if req.Name == "" {
		logger.Logger.Error("Invalid request: missing project name")
		conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m缺少项目名称\x1b[0m\r\n"))
		conn.Close()
		return
	}

	logger.Logger.Info("Compose create and up request", zap.String("name", req.Name))
	composeDir := filepath.Join(appPath, req.Name)
	// 获取或创建 Hub（使用二进制模式）
	key := fmt.Sprintf("compose-up-%s", req.Name)
	s.streamManagerBytes.StartHub(conn, key, func() wsstream.StreamSource[[]byte] {
		return wsstream.NewComposeCreateUpSource(wsstream.ComposeCreateUpSourceOptions{
			ProjectName:   req.Name,
			YamlContent:   req.YamlContent,
			Force:         req.Force,
			ComposeDir:    composeDir,
			ComposeClient: s.composeClient,
			OnComplete: func(dir string) {
				logger.Logger.Info("Compose create and up 完成",
					zap.String("projectName", req.Name),
					zap.String("composeDir", dir))
			},
		})
	})
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
		s.streamManagerBytes.HandleWebSocket(c, projectName, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposeLogsSource(projectPath, projectName)
		})
	}
}

// handleComposePullWebSocket 处理 Compose 项目拉取镜像的 WebSocket 连接
func (s *Server) handleComposePullWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从路径参数获取项目名称
		projectName := c.Param("projectName")
		// 从查询参数获取 composeFile
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

		logger.Logger.Info("Compose pull WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("composeFile", composeFile))

		// 获取项目路径
		projectPath := path.Dir(composeFile)

		// 使用 StreamManager 处理 WebSocket 连接
		// 使用唯一的 key 来标识 pull 操作，避免与日志流冲突
		key := fmt.Sprintf("compose-pull-%s", projectName)
		s.streamManagerBytes.HandleWebSocket(c, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposePullSource(projectPath, projectName)
		})
	}
}

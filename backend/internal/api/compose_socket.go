package api

import (
	"fmt"
	"net/http"
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
		appPath := conf.EnvCfg.APP_PATH
		if appPath == "" {
			logger.Logger.Error("APP_PATH 未设置，无法创建项目")
			c.JSON(http.StatusBadRequest, gin.H{"error": "APP_PATH 未设置"})
			return
		}

		s.handleComposeCreateAndUpWebSocketCustom(c, appPath)
	}
}

func (s *Server) handleComposeCreateAndUpWebSocketCustom(c *gin.Context, appPath string) {
	conn, err := s.streamManagerBytes.UpgradeWebSocket(c)
	if err != nil {
		logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}

	var req struct {
		Name        string `json:"name"`
		YamlContent string `json:"yamlContent"`
		Force       bool   `json:"force"`
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := conn.ReadJSON(&req); err != nil {
		logger.Logger.Error("Failed to read create request", zap.Error(err))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m读取请求失败: "+err.Error()+"\x1b[0m\r\n"))
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if err := composecli.ValidateProjectName(req.Name); err != nil {
		logger.Logger.Warn("Invalid compose project name", zap.String("name", req.Name), zap.Error(err))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m非法项目名称: "+err.Error()+"\x1b[0m\r\n"))
		_ = conn.Close()
		return
	}

	composeDir, err := composecli.ValidateProjectDir(filepath.Join(appPath, req.Name))
	if err != nil {
		logger.Logger.Warn("Invalid compose project directory", zap.String("name", req.Name), zap.Error(err))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m非法项目路径: "+err.Error()+"\x1b[0m\r\n"))
		_ = conn.Close()
		return
	}

	logger.Logger.Info("Compose create and up request", zap.String("name", req.Name))
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

func (s *Server) handleComposeLogsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		projectPath, _, err := composecli.ValidateComposeFilePath(composeFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		logger.Logger.Info("Compose logs WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("projectPath", projectPath))

		s.streamManagerBytes.HandleWebSocket(c, projectName, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposeLogsSource(projectPath, projectName)
		})
	}
}

func (s *Server) handleComposeUpExistingWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		projectPath, _, err := composecli.ValidateComposeFilePath(composeFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		key := fmt.Sprintf("compose-up-existing-%s", projectName)
		s.streamManagerBytes.HandleWebSocket(c, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposeUpExistingSource(projectPath, projectName)
		})
	}
}

func (s *Server) handleComposePullWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		projectPath, _, err := composecli.ValidateComposeFilePath(composeFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		logger.Logger.Info("Compose pull WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("projectPath", projectPath))

		key := fmt.Sprintf("compose-pull-%s", projectName)
		s.streamManagerBytes.HandleWebSocket(c, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposePullSource(projectPath, projectName)
		})
	}
}

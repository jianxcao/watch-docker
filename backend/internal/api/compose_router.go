package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"go.uber.org/zap"
)

// setupComposeRoutes 设置 Compose 路由
func (s *Server) setupComposeRoutes(protected *gin.RouterGroup) {
	protected.GET("/compose", s.handleListComposeProjects())
	protected.POST("/compose/start", s.handleStartComposeProject())
	protected.POST("/compose/stop", s.handleStopComposeProject())
	protected.POST("/compose/restart", s.handleRestartComposeProject())
	protected.DELETE("/compose/delete", s.handleDeleteComposeProject())
	protected.POST("/compose/create", s.handleCreateComposeProject())
	protected.POST("/compose/new", s.handleSaveNewProject())
	protected.GET("/compose/:projectName/yaml", s.handleGetProjectYaml())
	protected.GET("/compose/logs/:projectName/ws", s.handleComposeLogsWebSocket())
	protected.GET("/compose/pull/:projectName/ws", s.handleComposePullWebSocket())
	protected.GET("/compose/create-and-up/ws", s.handleComposeCreateAndUpWebSocket())
}

func (s *Server) handleListComposeProjects() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
		defer cancel()

		projects, err := s.composeClient.ListProjects(ctx)
		if err != nil {
			s.logger.Error("scan compose projects failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "扫描 Compose 项目失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"projects": projects}))
	}
}

func (s *Server) handleStartComposeProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		var project composecli.ComposeProject
		if err := c.ShouldBindJSON(&project); err != nil {
			s.logger.Error("bind compose projects failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, err.Error()))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()

		if err := s.composeClient.StartProject(ctx, project.ComposeFile); err != nil {
			s.logger.Error("start compose project failed",
				zap.String("project", project.ComposeFile), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleStopComposeProject() gin.HandlerFunc {
	return func(c *gin.Context) {

		var project composecli.ComposeProject
		if err := c.ShouldBindJSON(&project); err != nil {
			s.logger.Error("bind compose project failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, err.Error()))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()

		if err := s.composeClient.StopProject(ctx, project.ComposeFile); err != nil {
			s.logger.Error("stop compose project failed",
				zap.String("project", project.ComposeFile), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleRestartComposeProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		var project composecli.ComposeProject
		if err := c.ShouldBindJSON(&project); err != nil {
			s.logger.Error("bind compose project failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, ""))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
		defer cancel()

		if err := s.composeClient.RestartProject(ctx, project.ComposeFile); err != nil {
			s.logger.Error("restart compose project failed",
				zap.String("project", project.ComposeFile), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleDeleteComposeProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		var project composecli.ComposeProject
		if err := c.ShouldBindJSON(&project); err != nil {
			s.logger.Error("bind compose project failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, err.Error()))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()

		if err := s.composeClient.DeleteProject(ctx, project.ComposeFile, project.Status); err != nil {
			s.logger.Error("delete compose project failed",
				zap.String("project", project.ComposeFile),
				zap.String("status", string(project.Status)),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleCreateComposeProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		var project composecli.ComposeProject
		if err := c.ShouldBindJSON(&project); err != nil {
			s.logger.Error("bind compose project failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, err.Error()))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()

		if err := s.composeClient.CreateProject(ctx, project.ComposeFile, project.RunningCount > 0, false); err != nil {
			s.logger.Error("create compose project failed",
				zap.String("project", project.ComposeFile), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleSaveNewProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name        string `json:"name" binding:"required"`
			YamlContent string `json:"yamlContent" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("bind new project request failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, err.Error()))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		composeFile, err := s.composeClient.SaveNewProject(ctx, req.Name, req.YamlContent, false)
		if err != nil {
			s.logger.Error("save new project failed",
				zap.String("name", req.Name), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"ok":          true,
			"composeFile": composeFile,
		}))
	}
}

func (s *Server) handleGetProjectYaml() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if projectName == "" {
			s.logger.Error("missing projectName parameter")
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, "缺少项目名称参数"))
			return
		}

		if composeFile == "" {
			s.logger.Error("missing composeFile parameter")
			c.JSON(http.StatusOK, NewErrorResCode(CodeInvalidRequest, "缺少 composeFile 参数"))
			return
		}

		yamlContent, err := s.composeClient.GetProjectYaml(composeFile)
		if err != nil {
			s.logger.Error("get project yaml failed",
				zap.String("projectName", projectName),
				zap.String("composeFile", composeFile),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"yamlContent": yamlContent,
		}))
	}
}

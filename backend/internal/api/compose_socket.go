package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"
	"go.uber.org/zap"
)

type composeDeployEntry struct {
	id             uint64
	streamKey      string
	cleanDir       string
	dirProjectHint string   // 目录名小写，用于未发现运行时名时的跨目录碰撞兜底，绝不作为 -p 传给 Compose
	runtimeProject string   // 已知运行时项目名（小写）；首次部署未发现时为空
	deployFiles    []string // 清洗后的有序部署文件路径列表
	exclusive      bool     // true 表示编辑器保存并部署（create-and-up/ws）独占目标
}

func cleanDeployFiles(files []string) []string {
	cleaned := make([]string, 0, len(files))
	for _, f := range files {
		trimmed := strings.TrimSpace(f)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, filepath.Clean(trimmed))
	}
	return cleaned
}

func dirBaseHint(cleanDir, fallbackName string) string {
	if cleanDir != "" && cleanDir != "." && cleanDir != string(filepath.Separator) {
		if base := filepath.Base(cleanDir); base != "" && base != "." && base != string(filepath.Separator) {
			return strings.ToLower(strings.TrimSpace(base))
		}
	}
	return strings.ToLower(strings.TrimSpace(fallbackName))
}

func (s *Server) releaseComposeDeploy(id uint64) {
	s.composeUpMu.Lock()
	defer s.composeUpMu.Unlock()
	delete(s.activeComposeDeploys, id)
}

func (s *Server) bindComposeDeployPlan(id uint64, plan composecli.DeployProjectPlan) error {
	s.composeUpMu.Lock()
	defer s.composeUpMu.Unlock()

	entry, ok := s.activeComposeDeploys[id]
	if !ok {
		return fmt.Errorf("部署登记已失效，请重试")
	}
	rp := composePlanEffectiveProject(plan)
	for otherID, other := range s.activeComposeDeploys {
		if otherID == id {
			continue
		}
		if rp != "" && (other.runtimeProject == rp || (other.runtimeProject == "" && other.dirProjectHint == rp)) {
			return fmt.Errorf("项目 %s 正在部署中，请等待当前部署完成后重试", plan.RuntimeProjectName)
		}
		if rp == "" && entry.dirProjectHint != "" && other.runtimeProject == entry.dirProjectHint {
			return fmt.Errorf("项目 %s 正在部署中，请等待当前部署完成后重试", entry.dirProjectHint)
		}
	}
	if rp != "" {
		entry.runtimeProject = rp
	}
	entry.deployFiles = cleanDeployFiles(plan.DeployFiles)
	return nil
}

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
		ComposeFile string `json:"composeFile"`
		Force       bool   `json:"force"`
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := conn.ReadJSON(&req); err != nil {
		logger.Logger.Error("Failed to read create request", zap.Error(err))
		sendComposeUpConflictAndClose(conn, "读取请求失败: "+err.Error())
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if err := composecli.ValidateProjectName(req.Name); err != nil {
		logger.Logger.Warn("Invalid compose project name", zap.String("name", req.Name), zap.Error(err))
		sendComposeUpConflictAndClose(conn, "非法项目名称: "+err.Error())
		return
	}

	composeDir, err := composecli.ValidateProjectDir(filepath.Join(appPath, req.Name))
	if err != nil {
		logger.Logger.Warn("Invalid compose project directory", zap.String("name", req.Name), zap.Error(err))
		sendComposeUpConflictAndClose(conn, "非法项目路径: "+err.Error())
		return
	}

	cleanDir := filepath.Clean(strings.TrimSpace(composeDir))
	hint := dirBaseHint(cleanDir, req.Name)

	logger.Logger.Info("Compose create and up request", zap.String("name", req.Name), zap.String("composeFile", req.ComposeFile))

	s.composeUpMu.Lock()
	if s.activeComposeDeploys == nil {
		s.activeComposeDeploys = make(map[uint64]*composeDeployEntry)
	}
	for _, e := range s.activeComposeDeploys {
		if e.cleanDir == cleanDir || (e.runtimeProject != "" && e.runtimeProject == hint) {
			s.composeUpMu.Unlock()
			sendComposeUpConflictAndClose(conn, "项目正在部署中，请等待当前部署完成后重试")
			return
		}
	}

	s.composeDeploySeq++
	id := s.composeDeploySeq
	key := fmt.Sprintf("compose-create-up:%d:%s", id, cleanDir)
	s.activeComposeDeploys[id] = &composeDeployEntry{
		id:             id,
		streamKey:      key,
		cleanDir:       cleanDir,
		dirProjectHint: hint,
		exclusive:      true,
	}
	release := func() {
		s.releaseComposeDeploy(id)
	}
	started := s.streamManagerBytes.StartHubWithResult(conn, key, func() wsstream.StreamSource[[]byte] {
		return wsstream.NewComposeCreateUpSource(wsstream.ComposeCreateUpSourceOptions{
			Key:           key,
			ProjectName:   req.Name,
			YamlContent:   req.YamlContent,
			ComposeFile:   req.ComposeFile,
			Force:         req.Force,
			ComposeDir:    cleanDir,
			ComposeClient: s.composeClient,
			OnPlanResolved: func(plan composecli.DeployProjectPlan) error {
				return s.bindComposeDeployPlan(id, plan)
			},
			OnComplete: func(dir string) {
				logger.Logger.Info("Compose create and up 完成",
					zap.String("projectName", req.Name),
					zap.String("composeDir", dir))
			},
			OnDone: release,
		})
	})
	s.composeUpMu.Unlock()

	if !started {
		release()
	}
}

func (s *Server) sendComposeWSError(c *gin.Context, errMsg string) {
	conn, err := s.streamManagerBytes.UpgradeWebSocket(c)
	if err != nil {
		logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31m"+errMsg+"\x1b[0m\r\n"))
	_ = conn.Close()
}

func buildComposeLogsStreamKey(projectName, projectPath string, argsPrefix []string) string {
	cleanPath := ""
	if strings.TrimSpace(projectPath) != "" {
		cleanPath = filepath.Clean(strings.TrimSpace(projectPath))
	}
	rawKey, _ := json.Marshal(struct {
		Project string   `json:"project"`
		Dir     string   `json:"dir"`
		Prefix  []string `json:"prefix"`
	}{
		Project: strings.ToLower(strings.TrimSpace(projectName)),
		Dir:     cleanPath,
		Prefix:  argsPrefix,
	})
	return "compose-logs:" + string(rawKey)
}

func (s *Server) handleComposeLogsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			s.sendComposeWSError(c, "非法项目名称: "+err.Error())
			return
		}
		projectPath, argsPrefix, err := s.composeClient.ResolveComposeTarget(c.Request.Context(), projectName, composeFile)
		if err != nil {
			s.sendComposeWSError(c, "无法读取项目日志: "+err.Error())
			return
		}

		logger.Logger.Info("Compose logs WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("projectPath", projectPath),
			zap.Strings("argsPrefix", argsPrefix))

		logsKey := buildComposeLogsStreamKey(projectName, projectPath, argsPrefix)
		s.streamManagerBytes.HandleWebSocket(c, logsKey, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposeLogsSourceWithKey(logsKey, projectPath, projectName, argsPrefix...)
		})
	}
}

func composePlanEffectiveProject(plan composecli.DeployProjectPlan) string {
	return strings.ToLower(strings.TrimSpace(plan.RuntimeProjectName))
}

func buildComposePlanStreamKey(op string, plan composecli.DeployProjectPlan) string {
	cleanDir := filepath.Clean(strings.TrimSpace(plan.ProjectDir))
	runtimeProj := composePlanEffectiveProject(plan)
	cleanFiles := cleanDeployFiles(plan.DeployFiles)
	rawKey, _ := json.Marshal(struct {
		Op             string   `json:"op"`
		Dir            string   `json:"dir"`
		RuntimeProject string   `json:"runtimeProject"`
		Files          []string `json:"files"`
	}{
		Op:             op,
		Dir:            cleanDir,
		RuntimeProject: runtimeProj,
		Files:          cleanFiles,
	})
	return "compose-" + op + ":" + string(rawKey)
}

func sendComposeUpConflictAndClose(conn *websocket.Conn, errMsg string) {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[31m"+errMsg+"\x1b[0m\r\n"))
	statusBytes, _ := json.Marshal(struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		Status:  "error",
		Message: errMsg,
	})
	_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte("\x00JSON:"), statusBytes...))
	_ = conn.Close()
}

func (s *Server) handleComposeUpExistingWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			s.sendComposeWSError(c, "非法项目名称: "+err.Error())
			return
		}
		plan, err := s.composeClient.ResolveManagedCardPlan(c.Request.Context(), projectName, composeFile)
		if err != nil {
			s.sendComposeWSError(c, "无法部署项目: "+err.Error())
			return
		}

		logger.Logger.Info("Compose up existing WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("projectPath", plan.ProjectDir),
			zap.String("runtimeProjectName", plan.RuntimeProjectName),
			zap.Strings("deployFiles", plan.DeployFiles))

		cleanDir := filepath.Clean(strings.TrimSpace(plan.ProjectDir))
		hint := dirBaseHint(cleanDir, projectName)
		rp := composePlanEffectiveProject(plan)
		cleanFiles := cleanDeployFiles(plan.DeployFiles)
		key := buildComposePlanStreamKey("up-existing", plan)
		argsPrefix := plan.ComposeArgsPrefix()

		conn, err := s.streamManagerBytes.UpgradeWebSocket(c)
		if err != nil {
			logger.Logger.Error("WebSocket 升级失败", zap.Error(err))
			return
		}

		s.composeUpMu.Lock()
		if s.activeComposeDeploys == nil {
			s.activeComposeDeploys = make(map[uint64]*composeDeployEntry)
		}

		var conflict bool
		var shareEntry *composeDeployEntry
		for _, e := range s.activeComposeDeploys {
			if e.cleanDir == cleanDir {
				if e.exclusive {
					conflict = true
					break
				}
				if e.runtimeProject != "" && rp != "" {
					if e.runtimeProject != rp {
						continue
					}
					if slices.Equal(e.deployFiles, cleanFiles) {
						shareEntry = e
						continue
					}
					conflict = true
					break
				}
				if e.runtimeProject == "" {
					if slices.Equal(e.deployFiles, cleanFiles) {
						shareEntry = e
						continue
					}
					conflict = true
					break
				}
				conflict = true
				break
			}
			if rp != "" && (e.runtimeProject == rp || (e.runtimeProject == "" && e.dirProjectHint == rp)) {
				conflict = true
				break
			}
			if rp == "" && e.runtimeProject != "" && e.runtimeProject == hint {
				conflict = true
				break
			}
			if rp == "" && e.runtimeProject == "" && hint != "" && e.dirProjectHint == hint {
				conflict = true
				break
			}
		}

		if conflict {
			s.composeUpMu.Unlock()
			sendComposeUpConflictAndClose(conn, "项目正在使用其他配置部署中，请等待当前部署完成后重试")
			return
		}

		if shareEntry != nil {
			attached := s.streamManagerBytes.AttachToHub(conn, shareEntry.streamKey)
			s.composeUpMu.Unlock()
			if !attached {
				sendComposeUpConflictAndClose(conn, "上一轮部署正在收尾，请稍后重试")
			}
			return
		}

		if s.streamManagerBytes.HasActiveHub(key) {
			s.composeUpMu.Unlock()
			sendComposeUpConflictAndClose(conn, "上一轮部署正在收尾，请稍后重试")
			return
		}

		s.composeDeploySeq++
		id := s.composeDeploySeq
		s.activeComposeDeploys[id] = &composeDeployEntry{
			id:             id,
			streamKey:      key,
			cleanDir:       cleanDir,
			dirProjectHint: hint,
			runtimeProject: rp,
			deployFiles:    cleanFiles,
		}
		release := func() {
			s.releaseComposeDeploy(id)
		}
		started := s.streamManagerBytes.StartHubWithResult(conn, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposeUpExistingSourceWithOptions(wsstream.ComposeUpExistingSourceOptions{
				Key:         key,
				ProjectPath: plan.ProjectDir,
				ProjectName: projectName,
				ArgsPrefix:  argsPrefix,
				OnDone:      release,
			})
		})
		s.composeUpMu.Unlock()

		if !started {
			release()
		}
	}
}

func (s *Server) handleComposePullWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectName := c.Param("projectName")
		composeFile := c.Query("composeFile")

		if err := composecli.ValidateProjectName(projectName); err != nil {
			s.sendComposeWSError(c, "非法项目名称: "+err.Error())
			return
		}
		plan, err := s.composeClient.ResolveManagedCardPlan(c.Request.Context(), projectName, composeFile)
		if err != nil {
			s.sendComposeWSError(c, "无法拉取镜像: "+err.Error())
			return
		}

		logger.Logger.Info("Compose pull WebSocket connection request",
			zap.String("projectName", projectName),
			zap.String("projectPath", plan.ProjectDir),
			zap.String("runtimeProjectName", plan.RuntimeProjectName),
			zap.Strings("deployFiles", plan.DeployFiles))

		key := buildComposePlanStreamKey("pull", plan)
		argsPrefix := plan.ComposeArgsPrefix()
		s.streamManagerBytes.HandleWebSocket(c, key, func() wsstream.StreamSource[[]byte] {
			return wsstream.NewComposePullSourceWithKey(key, plan.ProjectDir, projectName, argsPrefix...)
		})
	}
}

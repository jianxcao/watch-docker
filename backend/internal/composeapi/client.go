package composeapi

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// Client Compose API 客户端
type Client struct {
	service api.Compose
}

// NewClient 创建新的 Compose API 客户端
func NewClient() (*Client, error) {

	// 创建 Docker CLI
	cli, err := command.NewDockerCli()
	if err != nil {
		return nil, err
	}

	// 初始化 CLI
	opts := flags.NewClientOptions()
	if err := cli.Initialize(opts); err != nil {
		return nil, err
	}

	// 创建 compose service
	service := compose.NewComposeService(cli)

	return &Client{
		service: service,
	}, nil
}

// ListProjects 列出所有 Compose 项目
func (c *Client) ListProjects(ctx context.Context) ([]ComposeProject, error) {
	logger.Logger.Debug("listing compose projects")

	// 首先扫描文件系统中的项目
	projects := c.scanProjects(ctx)
	mapScanProjects := make(map[string]ComposeProject)
	for _, project := range projects {
		mapScanProjects[project.Name] = project
	}

	// 使用 compose API 获取运行中的项目状态
	stacks, err := c.listAllProjects(ctx)
	if err != nil {
		logger.Logger.Error("failed to list running projects", zap.Error(err))
		// 即使失败也返回扫描到的项目
		result := make([]ComposeProject, 0, len(mapScanProjects))
		for _, project := range mapScanProjects {
			result = append(result, project)
		}
		return result, nil
	}

	// 合并运行状态信息
	for _, stack := range stacks {
		status, runningCount, exitedCount, createdCount := c.parseStackStatus(stack.Status)

		// 获取配置文件路径
		configFile := ""
		if len(stack.ConfigFiles) > 0 {
			configFile = stack.ConfigFiles
		}

		mapScanProjects[stack.Name] = ComposeProject{
			Name:         stack.Name,
			ComposeFile:  configFile,
			Status:       status,
			RunningCount: runningCount,
			ExitedCount:  exitedCount,
			CreatedCount: createdCount,
		}
	}

	// 转换为数组
	result := make([]ComposeProject, 0, len(mapScanProjects))
	for _, project := range mapScanProjects {
		result = append(result, project)
	}

	logger.Logger.Info("projects listed successfully", zap.Int("count", len(result)))
	return result, nil
}

// StartProject 启动项目（返回流式输出 channel）
func (c *Client) StartProject(ctx context.Context, composeFile string) (<-chan StreamMessage, error) {
	ch := make(chan StreamMessage, 100)

	project, err := c.loadProject(ctx, composeFile)
	if err != nil {
		close(ch)
		return ch, err
	}

	go func() {
		defer close(ch)

		writer := NewChannelWriter(ctx, ch)
		defer writer.Stop()

		err := c.service.Start(ctx, project.Name, api.StartOptions{
			Project: project,
			Attach:  &LogCompose{ch: ch},
		})

		if err != nil {
			ch <- StreamMessage{
				Type:    MessageTypeError,
				Content: err.Error(),
				Error:   err,
			}
			return
		}

		ch <- StreamMessage{
			Type:    MessageTypeComplete,
			Content: "Project started successfully",
		}
	}()

	return ch, nil
}

// StopProject 停止项目（返回流式输出 channel）
func (c *Client) StopProject(ctx context.Context, composeFile string) (<-chan StreamMessage, error) {
	ch := make(chan StreamMessage, 100)

	project, err := c.loadProject(ctx, composeFile)
	if err != nil {
		close(ch)
		return ch, err
	}

	go func() {
		defer close(ch)

		writer := NewChannelWriter(ctx, ch)
		defer writer.Stop()

		err := c.service.Stop(ctx, project.Name, api.StopOptions{
			Project: project,
		})

		if err != nil {
			ch <- StreamMessage{
				Type:    MessageTypeError,
				Content: err.Error(),
				Error:   err,
			}
			return
		}

		ch <- StreamMessage{
			Type:    MessageTypeComplete,
			Content: "Project stopped successfully",
		}
	}()

	return ch, nil
}

// RestartProject 重启项目（返回流式输出 channel）
func (c *Client) RestartProject(ctx context.Context, composeFile string) (<-chan StreamMessage, error) {
	ch := make(chan StreamMessage, 100)

	project, err := c.loadProject(ctx, composeFile)
	if err != nil {
		close(ch)
		return ch, err
	}

	go func() {
		defer close(ch)

		writer := NewChannelWriter(ctx, ch)
		defer writer.Stop()

		err := c.service.Restart(ctx, project.Name, api.RestartOptions{
			Project: project,
		})

		if err != nil {
			ch <- StreamMessage{
				Type:    MessageTypeError,
				Content: err.Error(),
				Error:   err,
			}
			return
		}

		ch <- StreamMessage{
			Type:    MessageTypeComplete,
			Content: "Project restarted successfully",
		}
	}()

	return ch, nil
}

// DeleteProject 删除项目（返回流式输出 channel）
func (c *Client) DeleteProject(ctx context.Context, composeFile string, status StackStatus) (<-chan StreamMessage, error) {
	ch := make(chan StreamMessage, 100)

	projectPath := path.Dir(composeFile)

	go func() {
		defer close(ch)

		writer := NewChannelWriter(ctx, ch)
		defer writer.Stop()

		// 如果不是 draft 状态，先执行 docker-compose down
		if status != StatusDraft {
			project, err := c.loadProject(ctx, composeFile)
			if err != nil {
				ch <- StreamMessage{
					Type:    MessageTypeError,
					Content: "Failed to load project: " + err.Error(),
					Error:   err,
				}
				return
			}

			ch <- StreamMessage{
				Type:    MessageTypeLog,
				Content: "Removing Docker resources...\n",
			}

			err = c.service.Down(ctx, project.Name, api.DownOptions{
				Project:       project,
				RemoveOrphans: true,
				Volumes:       true,
				Images:        "all",
			})

			if err != nil {
				ch <- StreamMessage{
					Type:    MessageTypeError,
					Content: "Failed to remove Docker resources: " + err.Error(),
					Error:   err,
				}
				return
			}
		}

		// 删除项目目录和配置文件
		if status == StatusDraft || status == StatusCreatedStack {
			ch <- StreamMessage{
				Type:    MessageTypeLog,
				Content: "Removing project directory...\n",
			}

			if err := os.RemoveAll(projectPath); err != nil {
				ch <- StreamMessage{
					Type:    MessageTypeError,
					Content: "Failed to remove project directory: " + err.Error(),
					Error:   err,
				}
				return
			}
		}

		ch <- StreamMessage{
			Type:    MessageTypeComplete,
			Content: "Project deleted successfully",
		}
	}()

	return ch, nil
}

// CreateProject 创建并启动项目（返回流式输出 channel）
func (c *Client) CreateProject(ctx context.Context, composeFile string, isRunning bool, isBuild bool) (<-chan StreamMessage, error) {
	ch := make(chan StreamMessage, 100)

	project, err := c.loadProject(ctx, composeFile)
	if err != nil {
		close(ch)
		return ch, err
	}

	go func() {
		defer close(ch)

		writer := NewChannelWriter(ctx, ch)
		defer writer.Stop()

		// 创建 Up 选项
		upOptions := api.UpOptions{
			Create: api.CreateOptions{
				Services:             project.ServiceNames(),
				RemoveOrphans:        true,
				Recreate:             api.RecreateNever,
				RecreateDependencies: api.RecreateNever,
			},
			Start: api.StartOptions{
				Project: project,
			},
		}

		if isRunning {
			upOptions.Create.Recreate = api.RecreateForce
		}

		if isBuild {
			upOptions.Create.Build = &api.BuildOptions{
				Progress: "auto",
			}
		}

		err := c.service.Up(ctx, project, upOptions)

		if err != nil {
			ch <- StreamMessage{
				Type:    MessageTypeError,
				Content: err.Error(),
				Error:   err,
			}
			return
		}

		ch <- StreamMessage{
			Type:    MessageTypeComplete,
			Content: "Project created and started successfully",
		}
	}()

	return ch, nil
}

// SaveNewProject 保存新的 Compose 项目（创建目录和 YAML 文件）
func (c *Client) SaveNewProject(ctx context.Context, name string, yamlContent string, force bool) (string, error) {
	appPath := conf.EnvCfg.APP_PATH
	if appPath == "" {
		return "", errors.New("APP_PATH 未设置，无法创建项目")
	}

	// 创建项目目录
	projectPath := filepath.Join(appPath, name)
	composeFile := filepath.Join(projectPath, "docker-compose.yaml")

	// 检查项目是否已存在
	if stat, err := os.Stat(projectPath); err == nil && stat.IsDir() {
		if !force {
			logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
			return "", errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		logger.Logger.Info("项目已存在，将覆盖 docker-compose.yml 文件", zap.String("path", projectPath))
	} else {
		// 创建目录
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			logger.Logger.Error("创建项目目录失败", zap.String("path", projectPath), zap.Error(err))
			return "", errors.New("创建项目目录失败: " + err.Error())
		}
		logger.Logger.Info("创建项目目录成功", zap.String("path", projectPath))
	}

	// 写入 docker-compose.yml 文件
	if err := os.WriteFile(composeFile, []byte(yamlContent), 0644); err != nil {
		logger.Logger.Error("写入 Compose 文件失败", zap.String("file", composeFile), zap.Error(err))
		return "", errors.New("写入 Compose 文件失败: " + err.Error())
	}

	logger.Logger.Info("项目文件保存成功",
		zap.String("name", name),
		zap.String("path", projectPath),
		zap.String("composeFile", composeFile),
		zap.Bool("force", force))

	return composeFile, nil
}

// GetProjectYaml 读取项目的 docker-compose.yaml 文件内容
func (c *Client) GetProjectYaml(composeFile string) (string, error) {
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		logger.Logger.Error("Compose 文件不存在", zap.String("file", composeFile))
		return "", errors.New("compose 文件不存在")
	}

	content, err := os.ReadFile(composeFile)
	if err != nil {
		logger.Logger.Error("读取 Compose 文件失败", zap.String("file", composeFile), zap.Error(err))
		return "", errors.New("读取 Compose 文件失败: " + err.Error())
	}

	logger.Logger.Info("读取 Compose 文件成功", zap.String("file", composeFile))
	return string(content), nil
}

// scanProjects 扫描文件系统中的 Compose 项目
func (c *Client) scanProjects(ctx context.Context) []ComposeProject {
	var projects []ComposeProject
	appPath := conf.EnvCfg.APP_PATH
	if appPath == "" {
		return projects
	}

	err := filepath.Walk(appPath, func(curPath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Logger.Error("扫描项目失败", zap.Error(err))
			return nil
		}

		if c.isComposeFile(info.Name()) {
			logger.Logger.Debug("扫描到项目", zap.String("curPath", curPath), zap.String("name", path.Base(path.Dir(curPath))))
			project := ComposeProject{
				Name:         path.Base(path.Dir(curPath)),
				ComposeFile:  curPath,
				Status:       StatusDraft,
				RunningCount: 0,
				ExitedCount:  0,
				CreatedCount: 0,
			}
			projects = append(projects, project)
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("扫描项目失败", zap.Error(err))
		return projects
	}

	return projects
}

// isComposeFile 检查是否是 compose 文件
func (c *Client) isComposeFile(filename string) bool {
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, cf := range composeFiles {
		if filename == cf {
			return true
		}
	}
	return false
}

// parseStackStatus 解析 stack 状态信息
func (c *Client) parseStackStatus(statusStr string) (status StackStatus, runningCount int, exitedCount int, createdCount int) {
	if statusStr == "" {
		return StatusUnknown, 0, 0, 0
	}

	// 使用正则表达式匹配状态和数量
	re := regexp.MustCompile(`(\w+)(?:\((\d+)\))?`)
	matches := re.FindAllStringSubmatch(statusStr, -1)

	for _, match := range matches {
		statusName := match[1]
		count := 1

		if len(match) > 2 && match[2] != "" {
			if c, err := strconv.Atoi(match[2]); err == nil {
				count = c
			}
		}

		switch statusName {
		case "running":
			runningCount = count
		case "exited":
			exitedCount = count
		case "created":
			createdCount = count
		}
	}

	// 确定总体状态
	if strings.HasPrefix(statusStr, "created") {
		status = StatusCreatedStack
	} else if runningCount > 0 && exitedCount > 0 {
		status = StatusPartial
	} else if runningCount > 0 {
		status = StatusRunning
	} else if exitedCount > 0 {
		status = StatusExited
	} else {
		status = StatusUnknown
	}

	return status, runningCount, exitedCount, createdCount
}

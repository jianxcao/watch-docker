package composecli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker/client"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

type Client struct {
	docker *client.Client
}

func NewClient(docker *client.Client) *Client {
	return &Client{
		docker: docker,
	}
}

// ScanProjects 扫描发现 Compose 项目
func (c *Client) ScanProjects(ctx context.Context) []ComposeProject {
	var projects []ComposeProject
	appPath := conf.EnvCfg.APP_PATH
	if appPath == "" {
		return projects
	}

	const maxDepth = 2 // 最大遍历深度

	err := filepath.Walk(appPath, func(curPath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Logger.Error("扫描项目失败", logger.ZapErr(err))
			return nil // 忽略错误，继续扫描
		}

		// 计算当前路径相对于 appPath 的深度
		relPath, err := filepath.Rel(appPath, curPath)
		if err != nil {
			return nil
		}

		// 计算深度（根目录为 0）
		depth := 0
		if relPath != "." {
			depth = len(strings.Split(relPath, string(os.PathSeparator)))
		}

		// 如果是目录且深度已达到限制，跳过该目录
		if info.IsDir() && depth >= maxDepth {
			return filepath.SkipDir
		}

		// 查找 compose 文件
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
		logger.Logger.Error("扫描项目失败", logger.ZapErr(err))
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

// findComposeFileInDir 在指定目录中查找 compose 文件
// 如果找到返回完整路径，否则返回默认的 docker-compose.yaml 路径
func (c *Client) findComposeFileInDir(dir string) string {
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, cf := range composeFiles {
		fullPath := filepath.Join(dir, cf)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	// 如果没有找到，返回默认的 docker-compose.yaml
	return filepath.Join(dir, "docker-compose.yaml")
}

// parseStatusInfo 解析 oStatus 字符串，提取状态信息
// oStatus 格式示例：
// "exited(2),running(2)" 或 "running" 或 "exited"
func parseStatusInfo(oStatus string) (status StackStatus, runningCount int, exitedCount int, createdCount int) {
	if oStatus == "" {
		return "unknown", 0, 0, 0
	}

	// 使用正则表达式匹配状态和数量
	re := regexp.MustCompile(`(\w+)(?:\((\d+)\))?`)
	matches := re.FindAllStringSubmatch(oStatus, -1)

	for _, match := range matches {
		statusName := match[1]
		count := 1 // 默认为1，如果没有括号中的数字

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

	// 确定总体状态：如果有运行的容器，状态为 running，否则根据退出容器确定
	if strings.HasPrefix(oStatus, "created") {
		status = StatusCreatedStack
	} else if runningCount > 0 && exitedCount > 0 {
		status = StatusPartial // 部分运行
	} else if runningCount > 0 {
		status = StatusRunning
	} else if exitedCount > 0 {
		status = StatusExited
	} else {
		status = StatusUnknown
	}

	return status, runningCount, exitedCount, createdCount
}

type ListOutput struct {
	Status      string `json:"status"`
	Name        string `json:"name"`
	ConfigFiles string `json:"ConfigFiles"`
}

// ScanProjects 扫描发现 Compose 项目
func (c *Client) ListProjects(ctx context.Context) ([]ComposeProject, error) {
	projects := c.ScanProjects(ctx)
	mapScanProjects := make(map[string]ComposeProject)
	for _, project := range projects {
		mapScanProjects[project.Name] = project
	}
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      ".",
		Args:          []string{"ls", "-a", "--format", "json"},
		OperationName: "start project",
		NeedOutput:    true,
	})
	output := string(res.Output)
	var listOutput []ListOutput
	err := json.Unmarshal([]byte(output), &listOutput)
	if err != nil {
		logger.Logger.Error("解析项目列表失败", zap.String("output", output), logger.ZapErr(err))
		return nil, err
	}
	for _, item := range listOutput {
		oStatus := item.Status
		status, runningCount, exitedCount, createdCount := parseStatusInfo(oStatus)
		mapScanProjects[item.Name] = ComposeProject{
			Name:         item.Name,
			ComposeFile:  item.ConfigFiles,
			Status:       status,
			RunningCount: runningCount,
			ExitedCount:  exitedCount,
			CreatedCount: createdCount,
		}
	}
	result := make([]ComposeProject, 0, len(mapScanProjects))
	for _, project := range mapScanProjects {
		result = append(result, project)
	}
	logger.Logger.Info("list projects", zap.String("output", output))
	return result, nil
}

// StartProject 使用 Docker API 启动项目
func (c *Client) StartProject(ctx context.Context, composeFile string) error {
	projectPath := path.Dir(composeFile)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"start"},
		OperationName: "start project",
		NeedOutput:    true,
	})
	logger.Logger.Info("启动APP", zap.String("output", string(res.Output)))
	return res.Error
}

// StopProject 停止项目中的所有服务
func (c *Client) StopProject(ctx context.Context, composeFile string) error {
	projectPath := path.Dir(composeFile)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"stop"},
		OperationName: "stop project",
		NeedOutput:    true,
	})
	logger.Logger.Info("停止APP", zap.String("output", string(res.Output)))
	return res.Error
}

// RestartProject 重新创建项目
func (c *Client) RestartProject(ctx context.Context, composeFile string) error {
	projectPath := path.Dir(composeFile)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"restart"},
		OperationName: "restart project",
		NeedOutput:    true,
	})
	logger.Logger.Info("重启APP", zap.String("output", string(res.Output)))
	return res.Error
}

// PullProject 拉取项目镜像
func (c *Client) PullProject(ctx context.Context, composeFile string) error {
	projectPath := path.Dir(composeFile)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"pull"},
		OperationName: "pull project",
		NeedOutput:    true,
	})
	logger.Logger.Info("拉取镜像", zap.String("output", string(res.Output)))
	return res.Error
}

// DeleteProject 删除项目及其所有资源
// 如果是 draft 状态，直接删除配置文件和目录
// 如果是其他状态，先执行 docker-compose down，然后删除配置文件和目录
func (c *Client) DeleteProject(ctx context.Context, composeFile string, status StackStatus) error {
	projectPath := path.Dir(composeFile)

	// 如果不是 draft 状态，先执行 docker-compose down 清理容器、网络和卷
	if status != StatusDraft {
		res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
			ExecPath:      projectPath,
			Args:          []string{"down", "--volumes", "--remove-orphans"},
			OperationName: "delete project",
			NeedOutput:    true,
		})
		logger.Logger.Info("删除APP（Docker资源）", zap.String("output", string(res.Output)))
		if res.Error != nil {
			return res.Error
		}
	}

	// 删除项目目录和配置文件
	if status == StatusDraft || status == StatusCreatedStack {
		if err := os.RemoveAll(projectPath); err != nil {
			logger.Logger.Error("删除项目目录失败", zap.String("path", projectPath), logger.ZapErr(err))
			return errors.New("删除项目目录失败: " + err.Error())
		}
	}

	logger.Logger.Info("删除项目成功",
		zap.String("projectPath", projectPath),
		zap.String("status", string(status)))

	return nil
}

func (c *Client) CreateProject(ctx context.Context, composeFile string, isRuning bool, isBuild bool) error {
	projectPath := path.Dir(composeFile)
	args := []string{"up", "-d", "--remove-orphans"}
	if isRuning {
		args = append(args, "--force-recreate")
	}
	if isBuild {
		args = append(args, "--build")
	}
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          args,
		OperationName: "delete project",
		NeedOutput:    true,
	})
	logger.Logger.Info("创建APP", zap.String("output", string(res.Output)))
	return res.Error
}

// SaveNewProject 保存新的 Compose 项目（创建目录和 YAML 文件）
func (c *Client) SaveNewProject(ctx context.Context, name string, yamlContent string, force bool) (string, error) {
	appPath := conf.EnvCfg.APP_PATH
	if appPath == "" {
		return "", errors.New("APP_PATH 未设置，无法创建项目")
	}

	// 创建项目目录
	projectPath := filepath.Join(appPath, name)

	// 检查项目是否已存在
	if stat, err := os.Stat(projectPath); err == nil && stat.IsDir() {
		// 项目目录已存在
		if !force {
			logger.Logger.Warn("项目已存在，需要 force=true 才能覆盖", zap.String("path", projectPath))
			return "", errors.New("项目已存在，如需覆盖请使用强制模式")
		}
		logger.Logger.Info("项目已存在，将覆盖 compose 文件", zap.String("path", projectPath))
	} else {
		// 项目目录不存在，创建目录
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			logger.Logger.Error("创建项目目录失败", zap.String("path", projectPath), logger.ZapErr(err))
			return "", errors.New("创建项目目录失败: " + err.Error())
		}
		logger.Logger.Info("创建项目目录成功", zap.String("path", projectPath))
	}

	// 查找目录中是否已存在 compose 文件，如果存在则使用已有的文件名
	composeFile := c.findComposeFileInDir(projectPath)

	// 写入 compose 文件（如果已存在会被覆盖）
	if err := os.WriteFile(composeFile, []byte(yamlContent), 0644); err != nil {
		logger.Logger.Error("写入 Compose 文件失败", zap.String("file", composeFile), logger.ZapErr(err))
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
func (c *Client) GetProjectYaml(file string) (string, error) {

	dir := path.Dir(file)
	composeFile := c.findComposeFileInDir(dir)
	// 检查文件是否存在
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		logger.Logger.Error("Compose 文件不存在", zap.String("file", composeFile))
		return "", errors.New("compose 文件不存在")
	}

	// 读取文件内容
	content, err := os.ReadFile(composeFile)
	if err != nil {
		logger.Logger.Error("读取 Compose 文件失败", zap.String("file", composeFile), logger.ZapErr(err))
		return "", errors.New("读取 Compose 文件失败: " + err.Error())
	}

	logger.Logger.Info("读取 Compose 文件成功", zap.String("file", composeFile))
	return string(content), nil
}

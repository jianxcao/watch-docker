package composecli

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker/client"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

type Client struct {
	docker       *client.Client
	projectPaths []string
}

func NewClient(docker *client.Client, projectPaths []string) *Client {
	return &Client{
		docker:       docker,
		projectPaths: projectPaths,
	}
}

// ScanProjects 扫描发现 Compose 项目
func (c *Client) ScanProjects(ctx context.Context) []ComposeProject {
	var projects []ComposeProject
	if len(c.projectPaths) == 0 {
		return projects
	}
	for _, basePath := range c.projectPaths {
		err := filepath.Walk(basePath, func(curPath string, info os.FileInfo, err error) error {
			if err != nil {
				logger.Logger.Error("扫描项目失败", logger.ZapErr(err))
				return nil // 忽略错误，继续扫描
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
			return nil
		}
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

// DeleteProject 删除项目及其所有资源
func (c *Client) DeleteProject(ctx context.Context, composeFile string) error {
	projectPath := path.Dir(composeFile)
	res := ExecuteDockerComposeCommand(ctx, ExecDockerComposeOptions{
		ExecPath:      projectPath,
		Args:          []string{"down", "--volumes", "--remove-orphans"},
		OperationName: "delete project",
		NeedOutput:    true,
	})
	logger.Logger.Info("删除APP", zap.String("output", string(res.Output)))
	return res.Error
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

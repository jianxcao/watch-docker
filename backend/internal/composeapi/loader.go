package composeapi

import (
	"context"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// loadProject 从 compose 文件加载项目
func (c *Client) loadProject(ctx context.Context, composeFile string) (*types.Project, error) {
	// 获取项目目录和文件名
	projectDir := filepath.Dir(composeFile)
	composeFileName := filepath.Base(composeFile)

	logger.Logger.Debug("loading project",
		zap.String("composeFile", composeFile),
		zap.String("projectDir", projectDir))

	// 创建 ProjectOptions
	options := &cli.ProjectOptions{
		Name:        filepath.Base(projectDir), // 使用目录名作为项目名
		WorkingDir:  projectDir,
		ConfigPaths: []string{composeFileName},
	}

	// 加载项目配置
	project, err := cli.ProjectFromOptions(ctx, options)
	if err != nil {
		logger.Logger.Error("failed to load project", zap.String("composeFile", composeFile), zap.Error(err))
		return nil, err
	}

	logger.Logger.Info("project loaded successfully",
		zap.String("name", project.Name),
		zap.Int("services", len(project.Services)))

	return project, nil
}

// listAllProjects 列出系统中所有的 compose 项目
func (c *Client) listAllProjects(ctx context.Context) ([]api.Stack, error) {
	logger.Logger.Debug("listing all compose projects")

	// 使用 compose service 列出项目
	stacks, err := c.service.List(ctx, api.ListOptions{
		All: true,
	})
	if err != nil {
		logger.Logger.Error("failed to list projects", zap.Error(err))
		return nil, err
	}

	logger.Logger.Info("projects listed successfully", zap.Int("count", len(stacks)))
	return stacks, nil
}

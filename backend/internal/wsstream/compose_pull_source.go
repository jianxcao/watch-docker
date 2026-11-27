package wsstream

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposePullSource 实现了 Compose 拉取镜像的流式数据源
type ComposePullSource struct {
	projectPath string
	projectName string
	argsPrefix  []string
	key         string
	mu          sync.Mutex
	reader      *ByteStreamReader
}

// NewComposePullSource 创建新的 Compose 拉取镜像数据源
func NewComposePullSource(projectPath, projectName string, argsPrefix ...string) *ComposePullSource {
	return &ComposePullSource{
		projectPath: projectPath,
		projectName: projectName,
		argsPrefix:  slices.Clone(argsPrefix),
		key:         fmt.Sprintf("compose-pull-%s", projectName),
	}
}

// Start 启动 Compose 拉取镜像流
func (s *ComposePullSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动 Compose 拉取镜像流",
		zap.String("projectPath", s.projectPath),
		zap.String("projectName", s.projectName))

	// 执行 docker compose pull 命令
	pullArgs := append(slices.Clone(s.argsPrefix), "--ansi", "always", "pull")
	result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      s.projectPath,
		Args:          pullArgs,
		OperationName: "compose pull",
	})

	if result.Error != nil {
		logger.Logger.Error("启动 Compose 拉取镜像流失败",
			zap.String("projectName", s.projectName),
			zap.Error(result.Error))
		return nil, result.Error
	}
	// 使用 ByteStreamReader 直接流式传输日志
	reader := NewByteStreamReader(result.Reader)
	s.mu.Lock()
	s.reader = reader
	s.mu.Unlock()
	return reader, nil
}

// Stop 停止 Compose 拉取镜像流
func (s *ComposePullSource) Stop() error {
	logger.Logger.Info("停止 Compose 拉取镜像流",
		zap.String("projectName", s.projectName))

	s.mu.Lock()
	reader := s.reader
	s.reader = nil
	s.mu.Unlock()

	if reader != nil {
		reader.Close()
	}
	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ComposePullSource) GetKey() string {
	return s.key
}

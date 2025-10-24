package wsstream

import (
	"context"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposeLogsSource 实现了 Compose 日志的流式数据源
type ComposeLogsSource struct {
	projectPath string
	projectName string
	key         string
	reader      *ByteStreamReader
}

// NewComposeLogsSource 创建新的 Compose 日志数据源
func NewComposeLogsSource(projectPath, projectName string) *ComposeLogsSource {
	return &ComposeLogsSource{
		projectPath: projectPath,
		projectName: projectName,
		key:         projectName, // 使用 projectName 作为唯一标识
	}
}

// Start 启动 Compose 日志流
func (s *ComposeLogsSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动 Compose 日志流",
		zap.String("projectPath", s.projectPath),
		zap.String("projectName", s.projectName))

	// 执行 docker compose logs 命令
	result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      s.projectPath,
		Args:          []string{"--ansi", "always", "logs", "--follow", "--timestamps", "--tail=500"},
		OperationName: "compose logs",
	})

	if result.Error != nil {
		logger.Logger.Error("启动 Compose 日志流失败",
			zap.String("projectName", s.projectName),
			zap.Error(result.Error))
		return nil, result.Error
	}
	// 使用 ByteStreamReader 直接流式传输日志
	s.reader = NewByteStreamReader(result.Reader)
	return s.reader, nil
}

// Stop 停止 Compose 日志流
func (s *ComposeLogsSource) Stop() error {
	logger.Logger.Info("停止 Compose 日志流",
		zap.String("projectName", s.projectName))

	if s.reader != nil {
		s.reader.Close()
		s.reader = nil
	}
	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ComposeLogsSource) GetKey() string {
	return s.key
}

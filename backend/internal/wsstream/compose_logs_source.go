package wsstream

import (
	"context"
	"os"
	"sync"

	"github.com/jianxcao/watch-docker/backend/internal/composecli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ComposeLogsSource 实现了 Compose 日志的流式数据源
type ComposeLogsSource struct {
	projectPath string
	projectName string
	argsPrefix  []string
	key         string
	mu          sync.Mutex
	reader      *ByteStreamReader
}

// NewComposeLogsSource 创建新的 Compose 日志数据源
func NewComposeLogsSource(projectPath, projectName string, argsPrefix ...string) *ComposeLogsSource {
	return NewComposeLogsSourceWithKey("", projectPath, projectName, argsPrefix...)
}

// NewComposeLogsSourceWithKey 创建带自定义流标识的 Compose 日志数据源
func NewComposeLogsSourceWithKey(key, projectPath, projectName string, argsPrefix ...string) *ComposeLogsSource {
	if key == "" {
		key = "compose-logs:" + projectName
	}
	return &ComposeLogsSource{
		projectPath: projectPath,
		projectName: projectName,
		argsPrefix:  append([]string(nil), argsPrefix...),
		key:         key,
	}
}

// Start 启动 Compose 日志流
func (s *ComposeLogsSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动 Compose 日志流",
		zap.String("projectPath", s.projectPath),
		zap.String("projectName", s.projectName))

	args := []string{"--ansi", "always", "logs", "--follow", "--timestamps", "--tail=500"}
	prefix := s.argsPrefix
	if len(prefix) == 0 && s.projectPath == "" {
		prefix = []string{"--env-file", os.DevNull, "-p", s.projectName}
	}
	if len(prefix) > 0 {
		args = append(append([]string(nil), prefix...), args...)
	}

	// 执行 docker compose logs 命令
	result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
		ExecPath:      s.projectPath,
		Args:          args,
		OperationName: "compose logs",
	})

	if result.Error != nil {
		logger.Logger.Error("启动 Compose 日志流失败",
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

// Stop 停止 Compose 日志流
func (s *ComposeLogsSource) Stop() error {
	logger.Logger.Info("停止 Compose 日志流",
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
func (s *ComposeLogsSource) GetKey() string {
	return s.key
}

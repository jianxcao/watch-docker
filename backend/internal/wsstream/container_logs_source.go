package wsstream

import (
	"context"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ContainerLogsSource 实现了容器日志的流式数据源
type ContainerLogsSource struct {
	containerID   string
	containerName string
	dockerClient  *dockercli.Client
	key           string
	reader        *ByteStreamReader
}

// ContainerLogsSourceOptions 容器日志数据源选项
type ContainerLogsSourceOptions struct {
	ContainerID   string
	ContainerName string
	Key           string
	DockerClient  *dockercli.Client
}

// NewContainerLogsSource 创建新的容器日志数据源
func NewContainerLogsSource(opts ContainerLogsSourceOptions) *ContainerLogsSource {
	return &ContainerLogsSource{
		containerID:   opts.ContainerID,
		containerName: opts.ContainerName,
		dockerClient:  opts.DockerClient,
		key:           opts.Key, // 使用 containerID 作为唯一标识
	}
}

// Start 启动容器日志流
func (s *ContainerLogsSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动容器日志流",
		zap.String("containerID", s.containerID),
		zap.String("containerName", s.containerName))

	// 获取容器日志流
	logReader, err := s.dockerClient.ContainerLogs(
		ctx,
		s.containerID,
		"",    // since: 从开始获取
		true,  // timestamps: 显示时间戳
		"500", // tail: 最后500行
		true,  // follow: 持续跟踪
	)

	if err != nil {
		logger.Logger.Error("启动容器日志流失败",
			zap.String("containerID", s.containerID),
			zap.String("containerName", s.containerName),
			zap.Error(err))
		return nil, err
	}

	// 使用 ByteStreamReader 直接流式传输日志
	s.reader = NewByteStreamReader(logReader)
	return s.reader, nil
}

// Stop 停止容器日志流
func (s *ContainerLogsSource) Stop() error {
	logger.Logger.Info("停止容器日志流",
		zap.String("containerID", s.containerID),
		zap.String("containerName", s.containerName))

	if s.reader != nil {
		s.reader.Close()
		s.reader = nil
	}
	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ContainerLogsSource) GetKey() string {
	return s.key
}

package wsstream

import (
	"context"
	"io"
	"sync"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/pkg/stdcopy"
	"go.uber.org/zap"
)

// ContainerLogsSource 实现了容器日志的流式数据源
type ContainerLogsSource struct {
	containerID   string
	containerName string
	dockerClient  *dockercli.Client
	key           string

	mu        sync.Mutex
	reader    *ByteStreamReader
	rawStream io.ReadCloser
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
		key:           opts.Key,
	}
}

// Start 启动容器日志流，自动判断容器 TTY 模式并在非 TTY 时使用 stdcopy 剥离 8 字节复用流头部
func (s *ContainerLogsSource) Start(ctx context.Context) (StreamReader[[]byte], error) {
	logger.Logger.Info("启动容器日志流",
		zap.String("containerID", s.containerID),
		zap.String("containerName", s.containerName))

	isTTY := false
	if info, err := s.dockerClient.InspectContainer(ctx, s.containerID); err == nil && info.Config != nil {
		isTTY = info.Config.Tty
	}

	logReader, err := s.dockerClient.ContainerLogs(
		ctx,
		s.containerID,
		"",
		true,
		"500",
		true,
	)
	if err != nil {
		logger.Logger.Error("启动容器日志流失败",
			zap.String("containerID", s.containerID),
			zap.String("containerName", s.containerName),
			zap.Error(err))
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.rawStream = logReader

	if isTTY {
		s.reader = NewByteStreamReader(logReader)
		return s.reader, nil
	}

	pr, pw := io.Pipe()
	go func() {
		defer func() {
			_ = pw.Close()
			_ = logReader.Close()
		}()
		if _, err := stdcopy.StdCopy(pw, pw, logReader); err != nil && ctx.Err() == nil && err != io.EOF {
			logger.Logger.Debug("解复用容器日志流结束",
				zap.String("containerID", s.containerID),
				zap.Error(err))
		}
	}()

	s.reader = NewByteStreamReader(pr)
	return s.reader, nil
}

// Stop 停止容器日志流
func (s *ContainerLogsSource) Stop() error {
	logger.Logger.Info("停止容器日志流",
		zap.String("containerID", s.containerID),
		zap.String("containerName", s.containerName))

	s.mu.Lock()
	reader := s.reader
	raw := s.rawStream
	s.reader = nil
	s.rawStream = nil
	s.mu.Unlock()

	if raw != nil {
		_ = raw.Close()
	}
	if reader != nil {
		_ = reader.Close()
	}
	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ContainerLogsSource) GetKey() string {
	return s.key
}

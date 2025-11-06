package wsstream

import (
	"context"
	"encoding/json"
	"io"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ContainerDetailStatsSource 实现了单个容器详细统计信息的流式数据源
type ContainerDetailStatsSource struct {
	containerID  string
	dockerClient *dockercli.Client
	key          string
	reader       *ChannelStreamReader[string]
	cancel       context.CancelFunc
}

// ContainerDetailStatsSourceOptions 容器详细统计数据源选项
type ContainerDetailStatsSourceOptions struct {
	ContainerID  string
	DockerClient *dockercli.Client
	Key          string
}

// NewContainerDetailStatsSource 创建新的容器详细统计数据源
func NewContainerDetailStatsSource(opts ContainerDetailStatsSourceOptions) *ContainerDetailStatsSource {
	return &ContainerDetailStatsSource{
		containerID:  opts.ContainerID,
		dockerClient: opts.DockerClient,
		key:          opts.Key,
	}
}

// Start 启动容器详细统计流
func (s *ContainerDetailStatsSource) Start(ctx context.Context) (StreamReader[string], error) {
	logger.Logger.Info("启动容器详细统计流", zap.String("containerID", s.containerID))

	// 创建可取消的上下文
	streamCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// 获取容器统计流（stream=true 表示持续获取）
	statsReader, err := s.dockerClient.ContainerStats(streamCtx, s.containerID, true)
	if err != nil {
		logger.Logger.Error("启动容器详细统计流失败",
			zap.String("containerID", s.containerID),
			zap.Error(err))
		cancel()
		return nil, err
	}

	// 创建消息通道
	messageChan := make(chan string, 10)

	// 创建 ChannelStreamReader
	s.reader = NewChannelStreamReader(messageChan)

	// 在后台goroutine中读取和解析统计数据
	go s.readStatsStream(streamCtx, statsReader.Body, messageChan)

	return s.reader, nil
}

// readStatsStream 读取并解析统计数据流
func (s *ContainerDetailStatsSource) readStatsStream(ctx context.Context, reader io.ReadCloser, messageChan chan string) {
	defer close(messageChan)
	defer reader.Close()

	decoder := json.NewDecoder(reader)

	for {
		select {
		case <-ctx.Done():
			logger.Logger.Info("容器详细统计流被取消", zap.String("containerID", s.containerID))
			return
		default:
			// 解码一条统计数据
			var stats map[string]interface{}
			if err := decoder.Decode(&stats); err != nil {
				if err == io.EOF || ctx.Err() != nil {
					logger.Logger.Info("容器详细统计流结束", zap.String("containerID", s.containerID))
					return
				}
				logger.Logger.Error("解码容器统计数据失败",
					zap.String("containerID", s.containerID),
					zap.Error(err))
				return
			}

			// 将统计数据转换为 JSON 字符串
			statsJSON, err := json.Marshal(stats)
			if err != nil {
				logger.Logger.Error("序列化容器统计数据失败",
					zap.String("containerID", s.containerID),
					zap.Error(err))
				continue
			}

			// 发送到通道
			select {
			case messageChan <- string(statsJSON):
			case <-ctx.Done():
				return
			}
		}
	}
}

// Stop 停止容器详细统计流
func (s *ContainerDetailStatsSource) Stop() error {
	logger.Logger.Info("停止容器详细统计流", zap.String("containerID", s.containerID))

	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 关闭 reader
	if s.reader != nil {
		s.reader.Close()
		s.reader = nil
	}

	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ContainerDetailStatsSource) GetKey() string {
	return s.key
}

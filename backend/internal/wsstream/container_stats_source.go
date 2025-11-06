package wsstream

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"go.uber.org/zap"
)

// ContainerStatsSource 实现了容器统计数据的流式数据源
// 定期推送容器状态和统计信息
type ContainerStatsSource struct {
	key      string
	docker   *dockercli.Client
	scanner  *scanner.Scanner
	interval time.Duration // 推送间隔
}

// ContainerStatsSourceOptions 容器统计数据源选项
type ContainerStatsSourceOptions struct {
	Docker   *dockercli.Client
	Scanner  *scanner.Scanner
	Interval time.Duration // 推送间隔，默认 2 秒
}

// NewContainerStatsSource 创建新的容器统计数据源
func NewContainerStatsSource(opts ContainerStatsSourceOptions) *ContainerStatsSource {
	interval := opts.Interval
	if interval == 0 {
		interval = 2 * time.Second // 默认 2 秒
	}

	return &ContainerStatsSource{
		key:      "container-stats",
		docker:   opts.Docker,
		scanner:  opts.Scanner,
		interval: interval,
	}
}

// Start 启动容器统计数据流
func (s *ContainerStatsSource) Start(ctx context.Context) (StreamReader[string], error) {
	logger.Logger.Debug("启动容器统计数据流")

	// 创建 channel 用于发送完整的 JSON 消息
	messageChan := make(chan string, 10)

	// 通知 Docker 客户端有新连接
	s.docker.AddStatsConnection(ctx)

	// 启动定期推送 goroutine
	go s.pushStats(ctx, messageChan)

	// 使用 ChannelStreamReader 保证每条消息的完整性
	return NewChannelStreamReader(messageChan), nil
}

// pushStats 定期推送容器统计数据
func (s *ContainerStatsSource) pushStats(ctx context.Context, messageChan chan string) {
	defer func() {
		close(messageChan)
		// 通知 Docker 客户端连接已断开
		s.docker.RemoveStatsConnection()
		logger.Logger.Debug("容器统计数据流已停止")
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// 立即发送一次数据
	s.sendStats(ctx, messageChan)

	// 定期发送
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Debug("容器统计数据流被取消")
			return
		case <-ticker.C:
			if err := s.sendStats(ctx, messageChan); err != nil {
				logger.Logger.Error("发送容器统计数据失败", zap.Error(err))
				return
			}
		}
	}
}

// sendStats 发送一次容器统计数据
func (s *ContainerStatsSource) sendStats(ctx context.Context, messageChan chan string) error {
	// 获取配置
	cfg := config.Get()

	// 使用scanner获取完整的容器状态信息
	containerStatuses, err := s.scanner.ScanOnce(ctx, cfg.Docker.IncludeStopped, cfg.Scan.Concurrency, true, false)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Error("获取容器状态失败", zap.Error(err))
		return err
	}

	if len(containerStatuses) == 0 {
		// 发送空数据
		response := map[string]interface{}{
			"type": "containers",
			"data": map[string]interface{}{
				"containers": []scanner.ContainerStatus{},
			},
			"timestamp": time.Now().Unix(),
		}

		message, err := json.Marshal(response)
		if err != nil {
			logger.Logger.Error("序列化空容器数据失败", zap.Error(err))
			return err
		}

		// 转换为 string 并添加换行符
		messageStr := string(message)

		// 发送到 channel
		select {
		case messageChan <- messageStr:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 收集运行中容器的ID
	runningContainerIDs := make([]string, 0)
	for _, cs := range containerStatuses {
		if cs.Running {
			runningContainerIDs = append(runningContainerIDs, cs.ID)
		}
	}

	// 获取运行中容器的统计数据
	var statsMap map[string]*dockercli.ContainerStats
	if len(runningContainerIDs) > 0 {
		statsMap, err = s.docker.GetContainersStats(ctx, runningContainerIDs)
		if err != nil {
			logger.Logger.Error("获取容器统计失败", zap.Error(err))
			// 即使获取统计失败，也要发送容器状态信息
			statsMap = make(map[string]*dockercli.ContainerStats)
		}
	} else {
		statsMap = make(map[string]*dockercli.ContainerStats)
	}

	// 将统计数据合并到容器状态中
	for i := range containerStatuses {
		// 添加统计数据（如果容器正在运行且有统计数据）
		if containerStatuses[i].Running && statsMap[containerStatuses[i].ID] != nil {
			containerStatuses[i].Stats = statsMap[containerStatuses[i].ID]
		}
	}

	// 构建响应数据
	response := map[string]interface{}{
		"type": "containers",
		"data": map[string]interface{}{
			"containers": containerStatuses,
		},
		"timestamp": time.Now().Unix(),
	}

	// 序列化为 JSON
	message, err := json.Marshal(response)
	if err != nil {
		logger.Logger.Error("序列化容器数据失败", zap.Error(err))
		return err
	}

	// 转换为 string 并添加换行符
	// 这样前端可以按行解析完整的 JSON 消息
	messageStr := string(message)

	// 发送到 channel
	select {
	case messageChan <- messageStr:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop 停止容器统计数据流
func (s *ContainerStatsSource) Stop() error {
	logger.Logger.Debug("停止容器统计数据流")
	// 由于使用 channel，关闭 reader 会触发 pushStats goroutine 停止
	return nil
}

// GetKey 获取数据源的唯一标识
func (s *ContainerStatsSource) GetKey() string {
	return s.key
}

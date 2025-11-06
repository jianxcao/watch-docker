package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// ContainerStats 容器资源统计信息
type ContainerStats struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpuPercent"`    // CPU使用率，0-100%
	MemoryUsage   uint64  `json:"memoryUsage"`   // 内存使用量（字节）
	MemoryLimit   uint64  `json:"memoryLimit"`   // 内存限制（字节）
	MemoryPercent float64 `json:"memoryPercent"` // 内存使用率，0-100%
	NetworkRxRate uint64  `json:"networkRxRate"` // 网络接收速率（字节/秒）
	NetworkTxRate uint64  `json:"networkTxRate"` // 网络发送速率（字节/秒）
	NetworkRx     uint64  `json:"networkRx"`     // 总接收字节数
	NetworkTx     uint64  `json:"networkTx"`     // 总发送字节数
	BlockRead     uint64  `json:"blockRead"`     // 块设备读取（字节）
	BlockWrite    uint64  `json:"blockWrite"`    // 块设备写入（字节）
	PidsCurrent   uint64  `json:"pidsCurrent"`
	PidsLimit     uint64  `json:"pidsLimit"`
}

// StatsManager 容器统计管理器
type StatsManager struct {
	dockerClient  DockerClientInterface               // Docker客户端接口
	statsCache    map[string]*ContainerStats          // 计算后的统计数据缓存
	rawStatsCache map[string]*container.StatsResponse // 原始统计数据缓存（用于计算差值）
	statsMutex    sync.RWMutex                        // 保护统计数据的读写锁

	// 配置选项
	maxConcurrency int           // 最大并发数
	collectTimeout time.Duration // 单个容器采集超时时间

	// 后台任务管理
	statsTimer *time.Timer   // 定时器
	stopChan   chan struct{} // 停止信号
	isRunning  bool          // 任务运行状态

	// WebSocket 连接管理
	connectionCount int32 // 当前连接数
}

// StatsManagerConfig 统计管理器配置
type StatsManagerConfig struct {
	MaxConcurrency int           // 最大并发数，默认10
	CollectTimeout time.Duration // 单个容器采集超时时间，默认3秒
}

// DockerClientInterface Docker客户端接口，用于解耦
type DockerClientInterface interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStatsOneShot(ctx context.Context, containerID string) (container.StatsResponseReader, error)
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(dockerClient DockerClientInterface) *StatsManager {
	return NewStatsManagerWithConfig(dockerClient, StatsManagerConfig{
		MaxConcurrency: 10,
		CollectTimeout: 3 * time.Second,
	})
}

// NewStatsManagerWithConfig 使用自定义配置创建统计管理器
func NewStatsManagerWithConfig(dockerClient DockerClientInterface, config StatsManagerConfig) *StatsManager {
	// 设置默认值
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}
	if config.CollectTimeout <= 0 {
		config.CollectTimeout = 3 * time.Second
	}

	return &StatsManager{
		dockerClient:   dockerClient,
		statsCache:     make(map[string]*ContainerStats),
		rawStatsCache:  make(map[string]*container.StatsResponse),
		maxConcurrency: config.MaxConcurrency,
		collectTimeout: config.CollectTimeout,
		stopChan:       make(chan struct{}),
		isRunning:      false,
	}
}

// StartMonitoring 启动后台统计监控
func (sm *StatsManager) StartMonitoring(ctx context.Context) {
	sm.statsMutex.Lock()
	if sm.isRunning {
		sm.statsMutex.Unlock()
		return
	}
	sm.isRunning = true
	sm.statsMutex.Unlock()

	go sm.statsMonitoringLoop(ctx)
}

// StopMonitoring 停止后台统计监控
func (sm *StatsManager) StopMonitoring() {
	sm.statsMutex.Lock()
	defer sm.statsMutex.Unlock()

	if !sm.isRunning {
		return
	}

	sm.isRunning = false
	if sm.statsTimer != nil {
		sm.statsTimer.Stop()
	}
	close(sm.stopChan)

	// 重新初始化停止信号
	sm.stopChan = make(chan struct{})
}

// GetContainerStats 获取容器资源统计信息（从缓存读取）
func (sm *StatsManager) GetContainerStats(ctx context.Context, id string) *ContainerStats {
	sm.statsMutex.RLock()
	defer sm.statsMutex.RUnlock()

	// 从缓存中获取统计数据
	if stats, exists := sm.statsCache[id]; exists {
		// 返回数据的副本，避免外部修改
		return &ContainerStats{
			ID:            stats.ID,
			Name:          stats.Name,
			CPUPercent:    stats.CPUPercent,
			MemoryUsage:   stats.MemoryUsage,
			MemoryLimit:   stats.MemoryLimit,
			MemoryPercent: stats.MemoryPercent,
			NetworkRxRate: stats.NetworkRxRate,
			NetworkTxRate: stats.NetworkTxRate,
			NetworkRx:     stats.NetworkRx,
			NetworkTx:     stats.NetworkTx,
			BlockRead:     stats.BlockRead,
			BlockWrite:    stats.BlockWrite,
			PidsCurrent:   stats.PidsCurrent,
			PidsLimit:     stats.PidsLimit,
		}
	}

	// 如果缓存中没有数据，返回默认的零值统计
	name := id
	if len(id) > 12 {
		name = id[:12]
	}

	return &ContainerStats{
		ID:            id,
		Name:          name,
		CPUPercent:    0,
		MemoryUsage:   0,
		MemoryLimit:   0,
		MemoryPercent: 0,
		NetworkRxRate: 0,
		NetworkTxRate: 0,
		NetworkRx:     0,
		NetworkTx:     0,
		BlockRead:     0,
		BlockWrite:    0,
		PidsCurrent:   0,
		PidsLimit:     0,
	}
}

// GetContainersStats 批量获取多个容器的资源统计信息（从缓存读取）
func (sm *StatsManager) GetContainersStats(ctx context.Context, containerIDs []string) (map[string]*ContainerStats, error) {
	statsMap := make(map[string]*ContainerStats)

	if len(containerIDs) == 0 {
		return statsMap, nil
	}

	sm.statsMutex.RLock()
	defer sm.statsMutex.RUnlock()

	// 从缓存中批量获取统计信息
	for _, containerID := range containerIDs {
		statsMap[containerID] = sm.GetContainerStats(ctx, containerID)
	}
	return statsMap, nil
}

// statsMonitoringLoop 后台统计监控循环
func (sm *StatsManager) statsMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Logger.Debug("信息统计监控停止")
			return
		case <-sm.stopChan:
			logger.Logger.Debug("信息统计监控停止")
			return
		case <-ticker.C:
			sm.collectAllContainerStats(ctx)
		}
	}
}

// collectAllContainerStats 收集所有运行中容器的统计信息
func (sm *StatsManager) collectAllContainerStats(ctx context.Context) {
	// 获取所有运行中的容器
	containers, err := sm.dockerClient.ContainerList(ctx, container.ListOptions{})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Error("获取运行中容器失败", zap.Error(err))
		return
	}

	// 创建当前容器ID集合
	currentContainerIDs := make(map[string]bool)
	for _, containerInfo := range containers {
		currentContainerIDs[containerInfo.ID] = true
	}

	// 使用信号量控制并发数，避免过多并发请求
	semaphore := make(chan struct{}, sm.maxConcurrency)

	// 并发收集统计信息
	var wg sync.WaitGroup
	for _, containerInfo := range containers {
		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 为每个容器创建独立的超时上下文，避免阻塞
			containerCtx, cancel := context.WithTimeout(ctx, sm.collectTimeout)
			defer cancel()

			sm.collectSingleContainerStats(containerCtx, containerID)
		}(containerInfo.ID)
	}

	wg.Wait()

	// 清理不再存在的容器统计数据
	sm.cleanupStaleStats(currentContainerIDs)
}

// collectSingleContainerStats 收集单个容器的统计信息
func (sm *StatsManager) collectSingleContainerStats(ctx context.Context, containerID string) {
	// logger.Logger.Info("collectSingleContainerStats", zap.String("containerID", containerID))
	// 使用defer recover避免单个容器的panic影响整体
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("收集容器统计信息失败", zap.String("containerID", containerID[:12]), zap.Any("error", r))
		}
	}()

	// 获取当前统计数据
	stats, err := sm.dockerClient.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		// 容器可能已停止或删除，这是正常情况，降低日志级别
		if ctx.Err() == context.DeadlineExceeded {
			logger.Logger.Error("获取容器统计信息超时", zap.String("containerID", containerID[:12]))
		} else {
			logger.Logger.Error("获取容器统计信息失败", zap.String("containerID", containerID[:12]), zap.Error(err))
		}
		return
	}
	defer func() {
		if closeErr := stats.Body.Close(); closeErr != nil {
			logger.Logger.Error("关闭容器统计信息失败", zap.String("containerID", containerID[:12]), zap.Error(closeErr))
		}
	}()

	var currentStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&currentStats); err != nil {
		logger.Logger.Error("解码容器统计信息失败", zap.String("containerID", containerID[:12]), zap.Error(err))
		return
	}

	sm.statsMutex.Lock()
	defer sm.statsMutex.Unlock()

	// 获取上一次的统计数据
	previousStats, hasPrevious := sm.rawStatsCache[containerID]

	// 存储当前原始数据作为下次的previous
	sm.rawStatsCache[containerID] = &currentStats

	// 如果有上一次的数据，计算差值并更新统计
	if hasPrevious {
		calculatedStats := sm.calculateStats(containerID, previousStats, &currentStats)
		if calculatedStats != nil {
			sm.statsCache[containerID] = calculatedStats
		}
	} else {
		// 第一次采样，创建一个默认的统计数据
		sm.statsCache[containerID] = &ContainerStats{
			ID:            containerID,
			Name:          containerID[:12],
			CPUPercent:    0,
			MemoryUsage:   currentStats.MemoryStats.Usage,
			MemoryLimit:   currentStats.MemoryStats.Limit,
			MemoryPercent: 0,
			NetworkRxRate: 0,
			NetworkTxRate: 0,
			NetworkRx:     sm.getTotalNetworkBytes(currentStats.Networks, "rx"),
			NetworkTx:     sm.getTotalNetworkBytes(currentStats.Networks, "tx"),
			BlockRead:     sm.getTotalBlockBytes(currentStats.BlkioStats.IoServiceBytesRecursive, "Read"),
			BlockWrite:    sm.getTotalBlockBytes(currentStats.BlkioStats.IoServiceBytesRecursive, "Write"),
			PidsCurrent:   currentStats.PidsStats.Current,
			PidsLimit:     currentStats.PidsStats.Limit,
		}
	}
}

// calculateStats 计算两次采样之间的统计差值
func (sm *StatsManager) calculateStats(containerID string, previous, current *container.StatsResponse) *ContainerStats {
	timeDelta := current.Read.Sub(previous.Read).Seconds()
	// logger.Logger.Info("calculateStats", zap.Float64("timeDelta", timeDelta))
	if timeDelta <= 0 {
		return nil
	}

	// 计算CPU使用率
	var cpuPercent float64
	cpuDelta := float64(current.CPUStats.CPUUsage.TotalUsage - previous.CPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(current.CPUStats.SystemUsage - previous.CPUStats.SystemUsage)

	if systemDelta > 0 && cpuDelta >= 0 {
		cpuPercent = (cpuDelta / systemDelta) * 100.0
	}

	// 计算内存使用率
	var memoryPercent float64
	var memoryUsage uint64

	if current.MemoryStats.Limit > 0 {
		var cacheToSubtract uint64
		// 按优先级查找实际存在的字段
		if file, exists := current.MemoryStats.Stats["inactive_file"]; exists {
			// cgroup v2 中表示可回收的非活跃文件缓存
			cacheToSubtract = file
		} else if totalCache, exists := current.MemoryStats.Stats["total_cache"]; exists {
			// 某些版本中可能存在的字段
			cacheToSubtract = totalCache
		} else if cache, exists := current.MemoryStats.Stats["cache"]; exists {
			// 备用字段
			cacheToSubtract = cache
		}
		if current.MemoryStats.Usage > cacheToSubtract {
			memoryUsage = current.MemoryStats.Usage - cacheToSubtract
		} else {
			memoryUsage = current.MemoryStats.Usage
		}
		memoryPercent = float64(memoryUsage) / float64(current.MemoryStats.Limit) * 100.0

	}

	// 计算网络速率
	prevRx := sm.getTotalNetworkBytes(previous.Networks, "rx")
	prevTx := sm.getTotalNetworkBytes(previous.Networks, "tx")
	currRx := sm.getTotalNetworkBytes(current.Networks, "rx")
	currTx := sm.getTotalNetworkBytes(current.Networks, "tx")

	var networkRxRate, networkTxRate uint64
	if currRx >= prevRx {
		networkRxRate = uint64(float64(currRx-prevRx) / timeDelta)
	}
	if currTx >= prevTx {
		networkTxRate = uint64(float64(currTx-prevTx) / timeDelta)
	}

	return &ContainerStats{
		ID:            containerID,
		Name:          containerID[:12],
		CPUPercent:    cpuPercent,
		MemoryUsage:   memoryUsage,
		MemoryLimit:   current.MemoryStats.Limit,
		MemoryPercent: memoryPercent,
		NetworkRxRate: networkRxRate,
		NetworkTxRate: networkTxRate,
		NetworkRx:     currRx,
		NetworkTx:     currTx,
		BlockRead:     sm.getTotalBlockBytes(current.BlkioStats.IoServiceBytesRecursive, "Read"),
		BlockWrite:    sm.getTotalBlockBytes(current.BlkioStats.IoServiceBytesRecursive, "Write"),
		PidsCurrent:   current.PidsStats.Current,
		PidsLimit:     current.PidsStats.Limit,
	}
}

// getTotalNetworkBytes 计算网络总字节数
func (sm *StatsManager) getTotalNetworkBytes(networks map[string]container.NetworkStats, direction string) uint64 {
	var total uint64
	for _, network := range networks {
		if direction == "rx" {
			total += network.RxBytes
		} else {
			total += network.TxBytes
		}
	}
	return total
}

// getTotalBlockBytes 计算块设备总字节数
func (sm *StatsManager) getTotalBlockBytes(entries []container.BlkioStatEntry, operation string) uint64 {
	var total uint64
	for _, entry := range entries {
		if entry.Op == operation {
			total += entry.Value
		}
	}
	return total
}

// cleanupStaleStats 清理不再存在的容器统计数据
func (sm *StatsManager) cleanupStaleStats(currentContainerIDs map[string]bool) {
	sm.statsMutex.Lock()
	defer sm.statsMutex.Unlock()

	// 清理不存在的容器数据
	for containerID := range sm.statsCache {
		if !currentContainerIDs[containerID] {
			delete(sm.statsCache, containerID)
			delete(sm.rawStatsCache, containerID)
		}
	}
}

// AddConnection 添加 WebSocket 连接，如果是第一个连接则启动统计监控
func (sm *StatsManager) AddConnection(ctx context.Context) {
	count := atomic.AddInt32(&sm.connectionCount, 1)
	logger.Logger.Debug("WebSocket 连接已添加，当前连接数:", zap.Int32("count", count))

	// 如果是第一个连接，启动统计监控
	if count == 1 {
		logger.Logger.Debug("启动容器统计监控（首个连接）")
		sm.StartMonitoring(ctx)
	}
}

// RemoveConnection 移除 WebSocket 连接，如果没有连接则停止统计监控
func (sm *StatsManager) RemoveConnection() {
	count := atomic.AddInt32(&sm.connectionCount, -1)
	logger.Logger.Debug("WebSocket 连接已移除，当前连接数:", zap.Int32("count", count))

	// 如果没有连接了，停止统计监控
	if count == 0 {
		logger.Logger.Debug("停止容器统计监控（无连接）")
		sm.StopMonitoring()
	}
}

// GetConnectionCount 获取当前连接数
func (sm *StatsManager) GetConnectionCount() int32 {
	return atomic.LoadInt32(&sm.connectionCount)
}

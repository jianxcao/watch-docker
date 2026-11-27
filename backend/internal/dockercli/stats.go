package dockercli

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
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

	// 后台任务与连接生命周期互斥锁，避免 Add/RemoveConnection 与 Start/StopMonitoring 竞态
	lifecycleMu     sync.Mutex
	stopChan        chan struct{}      // 停止信号
	loopCancel      context.CancelFunc // 取消后台采集循环
	isRunning       bool               // 任务运行状态
	connectionCount int32              // 当前连接数
}

// StatsManagerConfig 统计管理器配置
type StatsManagerConfig struct {
	MaxConcurrency int           // 最大并发数，默认10
	CollectTimeout time.Duration // 单个容器采集超时时间，默认3秒
}

// DockerClientInterface Docker客户端接口，用于解耦与单测 mock
type DockerClientInterface interface {
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
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
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	sm.startMonitoringLocked(ctx)
}

func (sm *StatsManager) startMonitoringLocked(parentCtx context.Context) {
	if sm.isRunning {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	loopCtx, cancel := context.WithCancel(parentCtx)
	sm.isRunning = true
	stopCh := make(chan struct{})
	sm.stopChan = stopCh
	sm.loopCancel = cancel

	go sm.statsMonitoringLoop(loopCtx, stopCh)
}

// StopMonitoring 停止后台统计监控
func (sm *StatsManager) StopMonitoring() {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	sm.stopMonitoringLocked()
}

func (sm *StatsManager) stopMonitoringLocked() {
	sm.isRunning = false
	if sm.loopCancel != nil {
		sm.loopCancel()
		sm.loopCancel = nil
	}
	if sm.stopChan != nil {
		close(sm.stopChan)
		sm.stopChan = nil
	}
	sm.statsMutex.Lock()
	clear(sm.statsCache)
	clear(sm.rawStatsCache)
	sm.statsMutex.Unlock()
}

// getContainerStatsLocked 在已持有 statsMutex 读/写锁的情况下读取单个容器统计副本（禁止再次加锁）
func (sm *StatsManager) getContainerStatsLocked(id string) *ContainerStats {
	if stats, exists := sm.statsCache[id]; exists {
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

	return &ContainerStats{
		ID:   id,
		Name: shortID(id),
	}
}

// GetContainerStats 获取容器资源统计信息（从缓存读取）
func (sm *StatsManager) GetContainerStats(ctx context.Context, id string) *ContainerStats {
	sm.statsMutex.RLock()
	defer sm.statsMutex.RUnlock()
	return sm.getContainerStatsLocked(id)
}

// GetContainersStats 批量获取多个容器的资源统计信息（从缓存读取）
func (sm *StatsManager) GetContainersStats(ctx context.Context, containerIDs []string) (map[string]*ContainerStats, error) {
	statsMap := make(map[string]*ContainerStats, len(containerIDs))

	if len(containerIDs) == 0 {
		return statsMap, nil
	}

	sm.statsMutex.RLock()
	defer sm.statsMutex.RUnlock()

	for _, containerID := range containerIDs {
		statsMap[containerID] = sm.getContainerStatsLocked(containerID)
	}
	return statsMap, nil
}

// statsMonitoringLoop 后台统计监控循环
func (sm *StatsManager) statsMonitoringLoop(ctx context.Context, stopCh <-chan struct{}) {
	defer func() {
		sm.lifecycleMu.Lock()
		if sm.stopChan == stopCh {
			sm.isRunning = false
			sm.stopChan = nil
			if sm.loopCancel != nil {
				sm.loopCancel()
				sm.loopCancel = nil
			}
		}
		sm.lifecycleMu.Unlock()
	}()

	// 启动后立即采样一次，避免新连接等待首个 1s tick
	sm.collectAllContainerStats(ctx)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Logger.Debug("信息统计监控停止")
			return
		case <-stopCh:
			logger.Logger.Debug("信息统计监控停止")
			return
		case <-ticker.C:
			sm.collectAllContainerStats(ctx)
		}
	}
}

// collectAllContainerStats 收集所有运行中容器的统计信息
func (sm *StatsManager) collectAllContainerStats(ctx context.Context) {
	listRes, err := sm.dockerClient.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Logger.Error("获取运行中容器失败", zap.Error(err))
		}
		return
	}
	containers := listRes.Items

	currentContainerIDs := make(map[string]bool, len(containers))
	for _, containerInfo := range containers {
		currentContainerIDs[containerInfo.ID] = true
	}

	semaphore := make(chan struct{}, sm.maxConcurrency)

	var wg sync.WaitGroup
	for _, containerInfo := range containers {
		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()

			containerCtx, cancel := context.WithTimeout(ctx, sm.collectTimeout)
			defer cancel()

			sm.collectSingleContainerStats(containerCtx, containerID)
		}(containerInfo.ID)
	}

	wg.Wait()

	sm.cleanupStaleStats(currentContainerIDs)
}

// collectSingleContainerStats 收集单个容器的统计信息
func (sm *StatsManager) collectSingleContainerStats(ctx context.Context, containerID string) {
	sid := shortID(containerID)
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("收集容器统计信息失败", zap.String("containerID", sid), zap.Any("error", r))
		}
	}()

	stats, err := sm.dockerClient.ContainerStats(ctx, containerID, client.ContainerStatsOptions{
		Stream:                false,
		IncludePreviousSample: false,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Logger.Debug("获取容器统计信息超时", zap.String("containerID", sid))
		} else if !errors.Is(err, context.Canceled) {
			logger.Logger.Debug("获取容器统计信息失败", zap.String("containerID", sid), zap.Error(err))
		}
		return
	}
	defer func() {
		if closeErr := stats.Body.Close(); closeErr != nil {
			logger.Logger.Debug("关闭容器统计信息失败", zap.String("containerID", sid), zap.Error(closeErr))
		}
	}()

	var currentStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&currentStats); err != nil {
		logger.Logger.Debug("解码容器统计信息失败", zap.String("containerID", sid), zap.Error(err))
		return
	}

	sm.statsMutex.Lock()
	defer sm.statsMutex.Unlock()

	previousStats, hasPrevious := sm.rawStatsCache[containerID]
	sm.rawStatsCache[containerID] = &currentStats

	if hasPrevious {
		calculatedStats := sm.calculateStats(containerID, previousStats, &currentStats)
		if calculatedStats != nil {
			sm.statsCache[containerID] = calculatedStats
		}
	} else {
		var memoryPercent float64
		memoryUsage := calcMemoryUsage(&currentStats)
		if currentStats.MemoryStats.Limit > 0 {
			memoryPercent = float64(memoryUsage) / float64(currentStats.MemoryStats.Limit) * 100.0
		}
		sm.statsCache[containerID] = &ContainerStats{
			ID:            containerID,
			Name:          sid,
			CPUPercent:    0,
			MemoryUsage:   memoryUsage,
			MemoryLimit:   currentStats.MemoryStats.Limit,
			MemoryPercent: memoryPercent,
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

func calcMemoryUsage(current *container.StatsResponse) uint64 {
	var cacheToSubtract uint64
	if file, exists := current.MemoryStats.Stats["inactive_file"]; exists {
		cacheToSubtract = file
	} else if totalCache, exists := current.MemoryStats.Stats["total_cache"]; exists {
		cacheToSubtract = totalCache
	} else if cache, exists := current.MemoryStats.Stats["cache"]; exists {
		cacheToSubtract = cache
	}
	if current.MemoryStats.Usage > cacheToSubtract {
		return current.MemoryStats.Usage - cacheToSubtract
	}
	return current.MemoryStats.Usage
}

// calculateStats 计算两次采样之间的统计差值
func (sm *StatsManager) calculateStats(containerID string, previous, current *container.StatsResponse) *ContainerStats {
	timeDelta := current.Read.Sub(previous.Read).Seconds()
	if timeDelta <= 0 {
		return nil
	}

	// 计算 CPU 使用率，先检查 uint64 单调递增防止容器重启时无符号减法回绕
	var cpuPercent float64
	currCPU := current.CPUStats.CPUUsage.TotalUsage
	prevCPU := previous.CPUStats.CPUUsage.TotalUsage
	currSys := current.CPUStats.SystemUsage
	prevSys := previous.CPUStats.SystemUsage

	if currCPU >= prevCPU && currSys > prevSys {
		cpuDelta := float64(currCPU - prevCPU)
		systemDelta := float64(currSys - prevSys)
		onlineCPUs := current.CPUStats.OnlineCPUs
		if onlineCPUs == 0 {
			onlineCPUs = uint32(len(current.CPUStats.CPUUsage.PercpuUsage))
		}
		if onlineCPUs == 0 {
			onlineCPUs = 1
		}
		cpuPercent = (cpuDelta / systemDelta) * float64(onlineCPUs) * 100.0
	}

	var memoryPercent float64
	memoryUsage := calcMemoryUsage(current)
	if current.MemoryStats.Limit > 0 {
		memoryPercent = float64(memoryUsage) / float64(current.MemoryStats.Limit) * 100.0
	}

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
		Name:          shortID(containerID),
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
	for _, netStat := range networks {
		if direction == "rx" {
			total += netStat.RxBytes
		} else {
			total += netStat.TxBytes
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

	for containerID := range sm.statsCache {
		if !currentContainerIDs[containerID] {
			delete(sm.statsCache, containerID)
			delete(sm.rawStatsCache, containerID)
		}
	}
}

// AddConnection 添加 WebSocket 连接，如果是第一个连接则启动统计监控
func (sm *StatsManager) AddConnection(ctx context.Context) {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()

	sm.connectionCount++
	logger.Logger.Debug("WebSocket 连接已添加", zap.Int32("count", sm.connectionCount))

	if sm.connectionCount >= 1 && !sm.isRunning {
		logger.Logger.Debug("启动容器统计监控")
		// 共享后台采集周期独立于单个请求 ctx，避免首个连接断开导致其他连接采集被取消
		sm.startMonitoringLocked(context.Background())
	}
}

// RemoveConnection 移除 WebSocket 连接，如果没有连接则停止统计监控
func (sm *StatsManager) RemoveConnection() {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()

	if sm.connectionCount > 0 {
		sm.connectionCount--
	}
	logger.Logger.Debug("WebSocket 连接已移除", zap.Int32("count", sm.connectionCount))

	if sm.connectionCount <= 0 {
		sm.connectionCount = 0
		logger.Logger.Debug("停止容器统计监控（无连接）")
		sm.stopMonitoringLocked()
	}
}

// GetConnectionCount 获取当前连接数
func (sm *StatsManager) GetConnectionCount() int32 {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	return sm.connectionCount
}

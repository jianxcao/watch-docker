package dockercli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/docker/docker/api/types/container"
)

// ContainerStats 容器资源统计信息
type ContainerStats struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryUsage   uint64  `json:"memoryUsage"` // 字节
	MemoryLimit   uint64  `json:"memoryLimit"` // 字节
	MemoryPercent float64 `json:"memoryPercent"`
	NetworkRx     uint64  `json:"networkRx"`  // 字节
	NetworkTx     uint64  `json:"networkTx"`  // 字节
	BlockRead     uint64  `json:"blockRead"`  // 字节
	BlockWrite    uint64  `json:"blockWrite"` // 字节
	PidsCurrent   uint64  `json:"pidsCurrent"`
	PidsLimit     uint64  `json:"pidsLimit"`
}

// GetContainerStats 获取容器资源统计信息（优化版本）
func (c *Client) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	// 获取容器统计信息（单次采样，避免 priming 等待）
	stats, err := c.docker.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()

	// 解析统计信息
	var v container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&v); err != nil {
		return nil, err
	}

	// 计算CPU使用率
	var cpuPercent float64
	if v.PreCPUStats.CPUUsage.TotalUsage != 0 {
		cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
		if systemDelta > 0.0 && cpuDelta > 0.0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}
	}

	// 计算内存使用率
	var memoryPercent float64
	if v.MemoryStats.Limit > 0 {
		memoryPercent = float64(v.MemoryStats.Usage) / float64(v.MemoryStats.Limit) * 100.0
	}

	// 计算网络统计
	var networkRx, networkTx uint64
	for _, network := range v.Networks {
		networkRx += network.RxBytes
		networkTx += network.TxBytes
	}

	// 计算块设备统计
	var blockRead, blockWrite uint64
	for _, block := range v.BlkioStats.IoServiceBytesRecursive {
		switch block.Op {
		case "Read":
			blockRead += block.Value
		case "Write":
			blockWrite += block.Value
		}
	}

	// 使用容器ID的前12位作为名称，避免额外的inspect调用
	name := id[:12]

	return &ContainerStats{
		ID:            id,
		Name:          name,
		CPUPercent:    cpuPercent,
		MemoryUsage:   v.MemoryStats.Usage,
		MemoryLimit:   v.MemoryStats.Limit,
		MemoryPercent: memoryPercent,
		NetworkRx:     networkRx,
		NetworkTx:     networkTx,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
		PidsCurrent:   v.PidsStats.Current,
		PidsLimit:     v.PidsStats.Limit,
	}, nil
}

// GetContainersStats 批量获取多个容器的资源统计信息（优化版本）
func (c *Client) GetContainersStats(ctx context.Context, containerIDs []string) (map[string]*ContainerStats, error) {
	statsMap := make(map[string]*ContainerStats)

	if len(containerIDs) == 0 {
		return statsMap, nil
	}

	// 使用信号量控制并发数，避免过多并发请求
	const maxConcurrency = 5
	semaphore := make(chan struct{}, maxConcurrency)

	// 并发获取统计信息
	type result struct {
		id    string
		stats *ContainerStats
		err   error
	}

	results := make(chan result, len(containerIDs))

	for _, id := range containerIDs {
		go func(containerID string) {
			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 为每个容器创建独立的超时上下文
			containerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			stats, err := c.GetContainerStats(containerCtx, containerID)
			results <- result{id: containerID, stats: stats, err: err}
		}(id)
	}

	// 收集结果
	for i := 0; i < len(containerIDs); i++ {
		res := <-results
		if res.err != nil {
			// 记录错误但继续处理其他容器
			continue
		}
		statsMap[res.id] = res.stats
	}

	return statsMap, nil
}

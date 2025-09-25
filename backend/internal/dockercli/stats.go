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

// StatsWithTimeDelta 包含两次采样的统计数据和时间差
type StatsWithTimeDelta struct {
	Stats1    container.StatsResponse
	Stats2    container.StatsResponse
	TimeDelta float64 // 时间间隔（秒）
}

// sampleStatsWithInterval 执行双次采样并返回结果
func (c *Client) sampleStatsWithInterval(ctx context.Context, id string, interval time.Duration) (*StatsWithTimeDelta, error) {
	// 第一次采样
	stats1, err := c.docker.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	var v1 container.StatsResponse
	if err := json.NewDecoder(stats1.Body).Decode(&v1); err != nil {
		stats1.Body.Close()
		return nil, err
	}
	stats1.Body.Close()

	// 记录第一次采样时间
	firstTime := time.Now()

	// 等待指定时间间隔
	time.Sleep(interval)

	// 第二次采样
	stats2, err := c.docker.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	defer stats2.Body.Close()

	var v2 container.StatsResponse
	if err := json.NewDecoder(stats2.Body).Decode(&v2); err != nil {
		return nil, err
	}

	// 记录第二次采样时间
	secondTime := time.Now()

	// 计算实际时间间隔（秒）
	timeDelta := secondTime.Sub(firstTime).Seconds()

	return &StatsWithTimeDelta{
		Stats1:    v1,
		Stats2:    v2,
		TimeDelta: timeDelta,
	}, nil
}

// GetContainerStats 获取容器资源统计信息（双次采样版本）
func (c *Client) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	// 执行双次采样
	sampledStats, err := c.sampleStatsWithInterval(ctx, id, 500*time.Millisecond)
	if err != nil {
		return nil, err
	}

	// 获取采样数据
	v1 := sampledStats.Stats1
	v2 := sampledStats.Stats2
	timeDelta := sampledStats.TimeDelta

	// 计算CPU使用率（0-100%）
	var cpuPercent float64
	if timeDelta > 0 {
		cpuDelta := float64(v2.CPUStats.CPUUsage.TotalUsage - v1.CPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(v2.CPUStats.SystemUsage - v1.CPUStats.SystemUsage)

		if systemDelta > 0 && cpuDelta >= 0 {
			// 计算相对于单核的CPU使用率，然后限制在0-100%之间
			rawPercent := (cpuDelta / systemDelta) * 100.0
			// if rawPercent > 100.0 {
			// 	cpuPercent = 100.0
			// } else {

			// }
			cpuPercent = rawPercent
		}
	}

	// 计算内存使用率
	var memoryPercent float64
	var memoryUsage uint64

	// 检查内存统计是否有效
	if v2.MemoryStats.Limit > 0 {
		// 使用实际使用内存（排除缓存）
		if cache, exists := v2.MemoryStats.Stats["cache"]; exists {
			// 实际使用内存 = 总使用内存 - 缓存
			if v2.MemoryStats.Usage > cache {
				memoryUsage = v2.MemoryStats.Usage - cache
			} else {
				memoryUsage = v2.MemoryStats.Usage
			}
		} else {
			memoryUsage = v2.MemoryStats.Usage
		}
		memoryPercent = float64(memoryUsage) / float64(v2.MemoryStats.Limit) * 100.0
	}

	// 计算网络统计（总流量和实时速率）
	var networkRx1, networkTx1, networkRx2, networkTx2 uint64

	// 第一次采样的网络数据
	for _, network := range v1.Networks {
		networkRx1 += network.RxBytes
		networkTx1 += network.TxBytes
	}

	// 第二次采样的网络数据
	for _, network := range v2.Networks {
		networkRx2 += network.RxBytes
		networkTx2 += network.TxBytes
	}

	// 计算网络速率（字节/秒）
	var networkRxRate, networkTxRate uint64
	if timeDelta > 0 {
		if networkRx2 >= networkRx1 {
			networkRxRate = uint64(float64(networkRx2-networkRx1) / timeDelta)
		}
		if networkTx2 >= networkTx1 {
			networkTxRate = uint64(float64(networkTx2-networkTx1) / timeDelta)
		}
	}

	// 计算块设备统计
	var blockRead, blockWrite uint64
	for _, block := range v2.BlkioStats.IoServiceBytesRecursive {
		switch block.Op {
		case "Read":
			blockRead += block.Value
		case "Write":
			blockWrite += block.Value
		}
	}

	// 使用容器ID的前12位作为名称
	name := id[:12]

	return &ContainerStats{
		ID:            id,
		Name:          name,
		CPUPercent:    cpuPercent,
		MemoryUsage:   memoryUsage,
		MemoryLimit:   v2.MemoryStats.Limit,
		MemoryPercent: memoryPercent,
		NetworkRxRate: networkRxRate,
		NetworkTxRate: networkTxRate,
		NetworkRx:     networkRx2,
		NetworkTx:     networkTx2,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
		PidsCurrent:   v2.PidsStats.Current,
		PidsLimit:     v2.PidsStats.Limit,
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

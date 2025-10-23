package dockercli

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type Client struct {
	docker       *client.Client
	statsManager *StatsManager
}

func New(ctx context.Context, host string) (*Client, error) {
	return NewWithStatsConfig(ctx, host, StatsManagerConfig{})
}

// NewWithStatsConfig 使用自定义统计配置创建Docker客户端
func NewWithStatsConfig(ctx context.Context, host string, statsConfig StatsManagerConfig) (*Client, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if strings.TrimSpace(host) != "" {
		opts = append(opts, client.WithHost(host))
	}
	dockerClient, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	// 创建统计管理器
	var statsManager *StatsManager
	if statsConfig.MaxConcurrency > 0 || statsConfig.CollectTimeout > 0 {
		statsManager = NewStatsManagerWithConfig(dockerClient, statsConfig)
	} else {
		statsManager = NewStatsManager(dockerClient)
	}

	clientInstance := &Client{
		docker:       dockerClient,
		statsManager: statsManager,
	}
	return clientInstance, nil
}

func (c *Client) Close() error {
	c.statsManager.StopMonitoring()
	return c.docker.Close()
}

// GetDockerClient 返回底层的 Docker 客户端，供其他模块使用
func (c *Client) GetDockerClient() *client.Client {
	return c.docker
}

// AddStatsConnection 添加统计 WebSocket 连接
func (c *Client) AddStatsConnection(ctx context.Context) {
	c.statsManager.AddConnection(ctx)
}

// RemoveStatsConnection 移除统计 WebSocket 连接
func (c *Client) RemoveStatsConnection() {
	c.statsManager.RemoveConnection()
}

// GetContainerStats 获取容器统计信息
func (c *Client) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	return c.statsManager.GetContainerStats(ctx, id), nil
}

// GetContainersStats 获取多个容器统计信息
func (c *Client) GetContainersStats(ctx context.Context, containerIDs []string) (map[string]*ContainerStats, error) {
	return c.statsManager.GetContainersStats(ctx, containerIDs)
}

// GetVersion 获取Docker版本信息
func (c *Client) GetVersion(ctx context.Context) (types.Version, error) {
	return c.docker.ServerVersion(ctx)
}

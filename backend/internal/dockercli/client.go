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

type ContainerInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	ImageID     string            `json:"imageId"`
	RepoTags    []string          `json:"repoTags"`
	RepoDigests []string          `json:"repoDigests"`
	Labels      map[string]string `json:"labels"`
	State       string            `json:"state"`
	Status      string            `json:"status"`
	Created     int64             `json:"created"`
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
	clientInstance.StartStatsMonitoring(ctx)
	return clientInstance, nil
}

func (c *Client) Close() error {
	c.statsManager.StopMonitoring()
	return c.docker.Close()
}

// StartStatsMonitoring 启动后台统计监控
func (c *Client) StartStatsMonitoring(ctx context.Context) {
	c.statsManager.StartMonitoring(ctx)
}

// StopStatsMonitoring 停止后台统计监控
func (c *Client) StopStatsMonitoring() {
	c.statsManager.StopMonitoring()
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

package dockercli

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

type cachedImageMeta struct {
	Inspect     image.InspectResponse
	FetchedAt   time.Time
	ParentID    string
	ParentKnown bool
}

type cachedStartMeta struct {
	State     string
	StartedAt string
	UpdatedAt time.Time
}

type Client struct {
	docker       *client.Client
	statsManager *StatsManager

	eventsCancel context.CancelFunc
	cacheMu      sync.RWMutex
	imageCache   map[string]cachedImageMeta
	startedCache map[string]cachedStartMeta
}

func New(ctx context.Context, host string) (*Client, error) {
	return NewWithStatsConfig(ctx, host, StatsManagerConfig{})
}

// NewWithStatsConfig 使用自定义统计配置创建Docker客户端
func NewWithStatsConfig(ctx context.Context, host string, statsConfig StatsManagerConfig) (*Client, error) {
	opts := []client.Opt{client.FromEnv}
	if strings.TrimSpace(host) != "" {
		opts = append(opts, client.WithHost(host))
	}
	dockerClient, err := client.New(opts...)
	if err != nil {
		return nil, err
	}

	var statsManager *StatsManager
	if statsConfig.MaxConcurrency > 0 || statsConfig.CollectTimeout > 0 {
		statsManager = NewStatsManagerWithConfig(dockerClient, statsConfig)
	} else {
		statsManager = NewStatsManager(dockerClient)
	}

	watchCtx, cancel := context.WithCancel(context.Background())

	clientInstance := &Client{
		docker:       dockerClient,
		statsManager: statsManager,
		eventsCancel: cancel,
		imageCache:   make(map[string]cachedImageMeta),
		startedCache: make(map[string]cachedStartMeta),
	}

	go clientInstance.watchDockerEvents(watchCtx)

	return clientInstance, nil
}

func (c *Client) watchDockerEvents(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		evRes := c.docker.Events(ctx, client.EventsListOptions{})
		loopErr := func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-evRes.Err:
					return err
				case msg, ok := <-evRes.Messages:
					if !ok {
						return nil
					}
					c.handleDockerEvent(msg)
				}
			}
		}()
		if ctx.Err() != nil {
			return
		}
		if loopErr != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		} else {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}
}

func (c *Client) handleDockerEvent(msg events.Message) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	switch msg.Type {
	case events.ContainerEventType:
		if msg.Actor.ID != "" {
			delete(c.startedCache, msg.Actor.ID)
		}
	case events.ImageEventType:
		if msg.Actor.ID != "" {
			delete(c.imageCache, msg.Actor.ID)
		}
		// 标签或 pull 事件可能改变已有镜像的 RepoTags/RepoDigests，清空镜像缓存确保即时可见
		if msg.Action == events.ActionTag || msg.Action == events.ActionUnTag || msg.Action == events.ActionPull || msg.Action == events.ActionDelete {
			clear(c.imageCache)
		}
	}
}

func (c *Client) Close() error {
	if c.eventsCancel != nil {
		c.eventsCancel()
	}
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
func (c *Client) GetVersion(ctx context.Context) (client.ServerVersionResult, error) {
	return c.docker.ServerVersion(ctx, client.ServerVersionOptions{})
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// InvalidateContainerCache 清除单个容器缓存（例如重启/更新后），兼容短 ID 前缀匹配。
func (c *Client) InvalidateContainerCache(id string) {
	if id == "" {
		return
	}
	c.cacheMu.Lock()
	delete(c.startedCache, id)
	if len(id) < 64 {
		for k := range c.startedCache {
			if strings.HasPrefix(k, id) {
				delete(c.startedCache, k)
			}
		}
	}
	c.cacheMu.Unlock()
}

// InvalidateImageCache 清空镜像 inspect 缓存
func (c *Client) InvalidateImageCache() {
	c.cacheMu.Lock()
	clear(c.imageCache)
	c.cacheMu.Unlock()
}

// InspectContainerStateTime 读取容器 StartedAt（供缓存复用）
func (c *Client) getContainerStartedAt(ctx context.Context, id string, state string) string {
	const cacheTTL = 2 * time.Minute
	now := time.Now()

	c.cacheMu.RLock()
	if cached, ok := c.startedCache[id]; ok && cached.State == state && now.Sub(cached.UpdatedAt) < cacheTTL {
		c.cacheMu.RUnlock()
		return cached.StartedAt
	}
	c.cacheMu.RUnlock()

	res, err := c.docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return ""
	}
	var startedAt string
	if res.Container.State != nil {
		startedAt = res.Container.State.StartedAt
	}

	c.cacheMu.Lock()
	c.startedCache[id] = cachedStartMeta{
		State:     state,
		StartedAt: startedAt,
		UpdatedAt: now,
	}
	c.cacheMu.Unlock()
	return startedAt
}

func (c *Client) getCachedImageMeta(imageID string) (cachedImageMeta, bool) {
	const cacheTTL = 5 * time.Minute
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	if cached, ok := c.imageCache[imageID]; ok && time.Since(cached.FetchedAt) < cacheTTL {
		return cached, true
	}
	return cachedImageMeta{}, false
}

func (c *Client) getCachedImageInspect(imageID string) (image.InspectResponse, bool) {
	if cached, ok := c.getCachedImageMeta(imageID); ok {
		return cached.Inspect, true
	}
	return image.InspectResponse{}, false
}

func (c *Client) setCachedImageInspect(imageID string, resp image.InspectResponse) cachedImageMeta {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	now := time.Now()
	if prev, ok := c.imageCache[imageID]; ok && !now.After(prev.FetchedAt) {
		now = prev.FetchedAt.Add(time.Nanosecond)
	}
	meta := cachedImageMeta{
		Inspect:   resp,
		FetchedAt: now,
	}
	c.imageCache[imageID] = meta
	return meta
}

// setCachedImageParent 在未失效且 inspect 版本未变更的前提下回写父镜像 ID（保留原 FetchedAt）。
func (c *Client) setCachedImageParent(imageID string, fetchedAt time.Time, parentID string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if cur, ok := c.imageCache[imageID]; ok && cur.FetchedAt.Equal(fetchedAt) {
		cur.ParentID = parentID
		cur.ParentKnown = true
		c.imageCache[imageID] = cur
	}
}

// containerSummaryStateString 将 container.ContainerState 转为小写字符串
func containerSummaryStateString(st container.ContainerState) string {
	return string(st)
}

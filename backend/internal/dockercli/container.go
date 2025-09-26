package dockercli

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

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
	StartedAt   string            `json:"startedAt"` // 容器启动时间
	Ports       []PortInfo        `json:"ports"`     // 端口映射信息
}

// PortInfo 端口映射信息
type PortInfo struct {
	IP          string `json:"ip"`          // 主机IP地址
	PrivatePort int    `json:"privatePort"` // 容器内部端口
	PublicPort  int    `json:"publicPort"`  // 主机端口
	Type        string `json:"type"`        // 端口类型 (tcp/udp)
}

// ListContainers 列出容器并附带镜像的 RepoTags/RepoDigests（通过额外的 ImageInspect 合并）
func (c *Client) ListContainers(ctx context.Context, includeStopped bool) ([]ContainerInfo, error) {
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: includeStopped})
	if err != nil {
		return nil, err
	}

	// 先收集需要 inspect 的镜像 ID，避免重复请求
	imageIDSet := make(map[string]struct{})
	for _, ct := range containers {
		if ct.ImageID != "" {
			imageIDSet[ct.ImageID] = struct{}{}
		}
	}

	imageTags := make(map[string][]string)
	imageDigests := make(map[string][]string)
	for imageID := range imageIDSet {
		img, err := c.docker.ImageInspect(ctx, imageID)
		if err != nil {
			// inspect 失败则该镜像的 tags/digests 留空
			continue
		}
		imageTags[imageID] = img.RepoTags
		imageDigests[imageID] = img.RepoDigests
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, ct := range containers {
		name := ""
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		if name == "" {
			if len(ct.ID) >= 12 {
				name = ct.ID[:12]
			} else {
				name = ct.ID
			}
		}

		// 获取容器详细信息（包含启动时间）
		var startedAt string
		if inspect, err := c.docker.ContainerInspect(ctx, ct.ID); err == nil {
			if inspect.State != nil {
				startedAt = inspect.State.StartedAt
			}
		}

		// 转换端口信息
		ports := make([]PortInfo, 0, len(ct.Ports))
		for _, port := range ct.Ports {
			ports = append(ports, PortInfo{
				IP:          port.IP,
				PrivatePort: int(port.PrivatePort),
				PublicPort:  int(port.PublicPort),
				Type:        port.Type,
			})
		}

		info := ContainerInfo{
			ID:          ct.ID,
			Name:        name,
			Image:       ct.Image,
			ImageID:     ct.ImageID,
			RepoTags:    imageTags[ct.ImageID],
			RepoDigests: imageDigests[ct.ImageID],
			Labels:      ct.Labels,
			State:       ct.State,
			Status:      ct.Status,
			Created:     ct.Created,
			StartedAt:   startedAt,
			Ports:       ports,
		}
		result = append(result, info)
	}
	return result, nil
}

// InspectContainer 返回容器的详细信息
func (c *Client) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	return c.docker.ContainerInspect(ctx, id)
}

// StopContainer 停止容器（可选超时时间，单位秒）
func (c *Client) StopContainer(ctx context.Context, id string, timeoutSeconds int) error {
	var timeout *int
	if timeoutSeconds > 0 {
		t := timeoutSeconds
		timeout = &t
	}
	return c.docker.ContainerStop(ctx, id, container.StopOptions{Timeout: timeout})
}

// RenameContainer 重命名容器
func (c *Client) RenameContainer(ctx context.Context, id string, newName string) error {
	return c.docker.ContainerRename(ctx, id, newName)
}

// CreateContainer 使用给定配置与名称创建容器
func (c *Client) CreateContainer(ctx context.Context, name string, cfg *container.Config, host *container.HostConfig, netCfg *network.NetworkingConfig) (string, error) {
	resp, err := c.docker.ContainerCreate(ctx, cfg, host, netCfg, nil, name)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// StartContainer 启动容器
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.docker.ContainerStart(ctx, id, container.StartOptions{})
}

// RemoveContainer 删除容器
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	return c.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: force, RemoveVolumes: false, RemoveLinks: true})
}

// RemoveContainerWithVolumes 删除容器并清理关联的匿名卷
func (c *Client) RemoveContainerWithVolumes(ctx context.Context, id string, force bool) error {
	return c.docker.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: true,
		RemoveLinks:   true, // 清理传统的容器链接（如果存在）
	})
}

// WaitContainerStopped 等待容器完全停止
func (c *Client) WaitContainerStopped(ctx context.Context, id string, maxWaitSeconds int) error {
	timeout := time.Duration(maxWaitSeconds) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		inspect, err := c.docker.ContainerInspect(ctx, id)
		if err != nil {
			// 容器不存在或无法访问，认为已停止
			return nil
		}

		if inspect.State != nil && !inspect.State.Running {
			// 额外等待一小段时间确保文件系统完全释放
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	// 超时后强制杀死容器
	_ = c.docker.ContainerKill(ctx, id, "SIGKILL")
	time.Sleep(500 * time.Millisecond)
	return nil
}

// PruneSystem 清理悬挂的文件系统、网络、镜像等
func (c *Client) PruneSystem(ctx context.Context) error {
	// 清理悬挂的卷
	volFilter := filters.NewArgs()
	volFilter.Add("dangling", "true")
	_, err := c.docker.VolumesPrune(ctx, volFilter)
	if err != nil {
		return err
	}

	// 清理悬挂的网络
	netFilter := filters.NewArgs()
	netFilter.Add("dangling", "true")
	_, err = c.docker.NetworksPrune(ctx, netFilter)
	if err != nil {
		return err
	}

	// 清理悬挂的镜像
	imgFilter := filters.NewArgs()
	imgFilter.Add("dangling", "true")
	_, err = c.docker.ImagesPrune(ctx, imgFilter)
	return err
}

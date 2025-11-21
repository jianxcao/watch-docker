package dockercli

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
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

// RestartContainer 重启容器（可选超时时间，单位秒）
func (c *Client) RestartContainer(ctx context.Context, id string, timeoutSeconds int) error {
	var timeout *int
	if timeoutSeconds > 0 {
		t := timeoutSeconds
		timeout = &t
	}

	return c.docker.ContainerRestart(ctx, id, container.StopOptions{Timeout: timeout})
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
	return c.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: force, RemoveVolumes: false, RemoveLinks: false})
}

// RemoveContainerWithVolumes 删除容器并清理关联的匿名卷
func (c *Client) RemoveContainerWithVolumes(ctx context.Context, id string, force bool) error {
	return c.docker.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: true,
		RemoveLinks:   false, // 清理传统的容器链接（如果存在）
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
	res, err := c.docker.VolumesPrune(ctx, volFilter)
	logger.Logger.Info("VolumesPrune", zap.Any("res", res))
	if err != nil {
		return err
	}

	// 清理未使用的网络（不支持dangling过滤器，使用空过滤器清理未使用的网络）
	netFilter := filters.NewArgs()
	netRes, err := c.docker.NetworksPrune(ctx, netFilter)
	logger.Logger.Info("NetworksPrune", zap.Any("res", netRes))
	if err != nil {
		return err
	}

	// 清理悬挂的镜像
	imgFilter := filters.NewArgs(filters.Arg("dangling", "true"))
	imgRes, err := c.docker.ImagesPrune(ctx, imgFilter)
	logger.Logger.Info("ImagesPrune", zap.Any("res", imgRes))
	return err
}

// SafeRemoveImage 安全删除镜像（检查是否有其他容器使用）
func (c *Client) SafeRemoveImage(ctx context.Context, imageID string) error {
	if imageID == "" {
		return nil
	}

	// 检查是否有其他容器在使用这个镜像
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, ct := range containers {
		if ct.ImageID == imageID {
			// 有其他容器使用，不删除镜像
			return nil
		}
	}
	logger.Logger.Info("没有其他容器使用，可以安全删除镜像", zap.String("imageID", imageID))
	// 没有其他容器使用，可以安全删除
	_, err = c.docker.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true})
	return err
}

// SafeRemoveNetworks 安全删除自定义网络（检查是否有其他容器使用）
func (c *Client) SafeRemoveNetworks(ctx context.Context, networkIDs []string) error {
	for _, networkID := range networkIDs {
		if networkID == "" {
			continue
		}

		// 检查网络信息
		netInfo, err := c.docker.NetworkInspect(ctx, networkID, network.InspectOptions{})
		if err != nil {
			continue // 网络可能已删除，跳过
		}

		// 跳过系统网络（bridge, host, none）
		if netInfo.Name == "bridge" || netInfo.Name == "host" || netInfo.Name == "none" {
			continue
		}

		// 检查是否有其他容器连接到这个网络
		if len(netInfo.Containers) > 0 {
			continue // 有其他容器使用，不删除
		}

		// 安全删除自定义网络
		_ = c.docker.NetworkRemove(ctx, networkID)
	}
	return nil
}

// SafeRemoveVolumes 安全删除匿名卷（检查是否有其他容器使用）
func (c *Client) SafeRemoveVolumes(ctx context.Context, volumeNames []string) error {
	for _, volumeName := range volumeNames {
		if volumeName == "" {
			continue
		}

		// 检查卷是否存在
		_, err := c.docker.VolumeInspect(ctx, volumeName)
		if err != nil {
			continue // 卷可能已删除，跳过
		}

		// 只删除匿名卷（名称像随机字符串的卷，通常很长且包含随机字符）
		if len(volumeName) < 40 {
			// 短名称通常是命名卷，跳过
			continue
		}

		// 检查是否有其他容器使用这个卷
		containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			continue
		}

		inUse := false
		for _, ct := range containers {
			// 检查容器的挂载信息
			inspect, err := c.docker.ContainerInspect(ctx, ct.ID)
			if err != nil {
				continue
			}

			for _, mount := range inspect.Mounts {
				if mount.Name == volumeName {
					inUse = true
					break
				}
			}
			if inUse {
				break
			}
		}

		if !inUse {
			// 没有其他容器使用，安全删除
			_ = c.docker.VolumeRemove(ctx, volumeName, false)
		}
	}
	return nil
}

// ExportContainer 导出容器为tar格式的文件流
func (c *Client) ExportContainer(ctx context.Context, id string) (io.ReadCloser, error) {
	return c.docker.ContainerExport(ctx, id)
}

// CleanupContainerResources 根据容器信息安全清理相关资源
func (c *Client) CleanupContainerResources(ctx context.Context, containerInfo container.InspectResponse) error {
	// 1. 清理镜像
	if containerInfo.Image != "" {
		_ = c.SafeRemoveImage(ctx, containerInfo.Image)
	}

	// // 2. 清理自定义网络 不清理网络
	// if containerInfo.NetworkSettings != nil && containerInfo.NetworkSettings.Networks != nil {
	// 	var networkIDs []string
	// 	for _, netEndpoint := range containerInfo.NetworkSettings.Networks {
	// 		if netEndpoint.NetworkID != "" {
	// 			networkIDs = append(networkIDs, netEndpoint.NetworkID)
	// 		}
	// 	}
	// 	_ = c.SafeRemoveNetworks(ctx, networkIDs)
	// }

	// 3. 清理匿名卷
	if len(containerInfo.Mounts) > 0 {
		var volumeNames []string
		for _, mount := range containerInfo.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				volumeNames = append(volumeNames, mount.Name)
			}
		}
		_ = c.SafeRemoveVolumes(ctx, volumeNames)
	}

	return nil
}

// ContainerLogs 获取容器日志流
func (c *Client) ContainerLogs(ctx context.Context, containerID string, since string, timestamps bool, tail string, follow bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      since,
		Timestamps: timestamps,
		Follow:     follow,
		Tail:       tail,
	}
	return c.docker.ContainerLogs(ctx, containerID, options)
}

// ContainerExecCreate 在容器中创建 exec 实例
func (c *Client) ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error) {
	return c.docker.ContainerExecCreate(ctx, containerID, config)
}

// ContainerExecAttach 附加到 exec 实例
func (c *Client) ContainerExecAttach(ctx context.Context, execID string, config container.ExecStartOptions) (types.HijackedResponse, error) {
	return c.docker.ContainerExecAttach(ctx, execID, config)
}

// ContainerExecResize 调整 exec 实例的终端大小
func (c *Client) ContainerExecResize(ctx context.Context, execID string, options container.ResizeOptions) error {
	return c.docker.ContainerExecResize(ctx, execID, options)
}

func (c *Client) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	resp, err := c.docker.ContainerStats(ctx, containerID, stream)
	if err != nil {
		return container.StatsResponseReader{}, err
	}
	return resp, nil
}

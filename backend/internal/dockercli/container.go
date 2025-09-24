package dockercli

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

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
	return c.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: force, RemoveVolumes: false})
}

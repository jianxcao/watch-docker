package dockercli

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

// VolumeInfo Volume信息
type VolumeInfo struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels"`
	Scope      string            `json:"scope"`
	Options    map[string]string `json:"options"`
	Status     map[string]any    `json:"status"`
	UsageData  *VolumeUsageData  `json:"usageData,omitempty"`
}

// VolumeUsageData Volume使用数据
type VolumeUsageData struct {
	Size     int64 `json:"size"`
	RefCount int   `json:"refCount"`
}

// VolumeListResponse Volume列表响应
type VolumeListResponse struct {
	Volumes     []VolumeInfo `json:"volumes"`
	TotalCount  int          `json:"totalCount"`
	TotalSize   int64        `json:"totalSize"`
	UsedCount   int          `json:"usedCount"`
	UnusedCount int          `json:"unusedCount"`
}

// ContainerRef 容器引用信息
type ContainerRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Running     bool   `json:"running"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
}

// VolumeDetailResponse Volume详情响应
type VolumeDetailResponse struct {
	Volume     VolumeInfo     `json:"volume"`
	Containers []ContainerRef `json:"containers"`
}

// VolumeCreateRequest 创建Volume请求
type VolumeCreateRequest struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driverOpts,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// VolumePruneResponse 清理Volume响应
type VolumePruneResponse struct {
	VolumesDeleted []string `json:"volumesDeleted"`
	SpaceReclaimed int64    `json:"spaceReclaimed"`
}

// ListVolumes 获取Volume列表
func (c *Client) ListVolumes(ctx context.Context) (*VolumeListResponse, error) {
	volumeList, err := c.docker.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}
	diskUsage, err := c.docker.DiskUsage(ctx, types.DiskUsageOptions{
		Types: []types.DiskUsageObject{types.VolumeObject},
	})
	fmt.Println(diskUsage)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	// 获取所有容器以计算引用计数（使用 Docker API 直接获取，包含 Mounts 信息）
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// 构建Volume名称到引用计数的映射
	volumeRefCount := make(map[string]int)
	for _, ctr := range containers {
		for _, mount := range ctr.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				volumeRefCount[mount.Name]++
			}
		}
	}

	var volumes []VolumeInfo
	var totalSize int64
	var usedCount, unusedCount int

	for _, vol := range volumeList.Volumes {
		refCount := volumeRefCount[vol.Name]

		// 获取Volume大小（如果可用）
		var size int64
		if vol.UsageData != nil {
			size = vol.UsageData.Size
		}
		totalSize += size

		if refCount > 0 {
			usedCount++
		} else {
			unusedCount++
		}

		volumeInfo := VolumeInfo{
			Name:       vol.Name,
			Driver:     vol.Driver,
			Mountpoint: vol.Mountpoint,
			CreatedAt:  vol.CreatedAt,
			Labels:     vol.Labels,
			Scope:      vol.Scope,
			Options:    vol.Options,
			Status:     vol.Status,
		}

		// 添加使用数据
		if vol.UsageData != nil || refCount > 0 {
			volumeInfo.UsageData = &VolumeUsageData{
				Size:     size,
				RefCount: refCount,
			}
		}

		volumes = append(volumes, volumeInfo)
	}

	response := &VolumeListResponse{
		Volumes:     volumes,
		TotalCount:  len(volumes),
		TotalSize:   totalSize,
		UsedCount:   usedCount,
		UnusedCount: unusedCount,
	}

	return response, nil
}

// GetVolume 获取Volume详情
func (c *Client) GetVolume(ctx context.Context, name string) (*VolumeDetailResponse, error) {
	vol, err := c.docker.VolumeInspect(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect volume: %w", err)
	}

	// 获取使用该Volume的容器
	containers, err := c.GetVolumeContainers(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume containers: %w", err)
	}

	// 计算大小和引用计数
	var size int64
	if vol.UsageData != nil {
		size = vol.UsageData.Size
	}

	volumeInfo := VolumeInfo{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  vol.CreatedAt,
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
		Status:     vol.Status,
	}

	if vol.UsageData != nil || len(containers) > 0 {
		volumeInfo.UsageData = &VolumeUsageData{
			Size:     size,
			RefCount: len(containers),
		}
	}

	response := &VolumeDetailResponse{
		Volume:     volumeInfo,
		Containers: containers,
	}

	return response, nil
}

// GetVolumeContainers 获取使用该Volume的容器列表
func (c *Client) GetVolumeContainers(ctx context.Context, volumeName string) ([]ContainerRef, error) {
	// 使用 Docker API 直接获取容器列表（包含 Mounts 信息）
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var refs []ContainerRef
	for _, ctr := range containers {
		for _, mount := range ctr.Mounts {
			if mount.Type == "volume" && mount.Name == volumeName {
				// 处理容器名称（去掉前缀 "/"）
				containerName := ""
				if len(ctr.Names) > 0 {
					containerName = ctr.Names[0]
					if len(containerName) > 0 && containerName[0] == '/' {
						containerName = containerName[1:]
					}
				}

				ref := ContainerRef{
					ID:          ctr.ID[:12], // 短ID
					Name:        containerName,
					Image:       ctr.Image,
					Running:     ctr.State == "running",
					Destination: mount.Destination,
					Mode:        mount.Mode,
				}
				refs = append(refs, ref)
			}
		}
	}

	return refs, nil
}

// CreateVolume 创建Volume
func (c *Client) CreateVolume(ctx context.Context, req *VolumeCreateRequest) (*VolumeInfo, error) {
	options := volume.CreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	}

	vol, err := c.docker.VolumeCreate(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}

	volumeInfo := &VolumeInfo{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
		Status:     vol.Status,
		UsageData: &VolumeUsageData{
			Size:     0,
			RefCount: 0,
		},
	}

	return volumeInfo, nil
}

// RemoveVolume 删除Volume
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	err := c.docker.VolumeRemove(ctx, name, force)
	if err != nil {
		return fmt.Errorf("failed to remove volume: %w", err)
	}
	return nil
}

// PruneVolumes 清理未使用的Volume
func (c *Client) PruneVolumes(ctx context.Context) (*VolumePruneResponse, error) {
	report, err := c.docker.VolumesPrune(ctx, filters.Args{})
	if err != nil {
		return nil, fmt.Errorf("failed to prune volumes: %w", err)
	}

	response := &VolumePruneResponse{
		VolumesDeleted: report.VolumesDeleted,
		SpaceReclaimed: int64(report.SpaceReclaimed),
	}

	return response, nil
}

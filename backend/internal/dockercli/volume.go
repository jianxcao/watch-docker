package dockercli

import (
	"context"
	"fmt"
	"strings"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
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
	volListRes, err := c.docker.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	startTime := time.Now()
	ctrListRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Logger.Error("ContainerList API 调用失败",
			zap.Duration("elapsed", elapsed),
			zap.Error(err))
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	containers := ctrListRes.Items
	logger.Logger.Debug("ContainerList API 调用成功",
		zap.Duration("elapsed", elapsed),
		zap.Int("containerCount", len(containers)))

	volumeRefCount := make(map[string]int)
	for _, ctr := range containers {
		for _, m := range ctr.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" {
				volumeRefCount[m.Name]++
			}
		}
	}

	volumes := make([]VolumeInfo, 0, len(volListRes.Items))
	var totalSize int64
	var usedCount, unusedCount int

	for _, vol := range volListRes.Items {
		refCount := volumeRefCount[vol.Name]

		var size int64
		if vol.UsageData != nil && vol.UsageData.Size > 0 {
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

		if size > 0 || refCount > 0 {
			volumeInfo.UsageData = &VolumeUsageData{
				Size:     size,
				RefCount: refCount,
			}
		}

		volumes = append(volumes, volumeInfo)
	}

	return &VolumeListResponse{
		Volumes:     volumes,
		TotalCount:  len(volumes),
		TotalSize:   totalSize,
		UsedCount:   usedCount,
		UnusedCount: unusedCount,
	}, nil
}

// GetVolume 获取Volume详情
func (c *Client) GetVolume(ctx context.Context, name string) (*VolumeDetailResponse, error) {
	volRes, err := c.docker.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect volume: %w", err)
	}
	vol := volRes.Volume

	containers, err := c.GetVolumeContainers(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume containers: %w", err)
	}

	var size int64
	if vol.UsageData != nil && vol.UsageData.Size > 0 {
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

	if size > 0 || len(containers) > 0 {
		volumeInfo.UsageData = &VolumeUsageData{
			Size:     size,
			RefCount: len(containers),
		}
	}

	return &VolumeDetailResponse{
		Volume:     volumeInfo,
		Containers: containers,
	}, nil
}

// GetVolumeContainers 获取使用该Volume的容器列表
func (c *Client) GetVolumeContainers(ctx context.Context, volumeName string) ([]ContainerRef, error) {
	ctrListRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var refs []ContainerRef
	for _, ctr := range ctrListRes.Items {
		for _, m := range ctr.Mounts {
			if m.Type == mount.TypeVolume && m.Name == volumeName {
				containerName := ""
				if len(ctr.Names) > 0 {
					containerName = strings.TrimPrefix(ctr.Names[0], "/")
				}

				refs = append(refs, ContainerRef{
					ID:          shortID(ctr.ID),
					Name:        containerName,
					Image:       ctr.Image,
					Running:     ctr.State == container.StateRunning,
					Destination: m.Destination,
					Mode:        m.Mode,
				})
			}
		}
	}

	return refs, nil
}

// CreateVolume 创建Volume
func (c *Client) CreateVolume(ctx context.Context, req *VolumeCreateRequest) (*VolumeInfo, error) {
	options := client.VolumeCreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	}

	volRes, err := c.docker.VolumeCreate(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}
	vol := volRes.Volume

	return &VolumeInfo{
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
	}, nil
}

// RemoveVolume 删除Volume
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	_, err := c.docker.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: force})
	if err != nil {
		return fmt.Errorf("failed to remove volume: %w", err)
	}
	return nil
}

// PruneVolumes 清理未使用的Volume
func (c *Client) PruneVolumes(ctx context.Context) (*VolumePruneResponse, error) {
	res, err := c.docker.VolumePrune(ctx, client.VolumePruneOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to prune volumes: %w", err)
	}

	return &VolumePruneResponse{
		VolumesDeleted: res.Report.VolumesDeleted,
		SpaceReclaimed: int64(res.Report.SpaceReclaimed),
	}, nil
}

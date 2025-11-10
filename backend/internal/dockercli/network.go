package dockercli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"go.uber.org/zap"
)

// NetworkIPAMConfig IPAM 配置
type NetworkIPAMConfig struct {
	Subnet     string            `json:"subnet,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	IPRange    string            `json:"ipRange,omitempty"`
	AuxAddress map[string]string `json:"auxAddress,omitempty"`
}

// NetworkIPAM IPAM 信息
type NetworkIPAM struct {
	Driver  string              `json:"driver"`
	Options map[string]string   `json:"options,omitempty"`
	Config  []NetworkIPAMConfig `json:"config,omitempty"`
}

// NetworkInfo 网络信息
type NetworkInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	Ingress    bool              `json:"ingress"`
	EnableIPv6 bool              `json:"enableIPv6"`
	IPAM       NetworkIPAM       `json:"ipam"`
	Created    string            `json:"created"`
	Labels     map[string]string `json:"labels"`
	Options    map[string]string `json:"options"`
	// 使用统计
	ContainerCount int `json:"containerCount"`
}

// NetworkContainer 连接到网络的容器信息
type NetworkContainer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Running     bool   `json:"running"`
	IPv4Address string `json:"ipv4Address,omitempty"`
	IPv6Address string `json:"ipv6Address,omitempty"`
	MacAddress  string `json:"macAddress,omitempty"`
	EndpointID  string `json:"endpointId,omitempty"`
}

// NetworkListResponse 网络列表响应
type NetworkListResponse struct {
	Networks     []NetworkInfo `json:"networks"`
	TotalCount   int           `json:"totalCount"`
	UsedCount    int           `json:"usedCount"`
	UnusedCount  int           `json:"unusedCount"`
	BuiltInCount int           `json:"builtInCount"`
	CustomCount  int           `json:"customCount"`
}

// NetworkDetailResponse 网络详情响应
type NetworkDetailResponse struct {
	Network    NetworkInfo        `json:"network"`
	Containers []NetworkContainer `json:"containers"`
}

// NetworkCreateRequest 创建网络请求
type NetworkCreateRequest struct {
	Name       string                    `json:"name"`
	Driver     string                    `json:"driver,omitempty"`
	Scope      string                    `json:"scope,omitempty"`
	Internal   bool                      `json:"internal"`
	Attachable bool                      `json:"attachable"`
	Ingress    bool                      `json:"ingress"`
	EnableIPv6 bool                      `json:"enableIPv6"`
	IPAM       *NetworkIPAMCreateRequest `json:"ipam,omitempty"`
	Options    map[string]string         `json:"options,omitempty"`
	Labels     map[string]string         `json:"labels,omitempty"`
}

// NetworkIPAMCreateRequest 创建网络时的 IPAM 配置
type NetworkIPAMCreateRequest struct {
	Driver  string                    `json:"driver,omitempty"`
	Config  []NetworkIPAMConfigCreate `json:"config,omitempty"`
	Options map[string]string         `json:"options,omitempty"`
}

// NetworkIPAMConfigCreate 创建网络时的 IPAM 配置项
type NetworkIPAMConfigCreate struct {
	Subnet     string            `json:"subnet,omitempty"`
	IPRange    string            `json:"ipRange,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	AuxAddress map[string]string `json:"auxAddress,omitempty"`
}

// NetworkPruneResponse 清理网络响应
type NetworkPruneResponse struct {
	NetworksDeleted []string `json:"networksDeleted"`
}

// NetworkConnectRequest 连接容器到网络请求
type NetworkConnectRequest struct {
	Container   string            `json:"container"`
	IPv4Address string            `json:"ipv4Address,omitempty"`
	IPv6Address string            `json:"ipv6Address,omitempty"`
	Links       []string          `json:"links,omitempty"`
	Aliases     []string          `json:"aliases,omitempty"`
	DriverOpts  map[string]string `json:"driverOpts,omitempty"`
}

// NetworkDisconnectRequest 从网络断开容器请求
type NetworkDisconnectRequest struct {
	Container string `json:"container"`
	Force     bool   `json:"force"`
}

// ListNetworks 获取网络列表
func (c *Client) ListNetworks(ctx context.Context) (*NetworkListResponse, error) {
	networkList, err := c.docker.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	// 获取所有容器以统计网络使用情况
	startTime := time.Now()
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Logger.Error("ContainerList API 调用失败",
			zap.Duration("elapsed", elapsed),
			zap.Error(err))
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	logger.Logger.Info("ContainerList API 调用成功",
		zap.Duration("elapsed", elapsed),
		zap.Int("containerCount", len(containers)))

	// 统计每个网络的容器数量
	networkContainerCount := make(map[string]int)
	for _, ctr := range containers {
		if ctr.NetworkSettings != nil && ctr.NetworkSettings.Networks != nil {
			for networkName := range ctr.NetworkSettings.Networks {
				networkContainerCount[networkName]++
			}
		}
	}

	var networks []NetworkInfo
	var usedCount, unusedCount, builtInCount, customCount int

	// 内置网络列表
	builtInNetworks := map[string]bool{
		"bridge": true,
		"host":   true,
		"none":   true,
	}

	for _, net := range networkList {
		containerCount := networkContainerCount[net.Name]

		// 统计使用情况
		if containerCount > 0 {
			usedCount++
		} else {
			unusedCount++
		}

		// 统计内置和自定义网络
		if builtInNetworks[net.Name] {
			builtInCount++
		} else {
			customCount++
		}

		// 转换 IPAM 配置
		ipam := NetworkIPAM{
			Driver:  net.IPAM.Driver,
			Options: net.IPAM.Options,
		}

		if len(net.IPAM.Config) > 0 {
			ipamConfigs := make([]NetworkIPAMConfig, 0, len(net.IPAM.Config))
			for _, cfg := range net.IPAM.Config {
				ipamConfigs = append(ipamConfigs, NetworkIPAMConfig{
					Subnet:     cfg.Subnet,
					Gateway:    cfg.Gateway,
					IPRange:    cfg.IPRange,
					AuxAddress: cfg.AuxAddress,
				})
			}
			ipam.Config = ipamConfigs
		}

		networkInfo := NetworkInfo{
			ID:             net.ID,
			Name:           net.Name,
			Driver:         net.Driver,
			Scope:          net.Scope,
			Internal:       net.Internal,
			Attachable:     net.Attachable,
			Ingress:        net.Ingress,
			EnableIPv6:     net.EnableIPv6,
			IPAM:           ipam,
			Created:        net.Created.Format(time.RFC3339),
			Labels:         net.Labels,
			Options:        net.Options,
			ContainerCount: containerCount,
		}

		networks = append(networks, networkInfo)
	}

	response := &NetworkListResponse{
		Networks:     networks,
		TotalCount:   len(networks),
		UsedCount:    usedCount,
		UnusedCount:  unusedCount,
		BuiltInCount: builtInCount,
		CustomCount:  customCount,
	}

	return response, nil
}

// GetNetwork 获取网络详情
func (c *Client) GetNetwork(ctx context.Context, id string) (*NetworkDetailResponse, error) {
	net, err := c.docker.NetworkInspect(ctx, id, network.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect network: %w", err)
	}

	// 转换 IPAM 配置
	ipam := NetworkIPAM{
		Driver:  net.IPAM.Driver,
		Options: net.IPAM.Options,
	}

	if len(net.IPAM.Config) > 0 {
		ipamConfigs := make([]NetworkIPAMConfig, 0, len(net.IPAM.Config))
		for _, cfg := range net.IPAM.Config {
			ipamConfigs = append(ipamConfigs, NetworkIPAMConfig{
				Subnet:     cfg.Subnet,
				Gateway:    cfg.Gateway,
				IPRange:    cfg.IPRange,
				AuxAddress: cfg.AuxAddress,
			})
		}
		ipam.Config = ipamConfigs
	}

	networkInfo := NetworkInfo{
		ID:             net.ID,
		Name:           net.Name,
		Driver:         net.Driver,
		Scope:          net.Scope,
		Internal:       net.Internal,
		Attachable:     net.Attachable,
		Ingress:        net.Ingress,
		EnableIPv6:     net.EnableIPv6,
		IPAM:           ipam,
		Created:        net.Created.Format(time.RFC3339),
		Labels:         net.Labels,
		Options:        net.Options,
		ContainerCount: len(net.Containers),
	}

	// 获取连接到该网络的容器详细信息
	var networkContainers []NetworkContainer
	if len(net.Containers) > 0 {
		// 获取所有容器信息
		allContainers, err := c.docker.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			logger.Logger.Warn("failed to list containers for network details", zap.Error(err))
		} else {
			// 创建容器ID到容器信息的映射
			containerMap := make(map[string]container.Summary)
			for _, ctr := range allContainers {
				containerMap[ctr.ID] = ctr
			}

			for containerID, endpoint := range net.Containers {
				ctr, exists := containerMap[containerID]

				// 获取容器名称
				containerName := endpoint.Name
				if exists && len(ctr.Names) > 0 {
					containerName = ctr.Names[0]
					if len(containerName) > 0 && containerName[0] == '/' {
						containerName = containerName[1:]
					}
				}

				networkContainer := NetworkContainer{
					ID:          containerID[:12], // 短ID
					Name:        containerName,
					IPv4Address: endpoint.IPv4Address,
					IPv6Address: endpoint.IPv6Address,
					MacAddress:  endpoint.MacAddress,
					EndpointID:  endpoint.EndpointID,
				}

				// 如果容器存在，添加更多信息
				if exists {
					networkContainer.Image = ctr.Image
					networkContainer.Running = ctr.State == "running"
				}

				networkContainers = append(networkContainers, networkContainer)
			}
		}
	}

	response := &NetworkDetailResponse{
		Network:    networkInfo,
		Containers: networkContainers,
	}

	return response, nil
}

// CreateNetwork 创建网络
func (c *Client) CreateNetwork(ctx context.Context, req *NetworkCreateRequest) (*NetworkInfo, error) {
	// 验证 Ingress 网络必须使用 overlay 驱动和 global 作用域
	if req.Ingress && (req.Driver != "overlay" || req.Scope != "global") {
		// 自动修正：如果设置了 Ingress，但驱动不是 overlay 或作用域不是 global，则将 Ingress 设置为 false
		logger.Logger.Warn("ingress network requires overlay driver and global scope, auto-correcting",
			zap.String("name", req.Name),
			zap.String("driver", req.Driver),
			zap.String("scope", req.Scope))
		req.Ingress = false
	}

	// 验证 Ingress 和 Attachable 互斥
	if req.Ingress && req.Attachable {
		// 自动修正：如果同时设置了 Ingress 和 Attachable，则禁用 Attachable
		logger.Logger.Warn("ingress and attachable are mutually exclusive, disabling attachable",
			zap.String("name", req.Name))
		req.Attachable = false
	}

	// 验证 macvlan 或 ipvlan 驱动需要 parent 参数
	if req.Driver == "macvlan" || req.Driver == "ipvlan" {
		if req.Options == nil || req.Options["parent"] == "" {
			return nil, fmt.Errorf("%s 驱动需要指定 parent 参数（父网络接口），例如：eth0", req.Driver)
		}
		logger.Logger.Info("creating network with parent interface",
			zap.String("name", req.Name),
			zap.String("driver", req.Driver),
			zap.String("parent", req.Options["parent"]))
	}

	options := network.CreateOptions{
		Driver:     req.Driver,
		Scope:      req.Scope,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Ingress:    req.Ingress,
		EnableIPv6: &req.EnableIPv6,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	// 设置 IPAM 配置
	if req.IPAM != nil {
		ipam := network.IPAM{
			Driver:  req.IPAM.Driver,
			Options: req.IPAM.Options,
		}

		if len(req.IPAM.Config) > 0 {
			ipamConfigs := make([]network.IPAMConfig, 0, len(req.IPAM.Config))
			for _, cfg := range req.IPAM.Config {
				ipamConfigs = append(ipamConfigs, network.IPAMConfig{
					Subnet:     cfg.Subnet,
					IPRange:    cfg.IPRange,
					Gateway:    cfg.Gateway,
					AuxAddress: cfg.AuxAddress,
				})
			}
			ipam.Config = ipamConfigs
		}

		options.IPAM = &ipam
	}

	response, err := c.docker.NetworkCreate(ctx, req.Name, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	// 返回创建的网络信息
	if response.Warning != "" {
		logger.Logger.Warn("network created with warning",
			zap.String("warning", response.Warning),
			zap.String("networkId", response.ID))
	}

	// 获取完整的网络信息
	net, err := c.docker.NetworkInspect(ctx, response.ID, network.InspectOptions{})
	if err != nil {
		// 如果获取失败，返回基本信息
		logger.Logger.Warn("failed to inspect newly created network", zap.Error(err))
		return &NetworkInfo{
			ID:             response.ID,
			Name:           req.Name,
			Driver:         req.Driver,
			Scope:          req.Scope,
			Internal:       req.Internal,
			Attachable:     req.Attachable,
			Ingress:        req.Ingress,
			EnableIPv6:     req.EnableIPv6,
			Created:        time.Now().Format(time.RFC3339),
			Labels:         req.Labels,
			Options:        req.Options,
			ContainerCount: 0,
		}, nil
	}

	// 转换 IPAM 配置
	ipam := NetworkIPAM{
		Driver:  net.IPAM.Driver,
		Options: net.IPAM.Options,
	}

	if len(net.IPAM.Config) > 0 {
		ipamConfigs := make([]NetworkIPAMConfig, 0, len(net.IPAM.Config))
		for _, cfg := range net.IPAM.Config {
			ipamConfigs = append(ipamConfigs, NetworkIPAMConfig{
				Subnet:     cfg.Subnet,
				Gateway:    cfg.Gateway,
				IPRange:    cfg.IPRange,
				AuxAddress: cfg.AuxAddress,
			})
		}
		ipam.Config = ipamConfigs
	}

	networkInfo := &NetworkInfo{
		ID:             net.ID,
		Name:           net.Name,
		Driver:         net.Driver,
		Scope:          net.Scope,
		Internal:       net.Internal,
		Attachable:     net.Attachable,
		Ingress:        net.Ingress,
		EnableIPv6:     net.EnableIPv6,
		IPAM:           ipam,
		Created:        net.Created.Format(time.RFC3339),
		Labels:         net.Labels,
		Options:        net.Options,
		ContainerCount: 0,
	}

	return networkInfo, nil
}

// DeleteNetwork 删除网络
func (c *Client) DeleteNetwork(ctx context.Context, id string) error {
	err := c.docker.NetworkRemove(ctx, id)
	if err != nil {
		// 检查是否是因为有容器连接而失败
		if strings.Contains(err.Error(), "has active endpoints") {
			return fmt.Errorf("网络正在被容器使用，无法删除")
		}
		return fmt.Errorf("failed to remove network: %w", err)
	}
	return nil
}

// PruneNetworks 清理未使用的网络
func (c *Client) PruneNetworks(ctx context.Context) (*NetworkPruneResponse, error) {
	report, err := c.docker.NetworksPrune(ctx, filters.Args{})
	if err != nil {
		return nil, fmt.Errorf("failed to prune networks: %w", err)
	}

	response := &NetworkPruneResponse{
		NetworksDeleted: report.NetworksDeleted,
	}

	return response, nil
}

// ConnectContainer 将容器连接到网络
func (c *Client) ConnectContainer(ctx context.Context, networkID string, req *NetworkConnectRequest) error {
	endpointConfig := &network.EndpointSettings{
		IPAMConfig: &network.EndpointIPAMConfig{
			IPv4Address: req.IPv4Address,
			IPv6Address: req.IPv6Address,
		},
		Links:      req.Links,
		Aliases:    req.Aliases,
		DriverOpts: req.DriverOpts,
	}

	err := c.docker.NetworkConnect(ctx, networkID, req.Container, endpointConfig)
	if err != nil {
		return fmt.Errorf("failed to connect container to network: %w", err)
	}

	return nil
}

// DisconnectContainer 从网络断开容器
func (c *Client) DisconnectContainer(ctx context.Context, networkID string, req *NetworkDisconnectRequest) error {
	err := c.docker.NetworkDisconnect(ctx, networkID, req.Container, req.Force)
	if err != nil {
		return fmt.Errorf("failed to disconnect container from network: %w", err)
	}

	return nil
}

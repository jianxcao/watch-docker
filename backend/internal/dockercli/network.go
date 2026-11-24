package dockercli

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
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
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	Scope          string            `json:"scope"`
	Internal       bool              `json:"internal"`
	Attachable     bool              `json:"attachable"`
	Ingress        bool              `json:"ingress"`
	EnableIPv6     bool              `json:"enableIPv6"`
	IPAM           NetworkIPAM       `json:"ipam"`
	Created        string            `json:"created"`
	Labels         map[string]string `json:"labels"`
	Options        map[string]string `json:"options"`
	ContainerCount int               `json:"containerCount"`
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

func convertNetworkIPAM(ipam network.IPAM) NetworkIPAM {
	res := NetworkIPAM{
		Driver:  ipam.Driver,
		Options: ipam.Options,
	}
	if len(ipam.Config) > 0 {
		configs := make([]NetworkIPAMConfig, 0, len(ipam.Config))
		for _, cfg := range ipam.Config {
			item := NetworkIPAMConfig{}
			if cfg.Subnet.IsValid() {
				item.Subnet = cfg.Subnet.String()
			}
			if cfg.Gateway.IsValid() {
				item.Gateway = cfg.Gateway.String()
			}
			if cfg.IPRange.IsValid() {
				item.IPRange = cfg.IPRange.String()
			}
			if len(cfg.AuxAddress) > 0 {
				item.AuxAddress = make(map[string]string, len(cfg.AuxAddress))
				for k, v := range cfg.AuxAddress {
					if v.IsValid() {
						item.AuxAddress[k] = v.String()
					}
				}
			}
			configs = append(configs, item)
		}
		res.Config = configs
	}
	return res
}

func toNetworkInfo(netObj network.Network, containerCount int) NetworkInfo {
	created := ""
	if !netObj.Created.IsZero() {
		created = netObj.Created.Format(time.RFC3339)
	}
	return NetworkInfo{
		ID:             netObj.ID,
		Name:           netObj.Name,
		Driver:         netObj.Driver,
		Scope:          netObj.Scope,
		Internal:       netObj.Internal,
		Attachable:     netObj.Attachable,
		Ingress:        netObj.Ingress,
		EnableIPv6:     netObj.EnableIPv6,
		IPAM:           convertNetworkIPAM(netObj.IPAM),
		Created:        created,
		Labels:         netObj.Labels,
		Options:        netObj.Options,
		ContainerCount: containerCount,
	}
}

// ListNetworks 获取网络列表
func (c *Client) ListNetworks(ctx context.Context) (*NetworkListResponse, error) {
	netListRes, err := c.docker.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	networkList := netListRes.Items

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

	networkContainerCount := make(map[string]int)
	for _, ctr := range containers {
		if ctr.NetworkSettings != nil && ctr.NetworkSettings.Networks != nil {
			for networkName := range ctr.NetworkSettings.Networks {
				networkContainerCount[networkName]++
			}
		}
	}

	networks := make([]NetworkInfo, 0, len(networkList))
	var usedCount, unusedCount, builtInCount, customCount int

	builtInNetworks := map[string]bool{
		"bridge": true,
		"host":   true,
		"none":   true,
	}

	for _, netSummary := range networkList {
		containerCount := networkContainerCount[netSummary.Name]
		if containerCount > 0 {
			usedCount++
		} else {
			unusedCount++
		}

		if builtInNetworks[netSummary.Name] {
			builtInCount++
		} else {
			customCount++
		}

		networks = append(networks, toNetworkInfo(netSummary.Network, containerCount))
	}

	return &NetworkListResponse{
		Networks:     networks,
		TotalCount:   len(networks),
		UsedCount:    usedCount,
		UnusedCount:  unusedCount,
		BuiltInCount: builtInCount,
		CustomCount:  customCount,
	}, nil
}

// GetNetwork 获取网络详情
func (c *Client) GetNetwork(ctx context.Context, id string) (*NetworkDetailResponse, error) {
	inspectRes, err := c.docker.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect network: %w", err)
	}
	netDetail := inspectRes.Network

	networkInfo := toNetworkInfo(netDetail.Network, len(netDetail.Containers))

	var networkContainers []NetworkContainer
	if len(netDetail.Containers) > 0 {
		ctrListRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
		if err != nil {
			logger.Logger.Warn("failed to list containers for network details", zap.Error(err))
		} else {
			containerMap := make(map[string]container.Summary, len(ctrListRes.Items))
			for _, ctr := range ctrListRes.Items {
				containerMap[ctr.ID] = ctr
			}

			for containerID, endpoint := range netDetail.Containers {
				ctr, exists := containerMap[containerID]

				containerName := endpoint.Name
				if exists && len(ctr.Names) > 0 {
					containerName = strings.TrimPrefix(ctr.Names[0], "/")
				}

				ipv4Str := ""
				if endpoint.IPv4Address.IsValid() {
					ipv4Str = endpoint.IPv4Address.String()
				}
				ipv6Str := ""
				if endpoint.IPv6Address.IsValid() {
					ipv6Str = endpoint.IPv6Address.String()
				}

				nc := NetworkContainer{
					ID:          shortID(containerID),
					Name:        containerName,
					IPv4Address: ipv4Str,
					IPv6Address: ipv6Str,
					MacAddress:  endpoint.MacAddress.String(),
					EndpointID:  endpoint.EndpointID,
				}
				if exists {
					nc.Image = ctr.Image
					nc.Running = ctr.State == container.StateRunning
				}
				networkContainers = append(networkContainers, nc)
			}
		}
	}

	return &NetworkDetailResponse{
		Network:    networkInfo,
		Containers: networkContainers,
	}, nil
}

// CreateNetwork 创建网络
func (c *Client) CreateNetwork(ctx context.Context, req *NetworkCreateRequest) (*NetworkInfo, error) {
	if req.Ingress && (req.Driver != "overlay" || req.Scope != "global") {
		logger.Logger.Warn("ingress network requires overlay driver and global scope, auto-correcting",
			zap.String("name", req.Name),
			zap.String("driver", req.Driver),
			zap.String("scope", req.Scope))
		req.Ingress = false
	}

	if req.Ingress && req.Attachable {
		logger.Logger.Warn("ingress and attachable are mutually exclusive, disabling attachable",
			zap.String("name", req.Name))
		req.Attachable = false
	}

	if req.Driver == "macvlan" || req.Driver == "ipvlan" {
		if req.Options == nil || req.Options["parent"] == "" {
			return nil, fmt.Errorf("%s 驱动需要指定 parent 参数（父网络接口），例如：eth0", req.Driver)
		}
	}

	options := client.NetworkCreateOptions{
		Driver:     req.Driver,
		Scope:      req.Scope,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Ingress:    req.Ingress,
		EnableIPv6: &req.EnableIPv6,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	if req.IPAM != nil {
		ipam := network.IPAM{
			Driver:  req.IPAM.Driver,
			Options: req.IPAM.Options,
		}

		if len(req.IPAM.Config) > 0 {
			ipamConfigs := make([]network.IPAMConfig, 0, len(req.IPAM.Config))
			for _, cfg := range req.IPAM.Config {
				item := network.IPAMConfig{}
				if s := strings.TrimSpace(cfg.Subnet); s != "" {
					p, err := netip.ParsePrefix(s)
					if err != nil {
						return nil, fmt.Errorf("无效的子网格式 %q: %w", s, err)
					}
					item.Subnet = p
				}
				if r := strings.TrimSpace(cfg.IPRange); r != "" {
					p, err := netip.ParsePrefix(r)
					if err != nil {
						return nil, fmt.Errorf("无效的 IP 范围格式 %q: %w", r, err)
					}
					item.IPRange = p
				}
				if g := strings.TrimSpace(cfg.Gateway); g != "" {
					a, err := netip.ParseAddr(g)
					if err != nil {
						return nil, fmt.Errorf("无效的网关地址 %q: %w", g, err)
					}
					item.Gateway = a
				}
				if len(cfg.AuxAddress) > 0 {
					item.AuxAddress = make(map[string]netip.Addr, len(cfg.AuxAddress))
					for k, v := range cfg.AuxAddress {
						a, err := netip.ParseAddr(strings.TrimSpace(v))
						if err != nil {
							return nil, fmt.Errorf("无效的辅助地址 %q=%q: %w", k, v, err)
						}
						item.AuxAddress[k] = a
					}
				}
				ipamConfigs = append(ipamConfigs, item)
			}
			ipam.Config = ipamConfigs
		}

		options.IPAM = &ipam
	}

	response, err := c.docker.NetworkCreate(ctx, req.Name, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	if len(response.Warning) > 0 {
		logger.Logger.Warn("network created with warning",
			zap.Strings("warning", response.Warning),
			zap.String("networkId", response.ID))
	}

	inspectRes, err := c.docker.NetworkInspect(ctx, response.ID, client.NetworkInspectOptions{})
	if err != nil {
		logger.Logger.Warn("failed to inspect newly created network", zap.Error(err))
		return &NetworkInfo{
			ID:         response.ID,
			Name:       req.Name,
			Driver:     req.Driver,
			Scope:      req.Scope,
			Internal:   req.Internal,
			Attachable: req.Attachable,
			Ingress:    req.Ingress,
			EnableIPv6: req.EnableIPv6,
			Created:    time.Now().Format(time.RFC3339),
			Labels:     req.Labels,
			Options:    req.Options,
		}, nil
	}

	info := toNetworkInfo(inspectRes.Network.Network, 0)
	return &info, nil
}

// DeleteNetwork 删除网络
func (c *Client) DeleteNetwork(ctx context.Context, id string) error {
	_, err := c.docker.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "has active endpoints") {
			return fmt.Errorf("网络正在被容器使用，无法删除")
		}
		return fmt.Errorf("failed to remove network: %w", err)
	}
	return nil
}

// PruneNetworks 清理未使用的网络
func (c *Client) PruneNetworks(ctx context.Context) (*NetworkPruneResponse, error) {
	res, err := c.docker.NetworkPrune(ctx, client.NetworkPruneOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to prune networks: %w", err)
	}

	return &NetworkPruneResponse{
		NetworksDeleted: res.Report.NetworksDeleted,
	}, nil
}

// ConnectContainer 将容器连接到网络
func (c *Client) ConnectContainer(ctx context.Context, networkID string, req *NetworkConnectRequest) error {
	var ipamCfg *network.EndpointIPAMConfig
	if strings.TrimSpace(req.IPv4Address) != "" || strings.TrimSpace(req.IPv6Address) != "" {
		ipamCfg = &network.EndpointIPAMConfig{}
		if v4 := strings.TrimSpace(req.IPv4Address); v4 != "" {
			a, err := netip.ParseAddr(v4)
			if err != nil {
				return fmt.Errorf("无效的 IPv4 地址 %q: %w", v4, err)
			}
			ipamCfg.IPv4Address = a
		}
		if v6 := strings.TrimSpace(req.IPv6Address); v6 != "" {
			a, err := netip.ParseAddr(v6)
			if err != nil {
				return fmt.Errorf("无效的 IPv6 地址 %q: %w", v6, err)
			}
			ipamCfg.IPv6Address = a
		}
	}

	endpointConfig := &network.EndpointSettings{
		IPAMConfig: ipamCfg,
		Links:      req.Links,
		Aliases:    req.Aliases,
		DriverOpts: req.DriverOpts,
	}

	_, err := c.docker.NetworkConnect(ctx, networkID, client.NetworkConnectOptions{
		Container:      req.Container,
		EndpointConfig: endpointConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to connect container to network: %w", err)
	}

	return nil
}

// DisconnectContainer 从网络断开容器
func (c *Client) DisconnectContainer(ctx context.Context, networkID string, req *NetworkDisconnectRequest) error {
	_, err := c.docker.NetworkDisconnect(ctx, networkID, client.NetworkDisconnectOptions{
		Container: req.Container,
		Force:     req.Force,
	})
	if err != nil {
		return fmt.Errorf("failed to disconnect container from network: %w", err)
	}

	return nil
}

// ParseMACAddress 解析 MAC 地址字符串为 network.HardwareAddr
func ParseMACAddress(mac string) (network.HardwareAddr, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return nil, nil
	}
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, err
	}
	return network.HardwareAddr(hw), nil
}

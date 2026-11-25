package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/jianxcao/watch-docker/backend/internal/auth"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"github.com/jianxcao/watch-docker/backend/internal/wsstream"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// setupContainerRoutes 设置容器相关的路由
func (s *Server) setupContainerRoutes(protected *gin.RouterGroup) {
	protected.GET("/containers", s.handleListContainers())
	protected.POST("/containers/create", s.handleCreateContainer())
	protected.GET("/containers/:id", s.handleGetContainerDetail())
	protected.POST("/containers/stats", s.handleGetContainersStats())
	protected.GET("/containers/stats/ws", s.handleStatsWebSocket())
	protected.GET("/containers/:id/stats/ws", s.handleContainerDetailStatsWebSocket())
	protected.GET("/containers/logs/:containerID/ws", s.handleContainerLogsWebSocket())
	protected.GET("/containers/:id/shell/ws", s.handleContainerShellWebSocket())
	protected.POST("/containers/:id/update", s.handleUpdateContainer())
	protected.POST("/updates/run", s.handleBatchUpdate())
	protected.GET("/updates/batch/ws", s.handleBatchUpdateWebSocket())
	protected.POST("/containers/:id/stop", s.handleStopContainer())
	protected.POST("/containers/:id/start", s.handleStartContainer())
	protected.POST("/containers/:id/restart", s.handleRestartContainer())
	protected.DELETE("/containers/:id", s.handleDeleteContainer())
	protected.GET("/containers/:id/export", s.handleExportContainer())
	protected.POST("/containers/import", s.handleImportContainer())
	protected.POST("/system/prune", s.handlePruneSystem())
	protected.POST("/update/all", s.handleUpdateAll())

	// 文件操作路由组
	files := protected.Group("/containers/:id/files")
	{
		files.GET("", s.handleListContainerFiles())
		files.GET("/content", s.handleGetContainerFileContent())
		files.PUT("/content", s.handleUpdateContainerFileContent())
		files.POST("/upload", s.handleUploadContainerFiles())
		files.POST("/download-token", s.handleGenerateDownloadToken())
		files.POST("/create", s.handleCreateContainerPath())
		files.DELETE("/delete", s.handleDeleteContainerPath())
		files.POST("/rename", s.handleRenameContainerPath())
		files.POST("/chmod", s.handleChmodContainerPath())
	}
}

// ContainerCreateRequest 容器创建请求
type ContainerCreateRequest struct {
	Name              string                   `json:"name"`
	Image             string                   `json:"image" binding:"required"`
	Cmd               []string                 `json:"cmd"`
	Entrypoint        []string                 `json:"entrypoint"`
	WorkingDir        string                   `json:"workingDir"`
	Env               []string                 `json:"env"`
	ExposedPorts      map[string]struct{}      `json:"exposedPorts"`
	Labels            map[string]string        `json:"labels"`
	Hostname          string                   `json:"hostname"`
	Domainname        string                   `json:"domainname"`
	User              string                   `json:"user"`
	AttachStdin       bool                     `json:"attachStdin"`
	AttachStdout      bool                     `json:"attachStdout"`
	AttachStderr      bool                     `json:"attachStderr"`
	Tty               bool                     `json:"tty"`
	OpenStdin         bool                     `json:"openStdin"`
	StdinOnce         bool                     `json:"stdinOnce"`
	Binds             []string                 `json:"binds"`
	PortBindings      map[string][]PortBinding `json:"portBindings"`
	RestartPolicy     RestartPolicy            `json:"restartPolicy"`
	AutoRemove        bool                     `json:"autoRemove"`
	NetworkMode       string                   `json:"networkMode"`
	Privileged        bool                     `json:"privileged"`
	PublishAllPorts   bool                     `json:"publishAllPorts"`
	ReadonlyRootfs    bool                     `json:"readonlyRootfs"`
	Dns               []string                 `json:"dns"`
	DnsSearch         []string                 `json:"dnsSearch"`
	DnsOptions        []string                 `json:"dnsOptions"`
	ExtraHosts        []string                 `json:"extraHosts"`
	CapAdd            []string                 `json:"capAdd"`
	CapDrop           []string                 `json:"capDrop"`
	SecurityOpt       []string                 `json:"securityOpt"`
	CpuShares         int64                    `json:"cpuShares"`
	Memory            int64                    `json:"memory"`
	MemoryReservation int64                    `json:"memoryReservation"`
	CpuQuota          int64                    `json:"cpuQuota"`
	CpuPeriod         int64                    `json:"cpuPeriod"`
	CpusetCpus        string                   `json:"cpusetCpus"`
	CpusetMems        string                   `json:"cpusetMems"`
	BlkioWeight       uint16                   `json:"blkioWeight"`
	ShmSize           int64                    `json:"shmSize"`
	PidMode           string                   `json:"pidMode"`
	IpcMode           string                   `json:"ipcMode"`
	UTSMode           string                   `json:"utsMode"`
	Cgroup            string                   `json:"cgroup"`
	Runtime           string                   `json:"runtime"`
	Devices           []DeviceMapping          `json:"devices"`
	DeviceRequests    []DeviceRequest          `json:"deviceRequests"`
	NetworkConfig     *NetworkConfig           `json:"networkConfig"`
	NetworksToCreate  []NetworkToCreate        `json:"networksToCreate"` // 需要创建的网络列表
}

// PortBinding 端口绑定
type PortBinding struct {
	HostIP   string `json:"hostIP"`
	HostPort string `json:"hostPort"`
}

// RestartPolicy 重启策略
type RestartPolicy struct {
	Name              string `json:"name"`
	MaximumRetryCount int    `json:"maximumRetryCount"`
}

// DeviceMapping 设备映射
type DeviceMapping struct {
	PathOnHost        string `json:"pathOnHost"`
	PathInContainer   string `json:"pathInContainer"`
	CgroupPermissions string `json:"cgroupPermissions"`
}

// DeviceRequest GPU 等设备请求
type DeviceRequest struct {
	Driver       string            `json:"driver"`
	Count        int               `json:"count"`
	DeviceIDs    []string          `json:"deviceIDs"`
	Capabilities [][]string        `json:"capabilities"`
	Options      map[string]string `json:"options"`
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	EndpointsConfig map[string]*EndpointSettings `json:"endpointsConfig"`
}

// NetworkToCreate 待创建的网络配置
type NetworkToCreate struct {
	Name       string                    `json:"name" binding:"required"`
	Driver     string                    `json:"driver"` // bridge, overlay, macvlan 等
	EnableIPv6 bool                      `json:"enableIPv6"`
	IPAM       *NetworkIPAMCreateRequest `json:"ipam,omitempty"`
	Internal   bool                      `json:"internal"`
	Attachable bool                      `json:"attachable"`
	Labels     map[string]string         `json:"labels,omitempty"`
	Options    map[string]string         `json:"options,omitempty"`
}

// NetworkIPAMCreateRequest 网络 IPAM 配置
type NetworkIPAMCreateRequest struct {
	Driver  string                    `json:"driver,omitempty"`
	Config  []NetworkIPAMConfigCreate `json:"config,omitempty"`
	Options map[string]string         `json:"options,omitempty"`
}

// NetworkIPAMConfigCreate IPAM 配置项
type NetworkIPAMConfigCreate struct {
	Subnet     string            `json:"subnet,omitempty"`
	IPRange    string            `json:"ipRange,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	AuxAddress map[string]string `json:"auxAddress,omitempty"`
}

// EndpointSettings 端点设置
type EndpointSettings struct {
	IPAMConfig          *EndpointIPAMConfig `json:"ipamConfig"`
	Links               []string            `json:"links"`
	Aliases             []string            `json:"aliases"`
	NetworkID           string              `json:"networkID"`
	EndpointID          string              `json:"endpointID"`
	Gateway             string              `json:"gateway"`
	IPAddress           string              `json:"ipAddress"`
	IPPrefixLen         int                 `json:"ipPrefixLen"`
	IPv6Gateway         string              `json:"ipv6Gateway"`
	GlobalIPv6Address   string              `json:"globalIPv6Address"`
	GlobalIPv6PrefixLen int                 `json:"globalIPv6PrefixLen"`
	MacAddress          string              `json:"macAddress"`
}

// EndpointIPAMConfig IPAM 配置
type EndpointIPAMConfig struct {
	IPv4Address string `json:"ipv4Address"`
	IPv6Address string `json:"ipv6Address"`
}

// parseContainerPort 解析容器端口字符串，未指定协议时默认 tcp。
func parseContainerPort(portStr string) (network.Port, error) {
	p := strings.TrimSpace(portStr)
	if p == "" {
		return network.Port{}, fmt.Errorf("empty port")
	}
	if !strings.Contains(p, "/") {
		p += "/tcp"
	}
	return network.ParsePort(p)
}

// parseOptionalAddr 解析可选 IP 字符串；空字符串返回零值 netip.Addr。
func parseOptionalAddr(raw string) (netip.Addr, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return netip.Addr{}, nil
	}
	return netip.ParseAddr(trimmed)
}

func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// handleCreateContainer 处理容器创建
func (s *Server) handleCreateContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ContainerCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Error("bind create container request", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "请求参数错误: "+err.Error()))
			return
		}

		// 验证必填字段
		req.Image = strings.TrimSpace(req.Image)
		if req.Image == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "镜像名称不能为空"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()

		// 构建容器配置
		config := &container.Config{
			Image:        req.Image,        // 镜像名称
			Cmd:          req.Cmd,          // 容器启动命令
			Entrypoint:   req.Entrypoint,   // 入口点
			WorkingDir:   req.WorkingDir,   // 工作目录
			Env:          req.Env,          // 环境变量
			Labels:       req.Labels,       // 标签
			Hostname:     req.Hostname,     // 主机名
			Domainname:   req.Domainname,   // 域名
			User:         req.User,         // 用户(格式: uid:gid 或 username)
			AttachStdin:  req.AttachStdin,  // 附加标准输入, 通常用于交互式容器
			AttachStdout: req.AttachStdout, // 附加标准输出, 用于查看容器的日志输出
			AttachStderr: req.AttachStderr, // 附加标准错误, 用于查看容器的错误日志
			Tty:          req.Tty,          // 分配伪终端, 通常与 -t 参数对应，在交互式 shell 中使用
			OpenStdin:    req.OpenStdin,    // 打开标准输入, 通常与 -i 参数对应
			StdinOnce:    req.StdinOnce,    // 标准输入是否只使用一次, 如果为 true，当第一个附加的客户端断开连接后，stdin 会关闭
			// MacAddress:   req.MacAddress,   // 容器 MAC 地址
		}

		// 设置暴露端口
		if len(req.ExposedPorts) > 0 {
			config.ExposedPorts = make(network.PortSet, len(req.ExposedPorts))
			for port := range req.ExposedPorts {
				p, err := parseContainerPort(port)
				if err != nil {
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("无效的暴露端口 %q: %v", port, err)))
					return
				}
				config.ExposedPorts[p] = struct{}{}
			}
		}

		// 解析 DNS 地址
		var dnsAddrs []netip.Addr
		if len(req.Dns) > 0 {
			dnsAddrs = make([]netip.Addr, 0, len(req.Dns))
			for _, dnsStr := range req.Dns {
				addr, err := netip.ParseAddr(strings.TrimSpace(dnsStr))
				if err != nil {
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("无效的 DNS 地址 %q: %v", dnsStr, err)))
					return
				}
				dnsAddrs = append(dnsAddrs, addr)
			}
		}

		// 构建主机配置
		trimmedNetMode := strings.TrimSpace(req.NetworkMode)
		hostConfig := &container.HostConfig{
			Binds:           req.Binds,                                                                                                                                  // 数据卷绑定
			RestartPolicy:   container.RestartPolicy{Name: container.RestartPolicyMode(req.RestartPolicy.Name), MaximumRetryCount: req.RestartPolicy.MaximumRetryCount}, // 重启策略
			AutoRemove:      req.AutoRemove,                                                                                                                             // 容器退出时自动删除
			NetworkMode:     container.NetworkMode(trimmedNetMode),                                                                                                      // 网络模式
			Privileged:      req.Privileged,                                                                                                                             // 特权模式
			PublishAllPorts: req.PublishAllPorts,                                                                                                                        // 发布所有端口
			ReadonlyRootfs:  req.ReadonlyRootfs,                                                                                                                         // 只读根文件系统
			DNS:             dnsAddrs,                                                                                                                                   // DNS 服务器
			DNSSearch:       req.DnsSearch,                                                                                                                              // DNS 搜索域
			DNSOptions:      req.DnsOptions,                                                                                                                             // DNS 选项
			ExtraHosts:      req.ExtraHosts,                                                                                                                             // 额外的主机映射
			CapAdd:          req.CapAdd,                                                                                                                                 // 添加 Linux 能力
			CapDrop:         req.CapDrop,                                                                                                                                // 移除 Linux 能力
			SecurityOpt:     req.SecurityOpt,                                                                                                                            // 安全选项
			Resources: container.Resources{
				CPUShares:         req.CpuShares,         // CPU 份额(相对权重)
				Memory:            req.Memory,            // 内存限制(字节)
				MemoryReservation: req.MemoryReservation, // 内存预留(字节)
				CPUQuota:          req.CpuQuota,          // CPU 配额(微秒)
				CPUPeriod:         req.CpuPeriod,         // CPU 周期(微秒)
				CpusetCpus:        req.CpusetCpus,        // 允许使用的 CPU 集合
				CpusetMems:        req.CpusetMems,        // 允许使用的内存节点
				BlkioWeight:       req.BlkioWeight,       // 块 I/O 权重
			},
			ShmSize: req.ShmSize,                      // 共享内存大小(字节)
			PidMode: container.PidMode(req.PidMode),   // PID 命名空间模式
			IpcMode: container.IpcMode(req.IpcMode),   // IPC 命名空间模式
			UTSMode: container.UTSMode(req.UTSMode),   // UTS 命名空间模式
			Cgroup:  container.CgroupSpec(req.Cgroup), // Cgroup 父路径
			Runtime: req.Runtime,                      // 运行时(如 nvidia)
		}

		// 设置端口绑定(将主机端口映射到容器端口，同时自动声明 ExposedPorts)
		if len(req.PortBindings) > 0 {
			hostConfig.PortBindings = make(network.PortMap, len(req.PortBindings))
			if config.ExposedPorts == nil {
				config.ExposedPorts = make(network.PortSet, len(req.PortBindings))
			}
			for portStr, bindings := range req.PortBindings {
				port, err := parseContainerPort(portStr)
				if err != nil {
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("无效的端口配置 %q: %v", portStr, err)))
					return
				}
				config.ExposedPorts[port] = struct{}{}
				portBindings := make([]network.PortBinding, 0, len(bindings))
				for _, binding := range bindings {
					hostIP, err := parseOptionalAddr(binding.HostIP)
					if err != nil {
						c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("无效的主机 IP %q: %v", binding.HostIP, err)))
						return
					}
					portBindings = append(portBindings, network.PortBinding{
						HostIP:   hostIP,
						HostPort: binding.HostPort,
					})
				}
				hostConfig.PortBindings[port] = portBindings
			}
		}

		// 设置设备映射(将主机设备映射到容器)
		if len(req.Devices) > 0 {
			hostConfig.Devices = make([]container.DeviceMapping, 0, len(req.Devices))
			for _, dev := range req.Devices {
				hostConfig.Devices = append(hostConfig.Devices, container.DeviceMapping{
					PathOnHost:        dev.PathOnHost,        // 主机设备路径
					PathInContainer:   dev.PathInContainer,   // 容器内设备路径
					CgroupPermissions: dev.CgroupPermissions, // Cgroup 权限(如 rwm)
				})
			}
		}

		// 设置设备请求(用于 GPU 等特殊设备)
		if len(req.DeviceRequests) > 0 {
			hostConfig.DeviceRequests = make([]container.DeviceRequest, 0, len(req.DeviceRequests))
			for _, devReq := range req.DeviceRequests {
				hostConfig.DeviceRequests = append(hostConfig.DeviceRequests, container.DeviceRequest{
					Driver:       devReq.Driver,       // 设备驱动(如 nvidia)
					Count:        devReq.Count,        // 设备数量(-1 表示全部)
					DeviceIDs:    devReq.DeviceIDs,    // 设备 ID 列表
					Capabilities: devReq.Capabilities, // 设备能力(如 [[gpu]])
					Options:      devReq.Options,      // 其他选项
				})
			}
		}

		// 构建网络配置(主网络写入 Create，其余网络创建后通过 ConnectNetwork 附加)
		var networkConfig *network.NetworkingConfig
		var extraNetworks map[string]*network.EndpointSettings
		if req.NetworkConfig != nil && len(req.NetworkConfig.EndpointsConfig) > 0 &&
			!hostConfig.NetworkMode.IsHost() && !hostConfig.NetworkMode.IsNone() && !hostConfig.NetworkMode.IsContainer() {
			netNames := make([]string, 0, len(req.NetworkConfig.EndpointsConfig))
			for netName := range req.NetworkConfig.EndpointsConfig {
				netNames = append(netNames, netName)
			}
			sort.Strings(netNames)

			buildEndpoint := func(netName string, endpoint *EndpointSettings) (*network.EndpointSettings, error) {
				if endpoint == nil {
					return &network.EndpointSettings{}, nil
				}
				gw, err := parseOptionalAddr(endpoint.Gateway)
				if err != nil {
					return nil, fmt.Errorf("网络 %s 网关无效: %w", netName, err)
				}
				ipAddr, err := parseOptionalAddr(endpoint.IPAddress)
				if err != nil {
					return nil, fmt.Errorf("网络 %s IPv4 地址无效: %w", netName, err)
				}
				ip6Gw, err := parseOptionalAddr(endpoint.IPv6Gateway)
				if err != nil {
					return nil, fmt.Errorf("网络 %s IPv6 网关无效: %w", netName, err)
				}
				ip6Addr, err := parseOptionalAddr(endpoint.GlobalIPv6Address)
				if err != nil {
					return nil, fmt.Errorf("网络 %s IPv6 地址无效: %w", netName, err)
				}
				macAddr, err := dockercli.ParseMACAddress(endpoint.MacAddress)
				if err != nil {
					return nil, fmt.Errorf("网络 %s MAC 地址无效: %w", netName, err)
				}
				ep := &network.EndpointSettings{
					Links:               endpoint.Links,
					Aliases:             endpoint.Aliases,
					NetworkID:           endpoint.NetworkID,
					EndpointID:          endpoint.EndpointID,
					Gateway:             gw,
					IPAddress:           ipAddr,
					IPPrefixLen:         endpoint.IPPrefixLen,
					IPv6Gateway:         ip6Gw,
					GlobalIPv6Address:   ip6Addr,
					GlobalIPv6PrefixLen: endpoint.GlobalIPv6PrefixLen,
					MacAddress:          macAddr,
				}
				if endpoint.IPAMConfig != nil {
					v4, err := parseOptionalAddr(endpoint.IPAMConfig.IPv4Address)
					if err != nil {
						return nil, fmt.Errorf("网络 %s IPAM IPv4 无效: %w", netName, err)
					}
					v6, err := parseOptionalAddr(endpoint.IPAMConfig.IPv6Address)
					if err != nil {
						return nil, fmt.Errorf("网络 %s IPAM IPv6 无效: %w", netName, err)
					}
					ep.IPAMConfig = &network.EndpointIPAMConfig{
						IPv4Address: v4,
						IPv6Address: v6,
					}
				}
				return ep, nil
			}

			primaryName := netNames[0]
			hasPrimaryEndpoint := true
			if rawMode := trimmedNetMode; rawMode != "" {
				if rawMode == "default" {
					if _, hasDefault := req.NetworkConfig.EndpointsConfig["default"]; !hasDefault {
						if _, hasBridge := req.NetworkConfig.EndpointsConfig["bridge"]; hasBridge {
							rawMode = "bridge"
						}
					}
				}
				if _, ok := req.NetworkConfig.EndpointsConfig[rawMode]; ok {
					primaryName = rawMode
				} else {
					creatingNets := make(map[string]bool, len(req.NetworksToCreate))
					for _, n := range req.NetworksToCreate {
						if name := strings.TrimSpace(n.Name); name != "" {
							creatingNets[name] = true
						}
					}
					var exactIDMatches, prefixIDMatches []string
					allowPrefixMatch := len(rawMode) >= 4 && isHexString(rawMode) && !creatingNets[rawMode]
					for _, netName := range netNames {
						ep := req.NetworkConfig.EndpointsConfig[netName]
						if ep == nil {
							continue
						}
						netID := strings.TrimSpace(ep.NetworkID)
						if netID == "" {
							continue
						}
						if netID == rawMode {
							exactIDMatches = append(exactIDMatches, netName)
						} else if allowPrefixMatch && strings.HasPrefix(netID, rawMode) {
							prefixIDMatches = append(prefixIDMatches, netName)
						}
					}
					switch {
					case len(exactIDMatches) > 1:
						c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("主网络 %q 匹配到多个端点", rawMode)))
						return
					case len(exactIDMatches) == 1:
						primaryName = exactIDMatches[0]
					case len(prefixIDMatches) > 1:
						c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, fmt.Sprintf("主网络 ID 前缀 %q 存在歧义", rawMode)))
						return
					case len(prefixIDMatches) == 1:
						primaryName = prefixIDMatches[0]
					default:
						primaryName = rawMode
						hasPrimaryEndpoint = false
					}
				}
			}

			hostConfig.NetworkMode = container.NetworkMode(primaryName)
			if hasPrimaryEndpoint {
				primaryEp, err := buildEndpoint(primaryName, req.NetworkConfig.EndpointsConfig[primaryName])
				if err != nil {
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, err.Error()))
					return
				}
				networkConfig = &network.NetworkingConfig{
					EndpointsConfig: map[string]*network.EndpointSettings{
						primaryName: primaryEp,
					},
				}
			}

			extraNetworks = make(map[string]*network.EndpointSettings, len(netNames))
			for _, netName := range netNames {
				if hasPrimaryEndpoint && netName == primaryName {
					continue
				}
				ep, err := buildEndpoint(netName, req.NetworkConfig.EndpointsConfig[netName])
				if err != nil {
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, err.Error()))
					return
				}
				extraNetworks[netName] = ep
			}
			if len(extraNetworks) == 0 {
				extraNetworks = nil
			}
		}

		s.logger.Info("creating container",
			zap.String("name", req.Name),
			zap.String("image", req.Image))

		// 检查镜像是否存在，如果不存在则拉取镜像
		exists, err := s.docker.ImageExists(ctx, req.Image)
		if err != nil {
			s.logger.Error("check image existence failed",
				zap.String("image", req.Image),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "检查镜像失败: "+err.Error()))
			return
		}

		if !exists {
			// 镜像不存在，尝试拉取镜像
			s.logger.Info("image not found locally, pulling image",
				zap.String("image", req.Image))
			if err := s.docker.ImagePull(ctx, req.Image); err != nil {
				s.logger.Error("pull image failed",
					zap.String("image", req.Image),
					zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "拉取镜像失败: "+err.Error()))
				return
			}
			s.logger.Info("image pulled successfully",
				zap.String("image", req.Image))
		} else {
			s.logger.Info("image exists locally",
				zap.String("image", req.Image))
		}

		// 处理需要创建的网络
		if len(req.NetworksToCreate) > 0 {
			s.logger.Info("processing networks to create", zap.Int("count", len(req.NetworksToCreate)))

			createdNetworks := make([]string, 0)
			for _, netToCreate := range req.NetworksToCreate {
				// 验证网络名称
				if netToCreate.Name == "" {
					s.logger.Error("network name is empty")
					c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "网络名称不能为空"))
					return
				}

				// 设置默认驱动
				if netToCreate.Driver == "" {
					netToCreate.Driver = "bridge"
				}

				// 检查网络是否已存在
				_, err := s.docker.GetNetwork(ctx, netToCreate.Name)
				if err == nil {
					// 网络已存在，跳过创建
					s.logger.Info("network already exists, skipping creation",
						zap.String("network", netToCreate.Name))
					continue
				}

				// 网络不存在，创建网络
				s.logger.Info("creating network",
					zap.String("name", netToCreate.Name),
					zap.String("driver", netToCreate.Driver),
					zap.Bool("enableIPv6", netToCreate.EnableIPv6))

				// 构建网络创建请求
				createReq := &dockercli.NetworkCreateRequest{
					Name:       netToCreate.Name,
					Driver:     netToCreate.Driver,
					EnableIPv6: netToCreate.EnableIPv6,
					Internal:   netToCreate.Internal,
					Attachable: netToCreate.Attachable,
					Labels:     netToCreate.Labels,
					Options:    netToCreate.Options,
				}

				// 设置 IPAM 配置
				if netToCreate.IPAM != nil {
					createReq.IPAM = &dockercli.NetworkIPAMCreateRequest{
						Driver:  netToCreate.IPAM.Driver,
						Options: netToCreate.IPAM.Options,
					}

					if len(netToCreate.IPAM.Config) > 0 {
						createReq.IPAM.Config = make([]dockercli.NetworkIPAMConfigCreate, 0, len(netToCreate.IPAM.Config))
						for _, cfg := range netToCreate.IPAM.Config {
							createReq.IPAM.Config = append(createReq.IPAM.Config, dockercli.NetworkIPAMConfigCreate{
								Subnet:     cfg.Subnet,
								IPRange:    cfg.IPRange,
								Gateway:    cfg.Gateway,
								AuxAddress: cfg.AuxAddress,
							})
						}
					}
				}

				// 创建网络
				networkInfo, err := s.docker.CreateNetwork(ctx, createReq)
				if err != nil {
					s.logger.Error("create network failed",
						zap.String("name", netToCreate.Name),
						zap.Error(err))
					c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, fmt.Sprintf("创建网络 %s 失败: %s", netToCreate.Name, err.Error())))
					return
				}

				createdNetworks = append(createdNetworks, networkInfo.Name)
				s.logger.Info("network created successfully",
					zap.String("name", networkInfo.Name),
					zap.String("id", networkInfo.ID),
					zap.String("driver", networkInfo.Driver))
			}

			if len(createdNetworks) > 0 {
				s.logger.Info("networks created",
					zap.Strings("networks", createdNetworks))
			}
		}

		// 创建容器
		containerID, err := s.docker.CreateContainer(ctx, req.Name, config, hostConfig, networkConfig)
		if err != nil {
			s.logger.Error("create container failed",
				zap.String("name", req.Name),
				zap.String("image", req.Image),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "创建容器失败: "+err.Error()))
			return
		}

		// 若指定了多个网络，附加其余网络（失败时回滚已创建的容器）
		if len(extraNetworks) > 0 {
			extraNames := make([]string, 0, len(extraNetworks))
			for netName := range extraNetworks {
				extraNames = append(extraNames, netName)
			}
			sort.Strings(extraNames)
			for _, netName := range extraNames {
				if err := s.docker.ConnectContainerToNetwork(ctx, netName, containerID, extraNetworks[netName]); err != nil {
					s.logger.Error("connect extra network failed",
						zap.String("name", req.Name),
						zap.String("containerID", containerID),
						zap.String("network", netName),
						zap.Error(err))
					rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
					removeErr := s.docker.RemoveContainer(rollbackCtx, containerID, true)
					cancelRollback()
					if removeErr != nil {
						s.logger.Error("创建容器后回滚失败",
							zap.String("containerID", containerID),
							zap.String("network", netName),
							zap.Error(removeErr))
					}
					c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, fmt.Sprintf("连接网络 %s 失败: %s", netName, err.Error())))
					return
				}
			}
		}

		s.logger.Info("container created successfully",
			zap.String("name", req.Name),
			zap.String("containerID", containerID))

		// 创建成功后自动启动容器
		if err := s.docker.StartContainer(ctx, containerID); err != nil {
			s.logger.Error("start container failed",
				zap.String("name", req.Name),
				zap.String("containerID", containerID),
				zap.Error(err))
			// 启动失败不影响创建成功，但记录错误信息
			c.JSON(http.StatusOK, NewSuccessRes(gin.H{
				"id":      containerID,
				"message": "容器创建成功，但启动失败: " + err.Error(),
			}))
			return
		}

		s.logger.Info("container started successfully",
			zap.String("name", req.Name),
			zap.String("containerID", containerID))

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"id":      containerID,
			"message": "容器创建并启动成功",
		}))
	}
}

func (s *Server) handleUpdateContainer() gin.HandlerFunc {
	type reqBody struct {
		Image string `json:"image"`
	}
	return func(c *gin.Context) {
		id := c.Param("id")
		var body reqBody
		_ = c.ShouldBindJSON(&body)
		body.Image = strings.TrimSpace(body.Image)
		if body.Image == "" {
			// try inspect to get current image
			info, err := s.docker.InspectContainer(c.Request.Context(), id)
			if err != nil {
				s.logger.Error("inspect", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeImageRequired, "image required"))
				return
			}
			if info.Config != nil {
				body.Image = strings.TrimSpace(info.Config.Image)
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		newID, err := s.updater.UpdateContainer(ctx, id, body.Image)
		if err != nil {
			s.logger.Error("update container", zap.String("container", id), zap.String("image", body.Image), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(codeForUpdateErr(err), err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true, "newContainerId": newID}))
	}
}

// handleBatchUpdate 触发一次批量更新：
// 1) 扫描当前状态
// 2) 对需要更新的容器逐个执行更新（串行）
func (s *Server) handleBatchUpdate() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()

		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, true, true)
		if err != nil {
			s.logger.Error("batch scan failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "scan failed"))
			return
		}

		updated := make([]string, 0)
		failed := make(map[string]string)
		failedCodes := make(map[string]int)
		for _, st := range statuses {
			if st.Skipped || st.SkippedUpdate || st.Status != "UpdateAvailable" {
				continue
			}
			if !st.Running && !cfg.Docker.IncludeStopped {
				continue
			}
			uctx, cancelOne := context.WithTimeout(ctx, 5*time.Minute)
			if _, err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
				failed[st.Name] = err.Error()
				failedCodes[st.Name] = codeForUpdateErr(err)
				s.logger.Error("auto update failed", zap.String("container", st.Name), zap.String("image", st.Image), zap.Error(err))
			} else {
				updated = append(updated, st.Name)
			}
			cancelOne()
		}
		if len(failed) > 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeUpdateFailed, "some containers failed to update"))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"updated": updated, "failed": failed, "failedCodes": failedCodes}))
	}
}

// codeForUpdateErr 根据错误来源映射错误码
func codeForUpdateErr(err error) int {
	if err == nil {
		return SUCCESS
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "pull:"):
		return CodeRegistryError
	case strings.HasPrefix(msg, "inspect:"), strings.HasPrefix(msg, "stop:"), strings.HasPrefix(msg, "create:"), strings.HasPrefix(msg, "start new:"), strings.HasPrefix(msg, "remove:"):
		return CodeDockerError
	default:
		return CodeUpdateFailed
	}
}

func (s *Server) handleGetContainerDetail() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "container id required"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// 获取容器详细信息
		containerDetail, err := s.docker.InspectContainer(ctx, id)
		if err != nil {
			s.logger.Error("inspect container", zap.String("containerID", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取容器详情失败: "+err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"container": containerDetail}))
	}
}

func (s *Server) handleListContainers() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		isUserCache := c.Query("isUserCache") == "true"
		isHaveUpdate := c.Query("isHaveUpdate") == "true"
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		statuses, err := s.scanner.ScanOnce(ctx, true, cfg.Scan.Concurrency, isUserCache, isHaveUpdate)
		if err != nil {
			s.logger.Error("scan failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeScanFailed, "scan failed"))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"containers": statuses}))
	}
}

func (s *Server) handleGetContainersStats() gin.HandlerFunc {
	type reqBody struct {
		ContainerIDs []string `json:"containerIds" binding:"required"`
	}

	return func(c *gin.Context) {
		var body reqBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request body"))
			return
		}

		if len(body.ContainerIDs) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "container ids required"))
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// 获取容器统计信息
		statsMap, err := s.docker.GetContainersStats(ctx, body.ContainerIDs)
		if err != nil {
			s.logger.Error("get containers stats failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取容器统计信息失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"stats": statsMap}))
	}
}

// handleContainerDetailStatsWebSocket 处理单个容器详细统计的 WebSocket 连接
func (s *Server) handleContainerDetailStatsWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从路径参数获取容器 ID
		containerID := c.Param("id")

		if containerID == "" {
			s.logger.Error("Missing containerID parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing containerID parameter"})
			return
		}

		s.logger.Info("Container detail stats WebSocket connection request",
			zap.String("containerID", containerID))

		// 使用 StreamManager 处理 WebSocket 连接
		// 每个容器的统计流是独立的，使用 containerID 作为唯一标识
		key := fmt.Sprintf("container-detail-stats-%s", containerID)
		s.streamManagerString.HandleWebSocket(c, key, func() wsstream.StreamSource[string] {
			return wsstream.NewContainerDetailStatsSource(wsstream.ContainerDetailStatsSourceOptions{
				ContainerID:  containerID,
				DockerClient: s.docker,
				Key:          key,
			})
		})
	}
}

func (s *Server) handleStopContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.docker.StopContainer(c.Request.Context(), id, 20); err != nil {
			s.logger.Error("stop container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleStartContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.docker.StartContainer(c.Request.Context(), id); err != nil {
			s.logger.Error("start container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleRestartContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		ctx := c.Request.Context()

		// 使用 Docker SDK 的 ContainerRestart 方法重启容器
		if err := s.docker.RestartContainer(ctx, id, 20); err != nil {
			s.logger.Error("restart container failed", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "重启容器失败: "+err.Error()))
			return
		}

		s.logger.Info("container restarted successfully", zap.String("container", id))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

func (s *Server) handleDeleteContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		force := c.Query("force") == "true"
		removeVolumes := c.Query("removeVolumes") == "true"
		removeNetworks := c.Query("removeNetworks") == "true"

		ctx := c.Request.Context()

		s.logger.Info("deleting container",
			zap.String("container", id),
			zap.Bool("force", force),
			zap.Bool("removeVolumes", removeVolumes),
			zap.Bool("removeNetworks", removeNetworks))

		// 如果需要删除关联资源，先获取容器信息
		var containerInfo container.InspectResponse
		if removeVolumes || removeNetworks {
			var err error
			containerInfo, err = s.docker.InspectContainer(ctx, id)
			if err != nil {
				s.logger.Error("inspect container before delete", zap.String("container", id), zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "获取容器信息失败: "+err.Error()))
				return
			}
		}

		// 删除容器
		if err := s.docker.RemoveContainer(ctx, id, force); err != nil {
			s.logger.Error("delete container", zap.String("container", id), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		s.logger.Info("container deleted successfully", zap.String("container", id))

		// 删除关联的卷
		if removeVolumes && len(containerInfo.Mounts) > 0 {
			var volumeNames []string
			for _, m := range containerInfo.Mounts {
				if volName := dockercli.ResolveMountVolumeName(m); volName != "" {
					volumeNames = append(volumeNames, volName)
				}
			}
			if len(volumeNames) > 0 {
				s.logger.Info("attempting to remove volumes",
					zap.String("container", id),
					zap.Strings("volumes", volumeNames))
				if err := s.docker.SafeRemoveVolumes(ctx, volumeNames); err != nil {
					// 忽略卷删除错误，只记录日志
					s.logger.Warn("failed to remove some volumes",
						zap.String("container", id),
						zap.Strings("volumes", volumeNames),
						zap.Error(err))
				} else {
					s.logger.Info("volumes removed successfully",
						zap.String("container", id),
						zap.Strings("volumes", volumeNames))
				}
			}
		}

		// 删除关联的自定义网络
		if removeNetworks && containerInfo.NetworkSettings != nil && containerInfo.NetworkSettings.Networks != nil {
			var networkIDs []string
			for _, netEndpoint := range containerInfo.NetworkSettings.Networks {
				if netEndpoint.NetworkID != "" {
					networkIDs = append(networkIDs, netEndpoint.NetworkID)
				}
			}
			if len(networkIDs) > 0 {
				s.logger.Info("attempting to remove networks",
					zap.String("container", id),
					zap.Strings("networks", networkIDs))
				if err := s.docker.SafeRemoveNetworks(ctx, networkIDs); err != nil {
					// 忽略网络删除错误，只记录日志
					s.logger.Warn("failed to remove some networks",
						zap.String("container", id),
						zap.Strings("networks", networkIDs),
						zap.Error(err))
				} else {
					s.logger.Info("networks removed successfully",
						zap.String("container", id),
						zap.Strings("networks", networkIDs))
				}
			}
		}

		c.JSON(http.StatusOK, NewSuccessRes(nil))
	}
}

// handleExportContainer 处理容器导出
func (s *Server) handleExportContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		if containerID == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "container id required"))
			return
		}

		ctx := c.Request.Context()

		// 获取容器信息以生成文件名
		containerInfo, err := s.docker.InspectContainer(ctx, containerID)
		if err != nil {
			s.logger.Error("inspect container for export", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "容器不存在: "+containerID))
			return
		}

		s.logger.Info("exporting container", zap.String("containerID", containerID), zap.String("containerName", containerInfo.Name))

		// 导出容器
		reader, err := s.docker.ExportContainer(ctx, containerID)
		if err != nil {
			s.logger.Error("export container failed", zap.String("containerID", containerID), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "导出容器失败: "+err.Error()))
			return
		}
		defer reader.Close()

		// 生成文件名
		filename := generateContainerFileName(&containerInfo)

		s.logger.Info("starting container export", zap.String("containerID", containerID), zap.String("filename", filename))

		// 设置响应头
		c.Header("Content-Type", "application/x-tar")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Header("Content-Transfer-Encoding", "binary")

		// 流式传输文件
		_, err = io.Copy(c.Writer, reader)
		if err != nil {
			s.logger.Error("copy container tar stream", zap.String("containerID", containerID), zap.Error(err))
			return
		}
		s.logger.Info("container export completed", zap.String("containerID", containerID))
	}
}

// handleStatsWebSocket 处理容器统计 WebSocket 连接
func (s *Server) handleStatsWebSocket() gin.HandlerFunc {
	return s.wsStatsManager.HandleStatsWebSocket
}

func (s *Server) handleUpdateAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.scheduler.RunScanAndUpdate(c.Request.Context())
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
	}
}

// generateContainerFileName 根据容器信息生成导出文件名
func generateContainerFileName(container *container.InspectResponse) string {
	// 获取容器名称（去掉前缀斜杠）
	containerName := strings.TrimPrefix(container.Name, "/")

	if containerName != "" {
		// 替换不合法的文件名字符
		filename := strings.ReplaceAll(containerName, ":", "_")
		filename = strings.ReplaceAll(filename, "/", "_")
		filename = strings.ReplaceAll(filename, " ", "_")
		return fmt.Sprintf("container_%s.tar", filename)
	}

	// 如果没有名称，使用容器 ID 的前12位
	shortID := container.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	return fmt.Sprintf("container_%s.tar", shortID)
}

// handlePruneSystem 清理悬挂的文件系统、网络、镜像等
func (s *Server) handlePruneSystem() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()

		s.logger.Info("starting system prune")

		if err := s.docker.PruneSystem(ctx); err != nil {
			s.logger.Error("prune system failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "系统清理失败: "+err.Error()))
			return
		}

		s.logger.Info("system prune completed successfully")
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true, "message": "系统清理完成"}))
	}
}

// handleImportContainer 处理容器导入
func (s *Server) handleImportContainer() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 解析multipart/form-data
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			s.logger.Error("get upload file", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "获取上传文件失败"))
			return
		}
		defer file.Close()

		// 验证文件类型（可选）
		if header.Size == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "文件为空"))
			return
		}

		// 获取导入参数
		repository := c.DefaultPostForm("repository", "imported-container")
		tag := c.DefaultPostForm("tag", "latest")

		s.logger.Info("starting container import",
			zap.String("filename", header.Filename),
			zap.Int64("size", header.Size),
			zap.String("repository", repository),
			zap.String("tag", tag))

		ctx := c.Request.Context()

		// 导入容器（实际上是导入为镜像）
		err = s.docker.ImportImage(ctx, file, repository, tag)
		if err != nil {
			s.logger.Error("import container failed",
				zap.String("filename", header.Filename),
				zap.String("repository", repository),
				zap.String("tag", tag),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "导入容器失败: "+err.Error()))
			return
		}

		s.logger.Info("container import completed",
			zap.String("filename", header.Filename),
			zap.String("repository", repository),
			zap.String("tag", tag))
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"message": "容器导入成功"}))
	}
}

// handleListContainerFiles 列出容器文件
func (s *Server) handleListContainerFiles() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.DefaultQuery("path", "/")

		result, err := s.docker.ListContainerDirectory(c.Request.Context(), containerID, path)
		if err != nil {
			s.logger.Error("list container files failed", zap.Error(err), zap.String("container", containerID), zap.String("path", path))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(result))
	}
}

// handleGetContainerFileContent 读取容器文件内容
func (s *Server) handleGetContainerFileContent() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.Query("path")
		method := c.Query("method") // 可选：cat, archive, auto

		if path == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "path is required"))
			return
		}

		var content string
		var err error

		// 根据 method 参数选择读取方式
		switch method {
		case "archive":
			// 强制使用 Archive API
			content, err = s.docker.ReadContainerFileFromArchive(c.Request.Context(), containerID, path)
		case "auto", "":
			// 智能选择（Archive API + 自动降级）
			content, err = s.docker.ReadContainerFileAuto(c.Request.Context(), containerID, path)
		case "cat":
			// 默认使用 cat 命令（兼容性最好）
			content, err = s.docker.ReadContainerFile(c.Request.Context(), containerID, path)
		default:
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid method, use: cat, archive, or auto"))
			return
		}

		if err != nil {
			s.logger.Error("read container file failed",
				zap.String("path", path),
				zap.String("method", method),
				zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		// 检查文件大小（限制 1MB）
		// if len(content) > 1024*1024 {
		// 	c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "文件太大（最大 1MB）"))
		// 	return
		// }

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"content": content,
			"path":    path,
		}))
	}
}

// handleUpdateContainerFileContent 更新容器文件内容
func (s *Server) handleUpdateContainerFileContent() gin.HandlerFunc {
	type UpdateFileRequest struct {
		Content string `json:"content"`
	}

	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.Query("path")

		if path == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "path is required"))
			return
		}

		var req UpdateFileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request"))
			return
		}

		// 检查内容大小（限制 1MB）
		// if len(req.Content) > 1024*1024 {
		// 	c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "content is too large (max 1MB)"))
		// 	return
		// }

		err := s.docker.WriteContainerFile(c.Request.Context(), containerID, path, req.Content)
		if err != nil {
			s.logger.Error("write container file failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"success": true,
			"path":    path,
		}))
	}
}

// handleUploadContainerFiles 处理多文件上传（流式打包为 tar 上传，避免整文件读入内存）
func (s *Server) handleUploadContainerFiles() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.DefaultQuery("path", "/")

		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid form data"))
			return
		}

		files := form.File["files"]
		if len(files) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "no files provided"))
			return
		}

		uploaded := []string{}
		errors := []string{}

		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to open", fileHeader.Filename))
				continue
			}

			// 流式上传到容器
			err = s.docker.PutSingleFileStream(c.Request.Context(), containerID, path, fileHeader.Filename, fileHeader.Size, file)
			file.Close()
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", fileHeader.Filename, err))
				continue
			}

			uploaded = append(uploaded, fileHeader.Filename)
		}

		if len(errors) > 0 && len(uploaded) == 0 {
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, "failed to upload files"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"uploaded": uploaded,
			"errors":   errors,
		}))
	}
}

// handleGenerateDownloadToken 生成下载令牌
func (s *Server) handleGenerateDownloadToken() gin.HandlerFunc {
	type GenerateTokenRequest struct {
		Path string `json:"path" binding:"required"`
	}

	return func(c *gin.Context) {
		containerID := c.Param("id")

		var req GenerateTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request"))
			return
		}

		// 获取当前用户（从认证中间件设置）
		username, exists := c.Get("username")
		if !exists {
			username = "anonymous"
		}

		// 生成下载令牌（有效期 60 秒）
		tokenManager := auth.GetDownloadTokenManager()
		token, err := tokenManager.GenerateDownloadToken(containerID, req.Path, username.(string), 60*time.Second)
		if err != nil {
			s.logger.Error("generate download token failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeInternalError, "生成下载令牌失败"))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"token":     token,
			"expiresIn": 60, // 秒
		}))
	}
}

// handleDownloadContainerFile 处理文件下载（支持临时 token 和常规 token）
func (s *Server) handleDownloadContainerFile() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.Query("path")
		downloadToken := c.Query("download_token")

		if path == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "path is required"))
			return
		}

		// 优先使用下载令牌验证（原子性消费，避免 TOCTOU 竞态重放）
		if downloadToken != "" {
			tokenManager := auth.GetDownloadTokenManager()
			if err := tokenManager.ConsumeDownloadToken(downloadToken, containerID, path); err != nil {
				s.logger.Warn("invalid download token", zap.Error(err))
				c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "无效或已过期的下载令牌"))
				return
			}
		} else if auth.IsAuthEnabled() {
			token, err := auth.ExtractTokenFromRequest(c)
			if err != nil {
				c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "需要登录"))
				return
			}
			// 仅接受完整访问令牌，拒绝 2FA 临时令牌直接下载容器文件
			if _, err := auth.ValidateFullAccessClaims(token); err != nil {
				c.JSON(http.StatusUnauthorized, NewErrorResCode(CodeUnauthorized, "无效的token"))
				return
			}
		}

		// 执行下载
		reader, err := s.docker.GetContainerArchive(c.Request.Context(), containerID, path)
		if err != nil {
			s.logger.Error("get container archive failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}
		defer reader.Close()

		// 获取文件名（逐字保留空格，仅过滤头注入字符）
		filename := strings.NewReplacer("\"", "_", "\r", "", "\n", "").Replace(filepath.Base(path))

		// 设置响应头
		c.Header("Content-Type", "application/x-tar")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.tar\"", filename))

		// 流式传输
		_, err = io.Copy(c.Writer, reader)
		if err != nil {
			s.logger.Error("stream archive failed", zap.Error(err))
		}
	}
}

// handleCreateContainerPath 处理创建文件/目录
func (s *Server) handleCreateContainerPath() gin.HandlerFunc {
	type CreateRequest struct {
		Path string `json:"path" binding:"required"`
		Type string `json:"type" binding:"required,oneof=file directory"`
	}

	return func(c *gin.Context) {
		containerID := c.Param("id")

		var req CreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request"))
			return
		}

		var err error
		if req.Type == "file" {
			err = s.docker.CreateContainerFile(c.Request.Context(), containerID, req.Path)
		} else {
			err = s.docker.CreateContainerDirectory(c.Request.Context(), containerID, req.Path)
		}

		if err != nil {
			s.logger.Error("create path failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"success": true,
			"path":    req.Path,
			"type":    req.Type,
		}))
	}
}

// handleDeleteContainerPath 处理删除文件/目录
func (s *Server) handleDeleteContainerPath() gin.HandlerFunc {
	return func(c *gin.Context) {
		containerID := c.Param("id")
		path := c.Query("path")

		if path == "" {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "path is required"))
			return
		}

		err := s.docker.DeleteContainerPath(c.Request.Context(), containerID, path)
		if err != nil {
			s.logger.Error("delete path failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"success": true,
		}))
	}
}

// handleRenameContainerPath 处理重命名文件/目录
func (s *Server) handleRenameContainerPath() gin.HandlerFunc {
	type RenameRequest struct {
		OldPath string `json:"oldPath" binding:"required"`
		NewPath string `json:"newPath" binding:"required"`
	}

	return func(c *gin.Context) {
		containerID := c.Param("id")

		var req RenameRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request"))
			return
		}

		err := s.docker.RenameContainerPath(c.Request.Context(), containerID, req.OldPath, req.NewPath)
		if err != nil {
			s.logger.Error("rename path failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"success": true,
		}))
	}
}

// handleChmodContainerPath 处理修改文件权限
func (s *Server) handleChmodContainerPath() gin.HandlerFunc {
	type ChmodRequest struct {
		Path      string `json:"path" binding:"required"`
		Mode      string `json:"mode" binding:"required"`
		Recursive bool   `json:"recursive"`
	}

	return func(c *gin.Context) {
		containerID := c.Param("id")

		var req ChmodRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, NewErrorResCode(CodeBadRequest, "invalid request"))
			return
		}

		err := s.docker.ChmodContainerPath(c.Request.Context(), containerID, req.Path, req.Mode, req.Recursive)
		if err != nil {
			s.logger.Error("chmod failed", zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(CodeDockerError, err.Error()))
			return
		}

		c.JSON(http.StatusOK, NewSuccessRes(gin.H{
			"success": true,
		}))
	}
}

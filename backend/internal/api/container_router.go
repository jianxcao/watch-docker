package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
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
	protected.POST("/containers/:id/stop", s.handleStopContainer())
	protected.POST("/containers/:id/start", s.handleStartContainer())
	protected.POST("/containers/:id/restart", s.handleRestartContainer())
	protected.DELETE("/containers/:id", s.handleDeleteContainer())
	protected.GET("/containers/:id/export", s.handleExportContainer())
	protected.POST("/containers/import", s.handleImportContainer())
	protected.POST("/system/prune", s.handlePruneSystem())
	protected.GET("/update/all", s.handleUpdateAll())
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
			config.ExposedPorts = make(nat.PortSet)
			for port := range req.ExposedPorts {
				p, err := nat.NewPort("tcp", port)
				if err == nil {
					config.ExposedPorts[p] = struct{}{}
				}
			}
		}

		// 构建主机配置
		hostConfig := &container.HostConfig{
			Binds:           req.Binds,                                                                                                                                  // 数据卷绑定
			RestartPolicy:   container.RestartPolicy{Name: container.RestartPolicyMode(req.RestartPolicy.Name), MaximumRetryCount: req.RestartPolicy.MaximumRetryCount}, // 重启策略
			AutoRemove:      req.AutoRemove,                                                                                                                             // 容器退出时自动删除
			NetworkMode:     container.NetworkMode(req.NetworkMode),                                                                                                     // 网络模式
			Privileged:      req.Privileged,                                                                                                                             // 特权模式
			PublishAllPorts: req.PublishAllPorts,                                                                                                                        // 发布所有端口
			ReadonlyRootfs:  req.ReadonlyRootfs,                                                                                                                         // 只读根文件系统
			DNS:             req.Dns,                                                                                                                                    // DNS 服务器
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

		// 设置端口绑定(将主机端口映射到容器端口)
		if len(req.PortBindings) > 0 {
			hostConfig.PortBindings = make(nat.PortMap)
			for portStr, bindings := range req.PortBindings {
				port, err := nat.NewPort("tcp", portStr)
				if err != nil {
					// 尝试解析端口字符串(可能包含协议,格式: 80/tcp)
					parts := strings.Split(portStr, "/")
					if len(parts) == 2 {
						port, err = nat.NewPort(parts[1], parts[0])
					}
				}
				if err == nil {
					var portBindings []nat.PortBinding
					for _, binding := range bindings {
						portBindings = append(portBindings, nat.PortBinding{
							HostIP:   binding.HostIP,   // 主机 IP 地址
							HostPort: binding.HostPort, // 主机端口
						})
					}
					hostConfig.PortBindings[port] = portBindings
				}
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

		// 构建网络配置(配置容器连接到的网络)
		var networkConfig *network.NetworkingConfig
		if req.NetworkConfig != nil && req.NetworkConfig.EndpointsConfig != nil {
			networkConfig = &network.NetworkingConfig{
				EndpointsConfig: make(map[string]*network.EndpointSettings),
			}
			for netName, endpoint := range req.NetworkConfig.EndpointsConfig {
				endpointSettings := &network.EndpointSettings{
					Links:               endpoint.Links,               // 容器链接
					Aliases:             endpoint.Aliases,             // 网络别名
					NetworkID:           endpoint.NetworkID,           // 网络 ID
					EndpointID:          endpoint.EndpointID,          // 端点 ID
					Gateway:             endpoint.Gateway,             // 网关地址
					IPAddress:           endpoint.IPAddress,           // IPv4 地址
					IPPrefixLen:         endpoint.IPPrefixLen,         // IPv4 前缀长度
					IPv6Gateway:         endpoint.IPv6Gateway,         // IPv6 网关
					GlobalIPv6Address:   endpoint.GlobalIPv6Address,   // 全局 IPv6 地址
					GlobalIPv6PrefixLen: endpoint.GlobalIPv6PrefixLen, // IPv6 前缀长度
					MacAddress:          endpoint.MacAddress,          // MAC 地址
				}
				if endpoint.IPAMConfig != nil {
					endpointSettings.IPAMConfig = &network.EndpointIPAMConfig{
						IPv4Address: endpoint.IPAMConfig.IPv4Address, // 自定义 IPv4 地址
						IPv6Address: endpoint.IPAMConfig.IPv6Address, // 自定义 IPv6 地址
					}
				}
				networkConfig.EndpointsConfig[netName] = endpointSettings
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
		if body.Image == "" {
			// try inspect to get current image
			info, err := s.docker.InspectContainer(c.Request.Context(), id)
			if err != nil {
				s.logger.Error("inspect", zap.Error(err))
				c.JSON(http.StatusOK, NewErrorResCode(CodeImageRequired, "image required"))
				return
			}
			body.Image = info.Config.Image
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
		defer cancel()
		if err := s.updater.UpdateContainer(ctx, id, body.Image); err != nil {
			s.logger.Error("update container", zap.String("container", id), zap.String("image", body.Image), zap.Error(err))
			c.JSON(http.StatusOK, NewErrorResCode(codeForUpdateErr(err), err.Error()))
			return
		}
		c.JSON(http.StatusOK, NewSuccessRes(gin.H{"ok": true}))
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
			if st.Skipped || st.Status != "UpdateAvailable" {
				continue
			}
			if !st.Running && !cfg.Docker.IncludeStopped {
				continue
			}
			uctx, cancelOne := context.WithTimeout(ctx, 5*time.Minute)
			if err := s.updater.UpdateContainer(uctx, st.ID, st.Image); err != nil {
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
			for _, mount := range containerInfo.Mounts {
				if mount.Type == "volume" && mount.Name != "" {
					volumeNames = append(volumeNames, mount.Name)
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

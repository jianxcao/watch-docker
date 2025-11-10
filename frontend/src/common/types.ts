// 基础响应类型
export interface BaseResponse<T = any> {
  code: number
  msg: string
  data: T
}

// 系统信息类型
export interface SystemInfo {
  dockerVersion: string
  dockerAPIVersion: string
  dockerPlatform: string
  dockerGitCommit: string
  dockerGoVersion: string
  dockerBuildTime: string
  version: string
  appPath: string
  isOpenDockerShell: boolean
  isSecondaryVerificationEnabled?: boolean
}
// 容器资源统计信息
export interface ContainerStats {
  id: string
  name: string
  cpuPercent: number
  memoryUsage: number // 字节
  memoryLimit: number // 字节
  memoryPercent: number
  networkRxRate: number // 网络接收速率（字节/秒）
  networkTxRate: number // 网络发送速率（字节/秒）
  networkRx: number // 总接收字节数
  networkTx: number // 总发送字节数
  blockRead: number // 字节
  blockWrite: number // 字节
  pidsCurrent: number
  pidsLimit: number
}

// 详细的容器统计信息（Docker API 原始格式）
export interface ContainerDetailStats {
  id: string
  name: string
  // 当前读取时间 2025-11-06T06:34:06.577815611Z
  read: string
  preread: string
  pids_stats: {
    current: number
    limit: number
  }
  blkio_stats: {
    io_service_bytes_recursive: Array<{
      major: number
      minor: number
      op: string
      value: number
    }> | null
    io_serviced_recursive: any
    io_queue_recursive: any
    io_service_time_recursive: any
    io_wait_time_recursive: any
    io_merged_recursive: any
    io_time_recursive: any
    sectors_recursive: any
  }
  cpu_stats: {
    cpu_usage: {
      total_usage: number
      usage_in_kernelmode: number
      usage_in_usermode: number
      percpu_usage?: number[]
    }
    system_cpu_usage: number
    online_cpus: number
    throttling_data: {
      periods: number
      throttled_periods: number
      throttled_time: number
    }
  }
  precpu_stats: {
    cpu_usage: {
      total_usage: number
      usage_in_kernelmode: number
      usage_in_usermode: number
      percpu_usage?: number[]
    }
    system_cpu_usage: number
    online_cpus: number
    throttling_data: {
      periods: number
      throttled_periods: number
      throttled_time: number
    }
  }
  memory_stats: {
    usage: number
    max_usage?: number
    stats: {
      active_anon?: number
      active_file?: number
      anon?: number
      anon_thp?: number
      file?: number
      file_dirty?: number
      file_mapped?: number
      file_writeback?: number
      inactive_anon?: number
      inactive_file?: number
      kernel_stack?: number
      pgactivate?: number
      pgdeactivate?: number
      pgfault?: number
      pglazyfree?: number
      pglazyfreed?: number
      pgmajfault?: number
      pgrefill?: number
      pgscan?: number
      pgsteal?: number
      shmem?: number
      slab?: number
      slab_reclaimable?: number
      slab_unreclaimable?: number
      sock?: number
      thp_collapse_alloc?: number
      thp_fault_alloc?: number
      unevictable?: number
      workingset_activate?: number
      workingset_nodereclaim?: number
      workingset_refault?: number
      cache?: number
    }
    limit: number
    commitlimit?: number
    committed_as?: number
    failcnt?: number
  }
  // host 模式下回没有 networks
  networks?: {
    [key: string]: {
      rx_bytes: number
      rx_packets: number
      rx_errors: number
      rx_dropped: number
      tx_bytes: number
      tx_packets: number
      tx_errors: number
      tx_dropped: number
    }
  }
  num_procs?: number
  storage_stats?: any
}

// 端口映射信息
export interface PortInfo {
  ip: string
  privatePort: number
  publicPort: number
  type: string
}

// 容器状态类型
export interface ContainerStatus {
  id: string
  name: string
  image: string
  running: boolean
  status: 'UpToDate' | 'UpdateAvailable' | 'Skipped' | 'Error' | ''
  skipped: boolean
  skipReason: string
  labels: Record<string, string>
  lastCheckedAt: string
  startedAt: string // 容器启动时间
  ports: PortInfo[] // 端口映射信息
  stats?: ContainerStats // 可选的资源统计信息
}

// 镜像信息类型
export interface ImageInfo {
  id: string
  repoTags: string[]
  repoDigests: string[]
  size: number
  created: number
}

// 注册表认证配置
export interface RegistryAuth {
  host: string
  username: string
  token: string
}

// 配置类型
export interface Config {
  server: {
    addr: string
  }
  notify: {
    url: string
    method: 'GET' | 'POST'
    isEnable: boolean
  }
  docker: {
    host: string
    includeStopped: boolean
  }
  scan: {
    cron: string
    concurrency: number
    cacheTTL: number // 分钟数
    isUpdate: boolean
    allowComposeUpdate: boolean
  }
  policy: {
    skipLabels: string[]
    onlyLabels: string[]
    skipLocalBuild: boolean
    skipPinnedDigest: boolean
    skipSemverPinned: boolean
    floatingTags: string[]
  }
  registry: {
    auth: RegistryAuth[]
  }
  logging: {
    level: string
  }
}

// 批量更新结果
export interface BatchUpdateResult {
  updated: string[]
  failed: Record<string, string>
  failedCodes: Record<string, number>
}

// 菜单项类型
export interface MenuItem {
  key: string
  label: string
  icon?: string
  path: string
}

// 容器操作类型
export type ContainerAction = 'start' | 'stop' | 'update' | 'delete'

// 状态类型
export type ContainerStatusType = 'UpToDate' | 'UpdateAvailable' | 'Skipped' | 'Error'
export type ContainerState = 'running' | 'stopped' | 'paused' | 'restarting' | 'dead'

// Compose 端口映射信息
export interface ComposePortMapping {
  hostPort: number
  containerPort: number
  protocol: string
}

// Compose 服务信息
export interface ComposeService {
  name: string
  image: string
  status: string
  containerId: string
  ports: ComposePortMapping[]
  environment: Record<string, string>
  dependsOn: string[]
  replicas: number
}

// Compose 网络信息
export interface ComposeNetwork {
  name: string
  driver: string
  external: boolean
}

// Compose 卷信息
export interface ComposeVolume {
  name: string
  driver: string
  external: boolean
}

// Compose 项目信息
export interface ComposeProject {
  name: string
  composeFile: string
  status: ComposeProjectStatus
  runningCount: number
  exitedCount: number
  createdCount: number
}

// Compose 操作类型
export type ComposeAction = 'start' | 'stop' | 'restart' | 'delete' | 'create'

// Compose 项目状态类型
export type ComposeProjectStatus =
  | 'running'
  | 'exited'
  | 'partial'
  | 'draft'
  | 'created_stack'
  | 'unknown'

// ============= Volume 相关类型 =============

// Volume使用数据
export interface VolumeUsageData {
  size: number // 字节
  refCount: number // 引用计数
}

// Volume信息类型
export interface VolumeInfo {
  name: string
  driver: string
  mountpoint: string
  createdAt: string
  labels: Record<string, string>
  scope: string
  options: Record<string, string>
  status: Record<string, any>
  usageData?: VolumeUsageData
}

// Volume列表响应
export interface VolumeListResponse {
  volumes: VolumeInfo[]
  totalCount: number
  totalSize: number
  usedCount: number
  unusedCount: number
}

// 容器引用信息
export interface ContainerRef {
  id: string
  name: string
  image: string
  running: boolean
  destination: string // 容器内挂载路径
  mode: string // 读写模式
}

// Volume详情响应
export interface VolumeDetailResponse {
  volume: VolumeInfo
  containers?: ContainerRef[] | null
}

// Volume统计信息
export interface VolumeStats {
  total: number
  used: number
  unused: number
  totalSize: number
  formattedTotalSize: string
}

// Volume创建请求
export interface VolumeCreateRequest {
  name: string
  driver?: string
  driverOpts?: Record<string, string>
  labels?: Record<string, string>
}

// Volume清理响应
export interface VolumePruneResponse {
  volumesDeleted: string[]
  spaceReclaimed: number
}

// ============= Network 相关类型 =============

// 网络 IPAM 配置
export interface NetworkIPAMConfig {
  subnet?: string
  gateway?: string
  ipRange?: string
  auxAddress?: Record<string, string>
}

// 网络 IPAM 信息
export interface NetworkIPAM {
  driver: string
  options?: Record<string, string>
  config?: NetworkIPAMConfig[]
}

// 网络信息类型
export interface NetworkInfo {
  id: string
  name: string
  driver: string
  scope: string
  internal: boolean
  attachable: boolean
  ingress: boolean
  enableIPv6: boolean
  ipam: NetworkIPAM
  created: string
  labels: Record<string, string>
  options: Record<string, string>
  containerCount: number
}

// 网络容器信息
export interface NetworkContainer {
  id: string
  name: string
  image: string
  running: boolean
  ipv4Address?: string
  ipv6Address?: string
  macAddress?: string
  endpointId?: string
}

// 网络列表响应
export interface NetworkListResponse {
  networks: NetworkInfo[]
  totalCount: number
  usedCount: number
  unusedCount: number
  builtInCount: number
  customCount: number
}

// 网络详情响应
export interface NetworkDetailResponse {
  network: NetworkInfo
  containers: NetworkContainer[]
}

// 网络统计信息
export interface NetworkStats {
  total: number
  used: number
  unused: number
  builtIn: number
  custom: number
}

// 网络 IPAM 创建配置
export interface NetworkIPAMConfigCreate {
  subnet?: string
  ipRange?: string
  gateway?: string
  auxAddress?: Record<string, string>
}

// 网络 IPAM 创建请求
export interface NetworkIPAMCreateRequest {
  driver?: string
  config?: NetworkIPAMConfigCreate[]
  options?: Record<string, string>
}

// 网络创建请求
export interface NetworkCreateRequest {
  name: string
  driver?: string
  scope?: string
  internal?: boolean
  attachable?: boolean
  ingress?: boolean
  enableIPv6?: boolean
  ipam?: NetworkIPAMCreateRequest
  options?: Record<string, string>
  labels?: Record<string, string>
}

// 网络清理响应
export interface NetworkPruneResponse {
  networksDeleted: string[]
}

// 网络连接容器请求
export interface NetworkConnectRequest {
  container: string
  ipv4Address?: string
  ipv6Address?: string
  links?: string[]
  aliases?: string[]
  driverOpts?: Record<string, string>
}

// 网络断开容器请求
export interface NetworkDisconnectRequest {
  container: string
  force: boolean
}

// ============= Container Detail 相关类型 =============

// 容器端口绑定
export interface ContainerPortBinding {
  HostIp: string
  HostPort: string
}

// 容器挂载点
export interface ContainerMount {
  Type: string // "bind" | "volume" | "tmpfs"
  Name?: string
  Source: string
  Destination: string
  Driver?: string
  Mode: string
  RW: boolean
  Propagation: string
}

// 容器网络设置
export interface ContainerNetworkSettings {
  Bridge: string
  Gateway: string
  IPAddress: string
  IPPrefixLen: number
  MacAddress: string
  Networks: Record<string, ContainerNetworkEndpoint>
  Ports: Record<string, ContainerPortBinding[] | null>
}

// 容器网络端点
export interface ContainerNetworkEndpoint {
  IPAMConfig: any
  Links: string[] | null
  Aliases: string[] | null
  NetworkID: string
  EndpointID: string
  Gateway: string
  IPAddress: string
  IPPrefixLen: number
  IPv6Gateway: string
  GlobalIPv6Address: string
  GlobalIPv6PrefixLen: number
  MacAddress: string
  DriverOpts: Record<string, string> | null
}

// 容器状态
export interface ContainerStateDetail {
  Status: string
  Running: boolean
  Paused: boolean
  Restarting: boolean
  OOMKilled: boolean
  Dead: boolean
  Pid: number
  ExitCode: number
  Error: string
  StartedAt: string
  FinishedAt: string
}

// 容器配置
export interface ContainerConfig {
  Hostname: string
  Domainname: string
  User: string
  AttachStdin: boolean
  AttachStdout: boolean
  AttachStderr: boolean
  Tty: boolean
  OpenStdin: boolean
  StdinOnce: boolean
  Env: string[]
  Cmd: string[] | null
  Image: string
  Volumes: Record<string, any> | null
  WorkingDir: string
  Entrypoint: string[] | null
  OnBuild: string[] | null
  Labels: Record<string, string>
  ExposedPorts: Record<string, any> | null
}

// 容器主机配置
export interface ContainerHostConfig {
  Binds: string[] | null
  NetworkMode: string
  PortBindings: Record<string, ContainerPortBinding[] | null>
  RestartPolicy: {
    Name: string
    MaximumRetryCount: number
  }
  AutoRemove: boolean
  VolumeDriver: string
  VolumesFrom: string[] | null
  CapAdd: string[] | null
  CapDrop: string[] | null
  Dns: string[] | null
  DnsOptions: string[] | null
  DnsSearch: string[] | null
  ExtraHosts: string[] | null
  GroupAdd: string[] | null
  IpcMode: string
  Cgroup: string
  Links: string[] | null
  OomScoreAdj: number
  PidMode: string
  Privileged: boolean
  PublishAllPorts: boolean
  ReadonlyRootfs: boolean
  SecurityOpt: string[] | null
  UTSMode: string
  UsernsMode: string
  ShmSize: number
  Runtime: string
  ConsoleSize: [number, number]
  Isolation: string
  CpuShares: number
  Memory: number
  CpusetCpus: string
  CpusetMems: string
  CpuQuota: number
  CpuPeriod: number
  BlkioWeight: number
}

// 容器详情
export interface ContainerDetail {
  Id: string
  Created: string
  Path: string
  Args: string[]
  State: ContainerStateDetail
  Image: string
  ResolvConfPath: string
  HostnamePath: string
  HostsPath: string
  LogPath: string
  Name: string
  RestartCount: number
  Driver: string
  Platform: string
  MountLabel: string
  ProcessLabel: string
  AppArmorProfile: string
  ExecIDs: string[] | null
  HostConfig: ContainerHostConfig
  GraphDriver: {
    Name: string
    Data: Record<string, string>
  }
  Mounts: ContainerMount[]
  Config: ContainerConfig
  NetworkSettings: ContainerNetworkSettings
}

// 容器详情响应
export interface ContainerDetailResponse {
  container: ContainerDetail
}

// 端口绑定
export interface PortBinding {
  hostIP: string
  hostPort: string
}

// 重启策略类型
export type RestartPolicyType = 'no' | 'always' | 'unless-stopped' | 'on-failure'

// 重启策略
export interface RestartPolicyConfig {
  name: RestartPolicyType
  maximumRetryCount: number
}

// 设备映射
export interface DeviceMapping {
  pathOnHost: string
  pathInContainer: string
  cgroupPermissions: string
}

// GPU 等设备请求
export interface DeviceRequest {
  driver: string
  count: number
  deviceIDs: string[]
  capabilities: string[][]
  options: Record<string, string>
}

// 端点 IPAM 配置
export interface EndpointIPAMConfig {
  ipv4Address: string
  ipv6Address: string
  linkLocalIPs?: string[]
  ipv4Gateway?: string
  ipv6Gateway?: string
}

// 端点设置
export interface EndpointSettings {
  ipamConfig?: EndpointIPAMConfig
  links: string[]
  aliases: string[]
  networkID: string
  endpointID: string
  gateway: string
  ipAddress: string
  ipPrefixLen: number
  ipv6Gateway: string
  globalIPv6Address: string
  globalIPv6PrefixLen: number
  macAddress: string
}

// 网络配置
export interface NetworkConfigCreate {
  endpointsConfig: Record<string, EndpointSettings>
}

// 待创建的网络 IPAM 配置项
export interface NetworkIPAMConfigCreate {
  subnet?: string
  ipRange?: string
  gateway?: string
  auxAddress?: Record<string, string>
}

// 待创建的网络 IPAM 配置
export interface NetworkIPAMCreateRequest {
  driver?: string
  config?: NetworkIPAMConfigCreate[]
  options?: Record<string, string>
}

// 待创建的网络配置
export interface NetworkToCreate {
  name: string
  driver?: string
  enableIPv6?: boolean
  ipam?: NetworkIPAMCreateRequest
  internal?: boolean
  attachable?: boolean
  labels?: Record<string, string>
  options?: Record<string, string>
}

// 容器创建请求
export interface ContainerCreateRequest {
  name: string
  image: string
  cmd?: string[]
  entrypoint?: string[]
  workingDir?: string
  env?: string[]
  exposedPorts?: Record<string, Record<string, never>>
  labels?: Record<string, string>
  hostname?: string
  domainname?: string
  user?: string
  attachStdin?: boolean
  attachStdout?: boolean
  attachStderr?: boolean
  tty?: boolean
  openStdin?: boolean
  stdinOnce?: boolean
  binds?: string[]
  portBindings?: Record<string, PortBinding[]>
  restartPolicy?: RestartPolicyConfig
  autoRemove?: boolean
  networkMode?: string
  privileged?: boolean
  publishAllPorts?: boolean
  readonlyRootfs?: boolean
  dns?: string[]
  dnsSearch?: string[]
  dnsOptions?: string[]
  extraHosts?: string[]
  capAdd?: string[]
  capDrop?: string[]
  securityOpt?: string[]
  cpuShares?: number
  memory?: number
  memoryReservation?: number
  cpuQuota?: number
  cpuPeriod?: number
  cpusetCpus?: string
  cpusetMems?: string
  blkioWeight?: number
  shmSize?: number
  pidMode?: string
  ipcMode?: string
  utsMode?: string
  cgroup?: string
  runtime?: string
  devices?: DeviceMapping[]
  deviceRequests?: DeviceRequest[]
  networkConfig?: NetworkConfigCreate
  networksToCreate?: NetworkToCreate[] // 新增：需要创建的网络列表
}

// 容器创建响应
export interface ContainerCreateResponse {
  id: string
  message: string
}

import type { PortBinding } from '@/common/types'

/**
 * 基础配置表单
 */
export interface BasicFormValue {
  name: string
  image: string
  entrypointString: string
  cmdString: string
  workingDir: string
  user: string
  hostname: string
  domainname: string
  tty: boolean
  openStdin: boolean
  stdinOnce: boolean
}

/**
 * 环境变量项
 */
export interface EnvItem {
  key: string
  value: string
}

/**
 * 环境变量表单
 */
export interface EnvFormValue {
  env: string[]
  envList: EnvItem[]
  envText: string
}

/**
 * 端口映射项
 */
export interface PortItem {
  hostPort: number | null
  containerPort: number | null
  protocol: ('tcp' | 'udp')[]
}

/**
 * 端口配置表单
 */
export interface PortFormValue {
  portList: PortItem[]
  portBindings: Record<string, PortBinding[]>
  publishAllPorts: boolean
  exposedPorts?: Record<string, any>
}

/**
 * 数据卷项
 */
export interface VolumeItem {
  source: string
  target: string
  readonly: boolean
}

/**
 * 数据卷表单
 */
export interface VolumeFormValue {
  binds: string[]
  volumeList: VolumeItem[]
  volumeText: string
}

/**
 * 网络端点配置项
 */
export interface NetworkEndpointItem {
  networkName: string
  ipv4Address: string
  ipv4Gateway: string
  ipv6Address: string
  ipv6Gateway: string
  macAddress: string
  aliases: string[]
}

/**
 * 网络配置模式
 */
export type NetworkConfigMode = 'default' | 'custom'

/**
 * 自定义网络配置
 */
export interface CustomNetworkConfig {
  // 基础信息
  name: string // 网络名称（必填）
  exists?: boolean // 网络是否已存在（前端自动检测）

  // 网络创建配置（仅在网络不存在时使用）
  driver?: 'bridge' | 'overlay' | 'macvlan' | 'host' | 'none'
  parentInterface?: string // macvlan/ipvlan 的父网络接口（例如：eth0）
  enableIPv6?: boolean
  ipv4Subnet?: string
  ipv4Gateway?: string
  ipv6Subnet?: string
  ipv6Gateway?: string
  internal?: boolean
  attachable?: boolean

  // 容器连接配置（连接到网络时的配置，总是需要）
  containerIPv4Address?: string // 容器的静态 IPv4 地址
  containerIPv6Address?: string // 容器的静态 IPv6 地址
  macAddress?: string // 容器的 MAC 地址
  aliases?: string[] // 网络别名
}

/**
 * 网络配置表单
 */
export interface NetworkFormValue {
  configMode: NetworkConfigMode // 配置模式
  publishAllPorts: boolean
  dns: string[]
  dnsSearch: string[]
  dnsOptions: string[]
  extraHosts: string[]
  customNetworks?: CustomNetworkConfig[] // 自定义网络配置列表
}

/**
 * 运行与资源配置表单
 */
export interface RuntimeResourceFormValue {
  privileged: boolean
  readonlyRootfs: boolean
  autoRemove: boolean
  restartPolicyName: string
  restartPolicyMaxRetry: number
  memoryMB: number
  memoryReservationMB: number
  cpusetCpus: string
  shmSizeMB: number
}

/**
 * 标签项
 */
export interface LabelItem {
  key: string
  value: string
}

/**
 * 标签配置表单
 */
export interface LabelFormValue {
  labelList: LabelItem[]
  labels: Record<string, string>
}

/**
 * 高级配置表单
 */
export interface AdvancedFormValue {
  capAdd: string[]
  capDrop: string[]
  pidMode: string
  ipcMode: string
  utsMode: string
  cgroup: string
  runtime: string
  securityOpt: string[]
}

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
  containers: ContainerRef[]
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

// 基础响应类型
export interface BaseResponse<T = any> {
  code: number
  msg: string
  data: T
}

// 容器状态类型
export interface ContainerStatus {
  id: string
  name: string
  image: string
  running: boolean
  currentDigest: string
  remoteDigest: string
  status: 'UpToDate' | 'UpdateAvailable' | 'Skipped' | 'Error'
  skipped: boolean
  skipReason: string
  labels: Record<string, string>
  lastCheckedAt: string
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
  password: string
}

// 配置类型
export interface Config {
  server: {
    addr: string
  }
  docker: {
    host: string
    includeStopped: boolean
  }
  scan: {
    interval: string
    cron: string
    initialScanOnStart: boolean
    concurrency: number
    cacheTTL: string
  }
  update: {
    enabled: boolean
    autoUpdateCron: string
    allowComposeUpdate: boolean
    recreateStrategy: string
    removeOldContainer: boolean
  }
  policy: {
    skipLabels: string[]
    onlyLabels: string[]
    excludeLabels: string[]
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

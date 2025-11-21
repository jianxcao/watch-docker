// API 接口常量
export const API_ENDPOINTS = {
  // 健康检查
  HEALTH: '/healthz',
  READY: '/readyz',

  // 身份验证
  LOGIN: '/login',
  LOGOUT: '/logout',
  AUTH_STATUS: '/auth/status',
  INFO: '/info',

  // 容器相关
  CONTAINERS: '/containers',
  CONTAINER_CREATE: '/containers/create',
  CONTAINER_DETAIL: (id: string) => `/containers/${id}`,
  CONTAINERS_STATS: '/containers/stats',
  CONTAINER_STATS_WS: (id: string) => `/containers/${id}/stats/ws`,
  CONTAINER_UPDATE: (id: string) => `/containers/${id}/update`,
  CONTAINER_START: (id: string) => `/containers/${id}/start`,
  CONTAINER_STOP: (id: string) => `/containers/${id}/stop`,
  CONTAINER_RESTART: (id: string) => `/containers/${id}/restart`,
  CONTAINER_DELETE: (id: string) => `/containers/${id}`,
  CONTAINER_EXPORT: (id: string) => `/containers/${id}/export`,
  CONTAINER_IMPORT: '/containers/import',
  CONTAINER_SHELL_WS: (id: string, shell: string) => `/containers/${id}/shell/ws?shell=${shell}`,

  // 批量操作
  BATCH_UPDATE: '/updates/run',
  PRUNE_SYSTEM: '/system/prune',

  // 镜像相关
  IMAGES: '/images',
  IMAGE_DOWNLOAD: (id: string) => `/images/${id}/download`,
  IMAGE_IMPORT: '/images/import',

  // 配置相关
  CONFIG: '/config',

  // 网络相关
  NETWORKS: '/networks',
  NETWORK_DETAIL: (id: string) => `/networks/${id}`,
  NETWORK_DELETE: (id: string) => `/networks/${id}`,
  NETWORK_PRUNE: '/networks/prune',
  NETWORK_CONNECT: (id: string) => `/networks/${id}/connect`,
  NETWORK_DISCONNECT: (id: string) => `/networks/${id}/disconnect`,
} as const

// 错误码常量
export const ERROR_CODES = {
  SUCCESS: 0,
  BAD_REQUEST: 40000,
  IMAGE_REQUIRED: 40001,
  SCAN_FAILED: 50001,
  UPDATE_FAILED: 50002,
  DOCKER_ERROR: 50003,
  REGISTRY_ERROR: 50004,
} as const

// HTTP 状态码
export const HTTP_STATUS = {
  OK: 200,
  BAD_REQUEST: 400,
  UNAUTHORIZED: 401,
  FORBIDDEN: 403,
  NOT_FOUND: 404,
  INTERNAL_SERVER_ERROR: 500,
} as const

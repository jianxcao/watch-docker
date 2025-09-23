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
  CONTAINER_UPDATE: (id: string) => `/containers/${id}/update`,
  CONTAINER_START: (id: string) => `/containers/${id}/start`,
  CONTAINER_STOP: (id: string) => `/containers/${id}/stop`,
  CONTAINER_DELETE: (id: string) => `/containers/${id}`,

  // 批量操作
  BATCH_UPDATE: '/updates/run',

  // 镜像相关
  IMAGES: '/images',

  // 配置相关
  CONFIG: '/config',
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

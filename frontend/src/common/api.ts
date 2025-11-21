import { API_ENDPOINTS } from '@/constants/api'
import axios from './axiosConfig'
import type {
  BatchUpdateResult,
  ComposeProject,
  Config,
  ContainerStatus,
  ContainerCreateRequest,
  ContainerCreateResponse,
  ImageInfo,
  SystemInfo,
  VolumeListResponse,
  VolumeDetailResponse,
  VolumeCreateRequest,
  VolumePruneResponse,
  NetworkListResponse,
  NetworkDetailResponse,
  NetworkCreateRequest,
  NetworkPruneResponse,
  NetworkConnectRequest,
  NetworkDisconnectRequest,
} from './types'

// 健康检查相关
export const healthApi = {
  // 健康检查
  health: () => axios.get<any>(API_ENDPOINTS.HEALTH),

  // 就绪检查
  ready: () => axios.get<any>(API_ENDPOINTS.READY),
}

// 身份验证相关API
export const authApi = {
  // 登录
  login: (username: string, password: string) =>
    axios.post<{
      token?: string
      username?: string
      needTwoFA?: boolean
      isSetup?: boolean
      method?: string
      tempToken?: string
    }>(API_ENDPOINTS.LOGIN, {
      username,
      password,
    }),

  // 登出
  logout: () => axios.post<{ message: string }>(API_ENDPOINTS.LOGOUT),

  // 检查身份验证状态
  checkAuthStatus: () => axios.get<{ authEnabled: boolean }>(API_ENDPOINTS.AUTH_STATUS),

  // 获取系统信息
  getInfo: () => axios.get<{ info: SystemInfo }>(API_ENDPOINTS.INFO),
}

// 容器相关API
export const containerApi = {
  // 获取容器列表
  getContainers: (isUserCache = true, isHaveUpdate = true) =>
    axios.get<{ containers: ContainerStatus[] }>(API_ENDPOINTS.CONTAINERS, {
      params: { isUserCache, isHaveUpdate },
    }),

  // 获取容器详情
  getContainerDetail: (id: string) =>
    axios.get<{ container: any }>(API_ENDPOINTS.CONTAINER_DETAIL(id)),

  // 创建容器
  createContainer: (data: ContainerCreateRequest) =>
    axios.post<ContainerCreateResponse>(API_ENDPOINTS.CONTAINER_CREATE, data),

  // 更新单个容器
  updateContainer: (id: string, image?: string) =>
    axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_UPDATE(id), { image }),

  // 批量更新容器
  batchUpdate: () => axios.post<BatchUpdateResult>(API_ENDPOINTS.BATCH_UPDATE),

  // 启动容器
  startContainer: (id: string) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_START(id)),

  // 停止容器
  stopContainer: (id: string) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_STOP(id)),

  // 重启容器
  restartContainer: (id: string) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_RESTART(id)),

  // 删除容器
  deleteContainer: (id: string, force: boolean = false, removeVolumes: boolean = false, removeNetworks: boolean = false) =>
    axios.delete<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_DELETE(id), {
      params: { force, removeVolumes, removeNetworks },
    }),

  // 系统清理
  pruneSystem: () => axios.post<{ ok: boolean; message: string }>(API_ENDPOINTS.PRUNE_SYSTEM),

  // 导入容器
  importContainer: (formData: FormData) =>
    axios.post<{ success: boolean; message: string }>(API_ENDPOINTS.CONTAINER_IMPORT, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    }),
}

// 镜像相关API
export const imageApi = {
  // 获取镜像列表
  getImages: () => axios.get<{ images: ImageInfo[] }>(API_ENDPOINTS.IMAGES),

  // 删除镜像
  deleteImage: (ref: string, force: boolean = false) =>
    axios.delete<{ ok: boolean }>(API_ENDPOINTS.IMAGES, {
      data: { ref, force },
    }),

  // 导入镜像
  importImage: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return axios.post<{ success: boolean; message: string }>(API_ENDPOINTS.IMAGE_IMPORT, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    })
  },
}

// 配置相关API
export const configApi = {
  // 获取配置
  getConfig: () => axios.get<{ config: Config }>(API_ENDPOINTS.CONFIG),

  // 保存配置
  saveConfig: (config: Config) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONFIG, config),
}

// Compose 项目管理API
export const composeApi = {
  // 获取 Compose 项目列表
  getProjects: () => axios.get<{ projects: ComposeProject[] }>('/compose'),

  // 启动项目
  startProject: (project: ComposeProject) => axios.post<{ ok: boolean }>(`/compose/start`, project),

  // 停止项目
  stopProject: (project: ComposeProject) => axios.post<{ ok: boolean }>(`/compose/stop`, project),

  createProject: (project: ComposeProject) =>
    axios.post<{ ok: boolean }>(`/compose/create`, project),
  // 重新创建项目
  restartProject: (project: ComposeProject) =>
    axios.post<{ ok: boolean }>(`/compose/restart`, project),

  // 删除项目
  deleteProject: (project: ComposeProject) =>
    axios.delete<{ ok: boolean }>(`/compose/delete`, {
      data: project,
    }),

  // 获取项目日志
  getProjectLogs: (name: string, lines = 100) =>
    axios.get<{ logs: string }>(`/compose/${name}/logs`, { params: { lines } }),

  // 创建新项目（保存 YAML 文件）
  saveNewProject: (name: string, yamlContent: string) =>
    axios.post<{ ok: boolean; composeFile: string }>(`/compose/new`, { name, yamlContent }),

  // 获取项目的 YAML 内容
  getProjectYaml: (projectName: string, composeFile: string) =>
    axios.get<{ yamlContent: string }>(`/compose/${projectName}/yaml`, {
      params: { composeFile },
    }),
}

// 二次验证 API
export const twoFAApi = {
  getStatus: () => axios.get('/2fa/status'),
  setupOTPInit: () => axios.post('/2fa/setup/otp/init'),
  setupOTPVerify: (code: string, secret: string) =>
    axios.post('/2fa/setup/otp/verify', { code, secret }),
  verifyOTP: (code: string) => axios.post('/2fa/verify/otp', { code }),
  webauthnRegisterBegin: () => axios.post('/2fa/setup/webauthn/begin'),
  webauthnRegisterFinish: (sessionData: string, response: any) =>
    axios.post('/2fa/setup/webauthn/finish', { sessionData, response }),
  webauthnLoginBegin: () => axios.post('/2fa/verify/webauthn/begin'),
  webauthnLoginFinish: (sessionData: string, response: any) =>
    axios.post('/2fa/verify/webauthn/finish', { sessionData, response }),
  disable: () => axios.post('/2fa/disable'),
}

// Volume 相关API
export const volumeApi = {
  // 获取Volume列表
  getVolumes: () => axios.get<VolumeListResponse>('/volumes'),

  // 获取Volume详情
  getVolume: (name: string) => axios.get<VolumeDetailResponse>(`/volumes/${name}`),

  // 创建Volume
  createVolume: (data: VolumeCreateRequest) => axios.post<{ volume: any }>('/volumes', data),

  // 删除Volume
  deleteVolume: (name: string, force: boolean = false) =>
    axios.delete<{ ok: boolean }>(`/volumes/${name}`, { params: { force } }),

  // 清理未使用的Volume
  pruneVolumes: () => axios.post<VolumePruneResponse>('/volumes/prune'),
}

// 网络相关API
export const networkApi = {
  // 获取网络列表
  getNetworks: () => axios.get<NetworkListResponse>(API_ENDPOINTS.NETWORKS),

  // 获取网络详情
  getNetwork: (id: string) => axios.get<NetworkDetailResponse>(API_ENDPOINTS.NETWORK_DETAIL(id)),

  // 创建网络
  createNetwork: (data: NetworkCreateRequest) =>
    axios.post<{ network: any }>(API_ENDPOINTS.NETWORKS, data),

  // 删除网络
  deleteNetwork: (id: string) => axios.delete<{ ok: boolean }>(API_ENDPOINTS.NETWORK_DELETE(id)),

  // 清理未使用的网络
  pruneNetworks: () => axios.post<NetworkPruneResponse>(API_ENDPOINTS.NETWORK_PRUNE),

  // 连接容器到网络
  connectContainer: (id: string, data: NetworkConnectRequest) =>
    axios.post<{ ok: boolean }>(API_ENDPOINTS.NETWORK_CONNECT(id), data),

  // 从网络断开容器
  disconnectContainer: (id: string, data: NetworkDisconnectRequest) =>
    axios.post<{ ok: boolean }>(API_ENDPOINTS.NETWORK_DISCONNECT(id), data),
}

// 导出所有API
export const api = {
  health: healthApi,
  auth: authApi,
  container: containerApi,
  image: imageApi,
  config: configApi,
  compose: composeApi,
  twoFA: twoFAApi,
  volume: volumeApi,
  network: networkApi,
}

export default api

// 单独导出常用的API函数，方便直接导入
export const { importContainer } = containerApi
export const { importImage } = imageApi

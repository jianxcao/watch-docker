import { API_ENDPOINTS } from '@/constants/api'
import axios from './axiosConfig'
import type { BatchUpdateResult, Config, ContainerStatus, ImageInfo } from './types'

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
    axios.post<{ token: string; username: string }>(API_ENDPOINTS.LOGIN, {
      username,
      password,
    }),

  // 登出
  logout: () => axios.post<{ message: string }>(API_ENDPOINTS.LOGOUT),

  // 检查身份验证状态
  checkAuthStatus: () => axios.get<{ authEnabled: boolean }>(API_ENDPOINTS.AUTH_STATUS),

  // 获取系统信息
  getInfo: () => axios.get<any>(API_ENDPOINTS.INFO),
}

// 容器相关API
export const containerApi = {
  // 获取容器列表
  getContainers: (isUserCache = true, isHaveUpdate = true) =>
    axios.get<{ containers: ContainerStatus[] }>(API_ENDPOINTS.CONTAINERS, {
      params: { isUserCache, isHaveUpdate },
    }),

  // 更新单个容器
  updateContainer: (id: string, image?: string) =>
    axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_UPDATE(id), { image }),

  // 批量更新容器
  batchUpdate: () => axios.post<BatchUpdateResult>(API_ENDPOINTS.BATCH_UPDATE),

  // 启动容器
  startContainer: (id: string) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_START(id)),

  // 停止容器
  stopContainer: (id: string) => axios.post<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_STOP(id)),

  // 删除容器
  deleteContainer: (id: string, force: boolean = false) =>
    axios.delete<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_DELETE(id), {
      params: { force },
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

// 导出所有API
export const api = {
  health: healthApi,
  auth: authApi,
  container: containerApi,
  image: imageApi,
  config: configApi,
}

export default api

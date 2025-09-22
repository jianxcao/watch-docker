import axios from './axiosConfig'
import type { ContainerStatus, ImageInfo, BatchUpdateResult, Config } from './types'
import { API_ENDPOINTS } from '@/constants/api'

// 健康检查相关
export const healthApi = {
  // 健康检查
  health: () => axios.get<any>(API_ENDPOINTS.HEALTH),

  // 就绪检查
  ready: () => axios.get<any>(API_ENDPOINTS.READY),
}

// 容器相关API
export const containerApi = {
  // 获取容器列表
  getContainers: () => axios.get<{ containers: ContainerStatus[] }>(API_ENDPOINTS.CONTAINERS),

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
  deleteContainer: (id: string) =>
    axios.delete<{ ok: boolean }>(API_ENDPOINTS.CONTAINER_DELETE(id)),
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
  container: containerApi,
  image: imageApi,
  config: configApi,
}

export default api

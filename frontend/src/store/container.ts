import { containerApi } from '@/common/api'
import type { BatchUpdateResult, ContainerStatus, ContainerOperationState } from '@/common/types'
import useStatsWebSocket from '@/hooks/useStatsWebSocket'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import statsEmitter from '@/evt/containerStats'

export const useContainerStore = defineStore('container', () => {
  // 状态
  const containers = ref<ContainerStatus[]>([])
  const loading = ref(false)
  const updating = ref(new Set<string>())
  const batchUpdating = ref(false)

  // 操作状态追踪：containerId -> 操作状态
  const operationStates = ref(new Map<string, ContainerOperationState>())

  // WebSocket 相关状态
  const statsWebSocket = useStatsWebSocket()
  const wsConnected = computed(() => statsWebSocket.isConnected.value)
  const wsConnectionState = computed(() => statsWebSocket.connectionState.value)

  // 计算属性
  const runningContainers = computed(() => containers.value.filter((c) => c.running))

  const stoppedContainers = computed(() => containers.value.filter((c) => !c.running))

  const updateableContainers = computed(() =>
    containers.value.filter((c) => c.status === 'UpdateAvailable' && !c.skipped),
  )

  const upToDateContainers = computed(() => containers.value.filter((c) => c.status === 'UpToDate'))

  const errorContainers = computed(() => containers.value.filter((c) => c.status === 'Error'))

  const skippedContainers = computed(() => containers.value.filter((c) => c.skipped))

  // 统计信息
  const stats = computed(() => ({
    total: containers.value.length,
    running: runningContainers.value.length,
    stopped: stoppedContainers.value.length,
    updateable: updateableContainers.value.length,
    upToDate: upToDateContainers.value.length,
    error: errorContainers.value.length,
    skipped: skippedContainers.value.length,
  }))

  // 操作状态方法
  const setOperationState = (id: string, state: ContainerOperationState) => {
    const map = new Map(operationStates.value)
    map.set(id, state)
    operationStates.value = map
  }

  const clearOperationState = (id: string) => {
    const map = new Map(operationStates.value)
    map.delete(id)
    operationStates.value = map
  }

  const getOperationState = (id: string): ContainerOperationState => {
    return operationStates.value.get(id) ?? { type: 'idle' }
  }

  // 方法：获取容器列表
  const fetchContainers = async (isUserCache = true, isHaveUpdate = true) => {
    loading.value = true
    try {
      const data = await containerApi.getContainers(isUserCache, isHaveUpdate)
      if (data.code === 0) {
        // 按照 ID 进行合并，新数据覆盖旧数据
        const newContainers = data.data.containers
        const existingContainersMap = new Map(containers.value.map((c) => [c.id, c]))

        containers.value = newContainers.map((newContainer) => {
          const existingContainer = existingContainersMap.get(newContainer.id)
          // 如果存在旧容器，合并数据，新数据覆盖旧数据
          if (existingContainer) {
            const res = { ...existingContainer, ...newContainer }
            return res
          }
          // 如果是新容器，直接使用新数据
          return newContainer
        })
      } else {
        console.error('获取容器列表失败:', data.msg)
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('获取容器列表失败:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  // 方法：更新单个容器
  const updateContainer = async (id: string, image?: string): Promise<boolean> => {
    updating.value.add(id)
    setOperationState(id, { type: 'updating', step: 'pulling' })
    try {
      const data = await containerApi.updateContainer(id, image)
      if (data.code === 0) {
        await fetchContainers()
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('更新容器失败:', error)
      throw error
    } finally {
      updating.value.delete(id)
      clearOperationState(id)
    }
  }

  // 方法：批量更新容器
  const batchUpdate = async (): Promise<BatchUpdateResult> => {
    batchUpdating.value = true
    try {
      const data = await containerApi.batchUpdate()
      if (data.code === 0) {
        await fetchContainers()
        return data.data
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('批量更新失败:', error)
      throw error
    } finally {
      batchUpdating.value = false
    }
  }

  // 方法：启动容器
  const startContainer = async (id: string): Promise<boolean> => {
    setOperationState(id, { type: 'starting' })
    try {
      const data = await containerApi.startContainer(id)
      if (data.code === 0) {
        await fetchContainers()
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('启动容器失败:', error)
      throw error
    } finally {
      clearOperationState(id)
    }
  }

  // 方法：停止容器
  const stopContainer = async (id: string): Promise<boolean> => {
    setOperationState(id, { type: 'stopping' })
    try {
      const data = await containerApi.stopContainer(id)
      if (data.code === 0) {
        await fetchContainers()
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('停止容器失败:', error)
      throw error
    } finally {
      clearOperationState(id)
    }
  }

  // 方法：重启容器
  const restartContainer = async (id: string): Promise<boolean> => {
    setOperationState(id, { type: 'restarting' })
    try {
      const data = await containerApi.restartContainer(id)
      if (data.code === 0) {
        await fetchContainers()
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('重启容器失败:', error)
      throw error
    } finally {
      clearOperationState(id)
    }
  }

  // 方法：删除容器
  const deleteContainer = async (id: string, force: boolean = false, removeVolumes: boolean = false, removeNetworks: boolean = false): Promise<boolean> => {
    setOperationState(id, { type: 'deleting' })
    try {
      const data = await containerApi.deleteContainer(id, force, removeVolumes, removeNetworks)
      if (data.code === 0) {
        await fetchContainers()
        return true
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('删除容器失败:', error)
      throw error
    } finally {
      clearOperationState(id)
    }
  }

  // 方法：根据ID查找容器
  const findContainerById = (id: string) => {
    return containers.value.find((c) => c.id === id)
  }

  // 方法：根据名称查找容器
  const findContainerByName = (name: string) => {
    return containers.value.find((c) => c.name === name)
  }

  // 方法：根据项目名称获取容器列表
  const getProjectContainers = (projectName: string) => {
    return computed(() => {
      return containers.value.filter((container) => {
        return container.labels?.['com.docker.compose.project'] === projectName
      })
    })
  }

  // 方法：检查容器是否正在更新
  const isContainerUpdating = (id: string) => {
    return updating.value.has(id)
  }

  // WebSocket 容器数据回调处理
  const handleContainersUpdate = (newContainers: ContainerStatus[]) => {
    const mapContainers = new Map<string, ContainerStatus>()
    newContainers.forEach((container) => {
      mapContainers.set(container.id, container)
    })
    const oldContainers = [...containers.value]
    containers.value = oldContainers.map((container) => {
      const newContainer = mapContainers.get(container.id)
      if (newContainer) {
        return newContainer
      }
      return container
    })
  }

  statsEmitter.on('containers', handleContainersUpdate)

  // 方法：导出容器
  const exportContainer = async (id: string): Promise<boolean> => {
    try {
      const { getToken } = await import('@/common/axiosConfig')
      const token = getToken()

      const baseUrl = '/api/v1'
      const downloadUrl = `${baseUrl}/containers/${id}/export?token=${encodeURIComponent(
        token,
      )}&_t=${Date.now()}`

      const downloadWindow = window.open(downloadUrl, '_blank')

      if (!downloadWindow) {
        throw new Error('浏览器阻止了弹窗，请允许弹窗后重试')
      }

      return true
    } catch (error: any) {
      console.error('导出容器失败:', error)
      throw error
    }
  }

  // 方法：获取容器详情
  const getContainerDetail = async (id: string): Promise<any> => {
    try {
      const data = await containerApi.getContainerDetail(id)
      if (data.code === 0) {
        return data.data.container
      } else {
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('获取容器详情失败:', error)
      throw error
    }
  }

  return {
    // 状态
    containers,
    loading,
    updating,
    batchUpdating,
    wsConnected,
    wsConnectionState,
    operationStates,

    // 计算属性
    runningContainers,
    stoppedContainers,
    updateableContainers,
    upToDateContainers,
    errorContainers,
    skippedContainers,
    stats,

    // 方法
    fetchContainers,
    updateContainer,
    batchUpdate,
    startContainer,
    stopContainer,
    restartContainer,
    deleteContainer,
    findContainerById,
    findContainerByName,
    getProjectContainers,
    isContainerUpdating,
    exportContainer,
    getContainerDetail,
    statsWebSocket,

    // 操作状态方法
    setOperationState,
    clearOperationState,
    getOperationState,
  }
})

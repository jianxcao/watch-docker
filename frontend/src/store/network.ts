import { networkApi } from '@/common/api'
import type { NetworkInfo, NetworkStats, NetworkCreateRequest } from '@/common/types'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useNetworkStore = defineStore('network', () => {
  // 状态
  const networks = ref<NetworkInfo[]>([])
  const loading = ref(false)

  // 内置网络列表
  const builtInNetworkNames = ['bridge', 'host', 'none']

  // 计算属性 - 使用中的网络
  const usedNetworks = computed(() => networks.value.filter((n) => n.containerCount > 0))

  // 计算属性 - 未使用的网络
  const unusedNetworks = computed(() => networks.value.filter((n) => n.containerCount === 0))

  // 计算属性 - 内置网络
  const builtInNetworks = computed(() =>
    networks.value.filter((n) => builtInNetworkNames.includes(n.name)),
  )

  // 计算属性 - 自定义网络
  const customNetworks = computed(() =>
    networks.value.filter((n) => !builtInNetworkNames.includes(n.name)),
  )

  // 计算属性 - 统计信息
  const stats = computed<NetworkStats>(() => {
    return {
      total: networks.value.length,
      used: usedNetworks.value.length,
      unused: unusedNetworks.value.length,
      builtIn: builtInNetworks.value.length,
      custom: customNetworks.value.length,
    }
  })

  // 方法：获取网络列表
  const fetchNetworks = async () => {
    loading.value = true
    try {
      const data = await networkApi.getNetworks()
      if (data.code === 0) {
        networks.value = data.data.networks || []
      } else {
        console.error('获取网络列表失败:', data.msg)
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('获取网络列表失败:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  // 方法：创建网络
  const createNetwork = async (data: NetworkCreateRequest) => {
    try {
      const response = await networkApi.createNetwork(data)
      if (response.code === 0) {
        await fetchNetworks() // 重新获取列表
        return true
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('创建网络失败:', error)
      throw error
    }
  }

  // 方法：删除网络
  const deleteNetwork = async (id: string) => {
    try {
      const response = await networkApi.deleteNetwork(id)
      if (response.code === 0) {
        await fetchNetworks() // 重新获取列表
        return true
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('删除网络失败:', error)
      throw error
    }
  }

  // 方法：清理未使用的网络
  const pruneNetworks = async () => {
    try {
      const response = await networkApi.pruneNetworks()
      if (response.code === 0) {
        await fetchNetworks() // 重新获取列表
        return response.data
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('清理网络失败:', error)
      throw error
    }
  }

  // 方法：根据ID查找网络
  const findNetworkById = (id: string) => {
    return networks.value.find((n) => n.id === id || n.id.startsWith(id))
  }

  // 方法：根据名称查找网络
  const findNetworkByName = (name: string) => {
    return networks.value.find((n) => n.name === name)
  }

  return {
    // 状态
    networks,
    loading,

    // 计算属性
    usedNetworks,
    unusedNetworks,
    builtInNetworks,
    customNetworks,
    stats,

    // 方法
    fetchNetworks,
    createNetwork,
    deleteNetwork,
    pruneNetworks,
    findNetworkById,
    findNetworkByName,
  }
})

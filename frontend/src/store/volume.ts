import { volumeApi } from '@/common/api'
import type { VolumeInfo, VolumeStats } from '@/common/types'
import { formatBytes } from '@/common/utils'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useVolumeStore = defineStore('volume', () => {
  // 状态
  const volumes = ref<VolumeInfo[]>([])
  const loading = ref(false)

  // 计算属性 - 使用中的Volume
  const usedVolumes = computed(() =>
    volumes.value.filter((v) => v.usageData && v.usageData.refCount > 0),
  )

  // 计算属性 - 未使用的Volume
  const unusedVolumes = computed(() =>
    volumes.value.filter((v) => !v.usageData || v.usageData.refCount === 0),
  )

  // 计算属性 - 统计信息
  const stats = computed<VolumeStats>(() => {
    const totalSize = volumes.value.reduce((sum, v) => sum + (v.usageData?.size || 0), 0)
    return {
      total: volumes.value.length,
      used: usedVolumes.value.length,
      unused: unusedVolumes.value.length,
      totalSize,
      formattedTotalSize: formatBytes(totalSize),
    }
  })

  // 方法：获取Volume列表
  const fetchVolumes = async () => {
    loading.value = true
    try {
      const data = await volumeApi.getVolumes()
      if (data.code === 0) {
        volumes.value = data.data.volumes || []
      } else {
        console.error('获取Volume列表失败:', data.msg)
        throw new Error(data.msg)
      }
    } catch (error) {
      console.error('获取Volume列表失败:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  // 方法：创建Volume
  const createVolume = async (data: {
    name: string
    driver?: string
    driverOpts?: Record<string, string>
    labels?: Record<string, string>
  }) => {
    try {
      const response = await volumeApi.createVolume(data)
      if (response.code === 0) {
        await fetchVolumes() // 重新获取列表
        return true
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('创建Volume失败:', error)
      throw error
    }
  }

  // 方法：删除Volume
  const deleteVolume = async (name: string, force: boolean = false) => {
    try {
      const response = await volumeApi.deleteVolume(name, force)
      if (response.code === 0) {
        await fetchVolumes() // 重新获取列表
        return true
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('删除Volume失败:', error)
      throw error
    }
  }

  // 方法：清理未使用的Volume
  const pruneVolumes = async () => {
    try {
      const response = await volumeApi.pruneVolumes()
      if (response.code === 0) {
        await fetchVolumes() // 重新获取列表
        return response.data
      } else {
        throw new Error(response.msg)
      }
    } catch (error) {
      console.error('清理Volume失败:', error)
      throw error
    }
  }

  // 方法：根据名称查找Volume
  const findVolumeByName = (name: string) => {
    return volumes.value.find((v) => v.name === name)
  }

  return {
    // 状态
    volumes,
    loading,

    // 计算属性
    usedVolumes,
    unusedVolumes,
    stats,

    // 方法
    fetchVolumes,
    createVolume,
    deleteVolume,
    pruneVolumes,
    findVolumeByName,
  }
})


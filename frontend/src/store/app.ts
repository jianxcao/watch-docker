import { defineStore } from 'pinia'
import { ref } from 'vue'
import { healthApi } from '@/common/api'

export const useAppStore = defineStore('app', () => {
  // 移动端抽屉菜单状态
  const drawerVisible = ref(false)

  // 全局加载状态
  const globalLoading = ref(false)

  // 系统健康状态
  const systemHealth = ref<'healthy' | 'unhealthy' | 'unknown'>('unknown')

  // 最后刷新时间
  const lastRefreshTime = ref<Date | null>(null)

  // 方法：切换抽屉菜单
  const toggleDrawer = () => {
    drawerVisible.value = !drawerVisible.value
  }

  // 方法：关闭抽屉菜单
  const closeDrawer = () => {
    drawerVisible.value = false
  }

  // 方法：设置系统健康状态
  const setSystemHealth = (status: 'healthy' | 'unhealthy' | 'unknown') => {
    systemHealth.value = status
  }

  // 方法：设置最后刷新时间
  const updateRefreshTime = () => {
    lastRefreshTime.value = new Date()
  }

  // 方法：设置全局加载状态
  const setGlobalLoading = (loading: boolean) => {
    globalLoading.value = loading
  }

  const checkHealth = async () => {
    try {
      await healthApi.health()
      setSystemHealth('healthy')
    } catch (error) {
      setSystemHealth('unhealthy')
      console.error('健康检查失败:', error)
    }
  }

  return {
    // 状态
    drawerVisible,
    globalLoading,
    systemHealth,
    lastRefreshTime,

    // 方法
    toggleDrawer,
    closeDrawer,
    setSystemHealth,
    updateRefreshTime,
    setGlobalLoading,
    checkHealth,
  }
})

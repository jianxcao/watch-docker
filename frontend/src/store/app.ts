import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useAppStore = defineStore('app', () => {
  // 移动端抽屉菜单状态
  const drawerVisible = ref(false)

  // 全局加载状态
  const globalLoading = ref(false)

  // 应用标题
  const appTitle = ref('Watch Docker')

  // 当前页面标题
  const pageTitle = ref('')

  // 系统健康状态
  const systemHealth = ref<'healthy' | 'unhealthy' | 'unknown'>('unknown')

  // 最后刷新时间
  const lastRefreshTime = ref<Date | null>(null)

  // 计算属性：完整标题
  const fullTitle = computed(() => {
    if (pageTitle.value) {
      return `${pageTitle.value} - ${appTitle.value}`
    }
    return appTitle.value
  })

  // 方法：切换抽屉菜单
  const toggleDrawer = () => {
    drawerVisible.value = !drawerVisible.value
  }

  // 方法：关闭抽屉菜单
  const closeDrawer = () => {
    drawerVisible.value = false
  }

  // 方法：设置页面标题
  const setPageTitle = (title: string) => {
    pageTitle.value = title
    document.title = fullTitle.value
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

  return {
    // 状态
    drawerVisible,
    globalLoading,
    appTitle,
    pageTitle,
    systemHealth,
    lastRefreshTime,

    // 计算属性
    fullTitle,

    // 方法
    toggleDrawer,
    closeDrawer,
    setPageTitle,
    setSystemHealth,
    updateRefreshTime,
    setGlobalLoading,
  }
})

<template>
  <n-config-provider :theme="null">
    <n-layout has-sider class="layout-container">
      <!-- 侧边菜单 (大屏幕) -->
      <n-layout-sider v-if="isLargeScreen" :width="240" :collapsed-width="0" collapse-mode="width" bordered
        show-trigger="bar" class="layout-sider">
        <div class="sider-content">
          <!-- 标题 -->
          <div class="sider-header">
            <n-space align="center">
              <img src="/logo.svg" alt="Logo" class="logo" />
              <n-h3 style="margin: 0;">Watch Docker</n-h3>
            </n-space>
          </div>

          <!-- 菜单 -->
          <n-menu :value="activeKey" :options="menuOptions" @update:value="handleMenuSelect" class="sider-menu" />

          <!-- 底部状态 -->
          <div class="sider-footer">
            <n-space vertical>
              <n-space align="center" justify="space-between">
                <n-text depth="3" style="font-size: 12px;">
                  v{{ version }}
                </n-text>
                <n-tag :type="healthStatus === 'healthy' ? 'success' : 'error'" size="small">
                  {{ healthText }}
                </n-tag>
              </n-space>
              <n-button text type="primary" size="small" @click="handleRefresh" :loading="appStore.globalLoading">
                <template #icon>
                  <n-icon>
                    <RefreshOutline />
                  </n-icon>
                </template>
                刷新数据
              </n-button>
            </n-space>
          </div>
        </div>
      </n-layout-sider>

      <!-- 主内容区域 -->
      <n-layout class="main-layout">
        <!-- 顶部栏 (小屏幕) -->
        <n-layout-header v-if="isSmallScreen" bordered class="mobile-header" style="height: 64px; padding: 0 16px;">
          <n-space align="center" justify="space-between" style="height: 100%;">
            <n-space align="center">
              <n-button text @click="appStore.toggleDrawer">
                <template #icon>
                  <n-icon size="20">
                    <MenuOutline />
                  </n-icon>
                </template>
              </n-button>
              <n-h3 style="margin: 0;">{{ currentPageTitle }}</n-h3>
            </n-space>

            <n-tag :type="healthStatus === 'healthy' ? 'success' : 'error'" size="small">
              {{ healthText }}
            </n-tag>
          </n-space>
        </n-layout-header>

        <!-- 内容区域 -->
        <n-layout-content class="layout-content">
          <div class="content-wrapper">
            <router-view />
          </div>
        </n-layout-content>
      </n-layout>
    </n-layout>

    <!-- 移动端抽屉菜单 (仅小屏幕显示) -->
    <MobileDrawer v-if="isSmallScreen" />

    <!-- 全局消息容器 -->
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <!-- 空的，用于提供全局组件 -->
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { useResponsive } from '@/hooks/useResponsive'
import { healthApi } from '@/common/api'
import MobileDrawer from './MobileDrawer.vue'
import type { MenuOption } from 'naive-ui'
import {
  HomeOutline,
  LayersOutline,
  ArchiveOutline,
  SettingsOutline,
  MenuOutline,
  RefreshOutline,
} from '@vicons/ionicons5'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()
const { isLargeScreen, isSmallScreen } = useResponsive()

// 版本信息
const version = '0.0.1'

// 当前活跃的菜单项
const activeKey = computed(() => {
  const path = route.path
  if (path === '/') return 'home'
  if (path === '/containers') return 'containers'
  if (path === '/images') return 'images'
  if (path === '/settings') return 'settings'
  return 'home'
})

// 当前页面标题
const currentPageTitle = computed(() => {
  switch (activeKey.value) {
    case 'home':
      return '首页'
    case 'containers':
      return '容器管理'
    case 'images':
      return '镜像管理'
    case 'settings':
      return '系统设置'
    default:
      return 'Watch Docker'
  }
})

// 系统健康状态
const healthStatus = computed(() => appStore.systemHealth)

const healthText = computed(() => {
  switch (healthStatus.value) {
    case 'healthy':
      return '正常'
    case 'unhealthy':
      return '异常'
    default:
      return '未知'
  }
})

// 菜单配置
const menuOptions = computed<MenuOption[]>(() => [
  {
    label: '首页',
    key: 'home',
    icon: () => h(HomeOutline),
  },
  {
    label: '容器管理',
    key: 'containers',
    icon: () => h(LayersOutline),
  },
  {
    label: '镜像管理',
    key: 'images',
    icon: () => h(ArchiveOutline),
  },
  {
    label: '系统设置',
    key: 'settings',
    icon: () => h(SettingsOutline),
  },
])

// 处理菜单选择
const handleMenuSelect = (key: string) => {
  switch (key) {
    case 'home':
      router.push('/')
      break
    case 'containers':
      router.push('/containers')
      break
    case 'images':
      router.push('/images')
      break
    case 'settings':
      router.push('/settings')
      break
  }
}

// 刷新数据
const handleRefresh = async () => {
  appStore.setGlobalLoading(true)
  try {
    // 根据当前页面刷新相应数据
    if (activeKey.value === 'containers') {
      await containerStore.fetchContainers()
    } else if (activeKey.value === 'images') {
      await imageStore.fetchImages()
    } else {
      // 首页刷新所有数据
      await Promise.all([
        containerStore.fetchContainers(),
        imageStore.fetchImages(),
      ])
    }
    appStore.updateRefreshTime()
  } catch (error) {
    console.error('刷新数据失败:', error)
  } finally {
    appStore.setGlobalLoading(false)
  }
}

// 检查系统健康状态
const checkHealth = async () => {
  try {
    await healthApi.health()
    appStore.setSystemHealth('healthy')
  } catch (error) {
    appStore.setSystemHealth('unhealthy')
    console.error('健康检查失败:', error)
  }
}

// 页面加载时检查系统健康状态并初始化数据
onMounted(async () => {
  await checkHealth()

  // 设置页面标题
  appStore.setPageTitle(currentPageTitle.value)

  // 根据当前路由初始化相应数据
  if (activeKey.value === 'containers') {
    await containerStore.fetchContainers()
  } else if (activeKey.value === 'images') {
    await imageStore.fetchImages()
  } else {
    // 首页加载所有数据
    await Promise.all([
      containerStore.fetchContainers(),
      imageStore.fetchImages(),
    ])
  }
})
</script>

<style scoped lang="less">
.layout-container {
  height: 100vh;
}

.layout-sider {
  background: #fff;
}

.sider-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px 0;
}

.sider-header {
  padding: 0 16px 24px 16px;
  border-bottom: 1px solid #f0f0f0;

  .logo {
    width: 32px;
    height: 32px;
  }
}

.sider-menu {
  flex: 1;
  margin-top: 16px;
  padding: 0 8px;
}

.sider-footer {
  margin-top: auto;
  padding: 16px;
  border-top: 1px solid #f0f0f0;
}

.main-layout {
  background: #f5f5f5;
}

.mobile-header {
  background: #fff;
  border-bottom: 1px solid #f0f0f0;
}

.layout-content {
  padding: 16px;
}

.content-wrapper {
  max-width: 1200px;
  margin: 0 auto;
}

// 响应式调整
@media (max-width: 768px) {
  .layout-content {
    padding: 8px;
  }
}
</style>

<template>
  <n-drawer v-model:show="drawerVisible" :width="280" placement="left" :trap-focus="false" :block-scroll="false">
    <n-drawer-content title="Watch Docker" closable>
      <div class="drawer-menu">
        <n-menu :value="activeKey" :options="menuOptions" @update:value="handleMenuSelect" />
      </div>

      <template #footer>
        <div class="drawer-footer">
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
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import type { MenuOption } from 'naive-ui'
import {
  HomeOutline,
  LayersOutline,
  ArchiveOutline,
  SettingsOutline,
  RefreshOutline,
} from '@vicons/ionicons5'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()

// 版本信息
const version = '0.0.1'

// 抽屉可见性
const drawerVisible = computed({
  get: () => appStore.drawerVisible,
  set: (value) => {
    if (!value) {
      appStore.closeDrawer()
    }
  }
})

// 当前活跃的菜单项
const activeKey = computed(() => {
  const path = route.path
  if (path === '/') return 'home'
  if (path === '/containers') return 'containers'
  if (path === '/images') return 'images'
  if (path === '/settings') return 'settings'
  return 'home'
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

  // 移动端选择菜单后关闭抽屉
  appStore.closeDrawer()
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
</script>

<style scoped lang="less">
.drawer-menu {
  margin-top: 16px;
}

.drawer-footer {
  padding: 16px 0 8px 0;
}
</style>

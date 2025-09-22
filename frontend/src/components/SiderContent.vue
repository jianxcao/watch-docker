<template>
  <n-el class="sider-content">
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
      <div class="flex items-center justify-center gap-2">
        <n-tag :type="healthStatus === 'healthy' ? 'success' : 'error'" size="small">
          {{ healthText }}
        </n-tag>
        <n-text depth="3" style="font-size: 12px;">
          v{{ version }}
        </n-text>
      </div>
      <n-icon @click="handleRefresh" :loading="appStore.globalLoading" class="cursor-pointer">
        <template v-if="appStore.globalLoading">
          <RefreshOutline class="rotating" />
        </template>
        <template v-else>
          <RefreshOutline />
        </template>
      </n-icon>
    </div>
  </n-el>
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

interface Props {
  onMenuSelect?: () => void // 菜单选择后的回调，用于移动端关闭抽屉
}

const props = withDefaults(defineProps<Props>(), {
  onMenuSelect: undefined,
})

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()

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

  // 触发回调（移动端用于关闭抽屉）
  if (props.onMenuSelect) {
    props.onMenuSelect()
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
</script>

<style scoped lang="less">
.sider-content {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.sider-header {
  height: 56px;
  box-sizing: border-box;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  justify-content: center;


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
  padding: 8px;
  height: 56px;
  box-sizing: border-box;
  border-top: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.rotating {
  animation: rotate 1s linear infinite;
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}
</style>

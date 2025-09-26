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

    <!-- 用户信息（如果启用了身份验证） -->
    <div v-if="authStore.authEnabled && authStore.isLoggedIn" class="user-info">
      <n-space align="center" justify="space-between">
        <n-space align="center" size="small">
          <n-avatar size="small" :style="{ backgroundColor: '#18a058' }">
            {{ authStore.username.charAt(0).toUpperCase() }}
          </n-avatar>
          <n-text>{{ authStore.username }}</n-text>
        </n-space>
        <n-button text size="large" @click="handleLogout" title="登出">
          <template #icon>
            <n-icon>
              <LogOutOutline />
            </n-icon>
          </template>
        </n-button>
      </n-space>
    </div>

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
      <div class="footer-actions">
        <n-icon @click="handleGithubClick" class="cursor-pointer" title="访问 GitHub 仓库" size="20">
          <LogoGithub />
        </n-icon>
        <n-icon @click="handleRefresh" :loading="appStore.globalLoading" class="cursor-pointer" title="刷新数据" size="20">
          <template v-if="appStore.globalLoading">
            <RefreshOutline class="rotating" />
          </template>
          <template v-else>
            <RefreshOutline />
          </template>
        </n-icon>
      </div>
    </div>
  </n-el>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'
import { useAuthStore } from '@/store/auth'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { useSettingStore } from '@/store/setting'
import type { MenuOption } from 'naive-ui'
import { useMessage } from 'naive-ui'
import {
  HomeOutline,
  LayersOutline,
  ArchiveOutline,
  SettingsOutline,
  RefreshOutline,
  LogOutOutline,
  LogoGithub,
} from '@vicons/ionicons5'
import LogIcon from '@/assets/svg/log.svg?component'

interface Props {
  onMenuSelect?: () => void // 菜单选择后的回调，用于移动端关闭抽屉
}

const props = withDefaults(defineProps<Props>(), {
  onMenuSelect: undefined,
})

const route = useRoute()
const router = useRouter()
const message = useMessage()
const appStore = useAppStore()
const authStore = useAuthStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()
const settingStore = useSettingStore()

// 版本信息
const version = computed(() => settingStore.systemInfo?.version)
// 当前活跃的菜单项
const activeKey = computed(() => {
  const path = route.path
  if (path === '/') return 'home'
  if (path === '/containers') return 'containers'
  if (path === '/images') return 'images'
  if (path === '/logs') return 'logs'
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
    label: '日志',
    key: 'logs',
    icon: () => h(LogIcon),
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
    case 'logs':
      router.push('/logs')
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
      await containerStore.fetchContainers(false, true)
    } else if (activeKey.value === 'images') {
      await imageStore.fetchImages()
    } else {
      // 首页刷新所有数据
      await Promise.all([
        containerStore.fetchContainers(false, true),
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

// 登出处理
const handleLogout = async () => {
  try {
    await authStore.logout()
    message.success('已登出')
  } catch (error) {
    console.error('登出失败:', error)
    message.error('登出失败')
  }
}

// GitHub 跳转处理
const handleGithubClick = () => {
  window.open('https://github.com/jianxcao/watch-docker', '_blank')
}

onMounted(() => {
  settingStore.fetchSystemInfo()
})

</script>

<style scoped lang="less">
.sider-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding-bottom: var(--bottom-inset);
  padding-top: var(--top-inset);
}

.sider-header {
  height: 66px;
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

.user-info {
  margin-top: auto;
  padding: 12px;
  border-top: 1px solid var(--border-color);
  border-bottom: 1px solid var(--border-color);
}

.sider-footer {
  padding: 12px;
  height: 56px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border-color);
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
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

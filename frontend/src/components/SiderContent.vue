<template>
  <n-el class="sider-content">
    <!-- 标题 -->
    <div class="sider-header">
      <n-space align="center">
        <img src="/logo.svg" alt="Logo" class="logo" />
        <n-h3 style="margin: 0">Watch Docker</n-h3>
      </n-space>
    </div>

    <!-- 菜单 -->
    <n-menu
      :value="activeKey"
      :options="menuOptions"
      @update:value="handleMenuSelect"
      class="sider-menu"
    />

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
        <n-text depth="3" style="font-size: 12px">
          {{ version }}
        </n-text>
      </div>
      <div class="footer-actions">
        <n-icon
          @click="handleGithubClick"
          class="cursor-pointer"
          title="访问 GitHub 仓库"
          size="20"
        >
          <LogoGithub />
        </n-icon>
        <n-icon
          @click="handleRefresh"
          :loading="appStore.globalLoading"
          class="cursor-pointer"
          title="刷新数据"
          size="20"
        >
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
  TerminalOutline,
} from '@vicons/ionicons5'
import ComposeIcon from '@/assets/svg/compose.svg?component'
import LogIcon from '@/assets/svg/log.svg?component'
import VolumeIcon from '@/assets/svg/volume.svg?component'
import NetworkIcon from '@/assets/svg/network.svg?component'
import { renderIcon } from '@/common/utils'

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
  if (path === '/' || path === '/home') {
    return 'home'
  }
  if (path.startsWith('/containers')) {
    return 'containers'
  }
  if (path.startsWith('/images')) {
    return 'images'
  }
  if (path.startsWith('/compose')) {
    return 'compose'
  }
  if (path.startsWith('/volumes')) {
    return 'volumes'
  }
  if (path.startsWith('/networks')) {
    return 'networks'
  }
  if (path === '/logs') {
    return 'logs'
  }
  if (path === '/terminal') {
    return 'terminal'
  }
  if (path === '/settings') {
    return 'settings'
  }
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
const menuOptions = computed<MenuOption[]>(
  () =>
    [
      {
        label: '首页',
        key: 'home',
        icon: renderIcon(HomeOutline, { size: 20 }),
      },
      {
        label: 'Compose 项目',
        key: 'compose',
        icon: renderIcon(ComposeIcon, { size: 20 }),
      },
      {
        label: '容器管理',
        key: 'containers',
        icon: renderIcon(LayersOutline, { size: 20 }),
      },
      {
        label: '镜像管理',
        key: 'images',
        icon: renderIcon(ArchiveOutline, { size: 20 }),
      },
      {
        label: '网络管理',
        key: 'networks',
        icon: renderIcon(NetworkIcon, { size: 20 }),
      },
      {
        label: 'Volume 管理',
        key: 'volumes',
        icon: renderIcon(VolumeIcon, { size: 20 }),
      },
      {
        label: '日志',
        key: 'logs',
        icon: renderIcon(LogIcon, { size: 20 }),
      },
      settingStore.systemInfo?.isOpenDockerShell && {
        label: '终端',
        key: 'terminal',
        icon: renderIcon(TerminalOutline, { size: 20 }),
      },
      {
        label: '系统设置',
        key: 'settings',
        icon: renderIcon(SettingsOutline, { size: 20 }),
      },
    ].filter(Boolean) as MenuOption[],
)

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
    case 'compose':
      router.push('/compose')
      break
    case 'volumes':
      router.push('/volumes')
      break
    case 'networks':
      router.push('/networks')
      break
    case 'logs':
      router.push('/logs')
      break
    case 'terminal':
      router.push('/terminal')
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
      await Promise.all([containerStore.fetchContainers(false, true), imageStore.fetchImages()])
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
</script>

<style scoped lang="less">
.sider-content {
  display: flex;
  flex-direction: column;
  height: 100vh;
  padding-bottom: var(--bottom-inset);
  padding-top: var(--top-inset);
  box-sizing: border-box;
}

.sider-header {
  // 加下 border 是56px
  height: 55px;
  box-sizing: border-box;
  padding: 0 16px;
  border-bottom: 1px solid var(--divider-color);
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
  padding: 12px;
  border-top: 1px solid var(--divider-color);
  border-bottom: 1px solid var(--divider-color);
  box-sizing: border-box;
}

.sider-footer {
  padding: 12px;
  height: 56px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--divider-color);
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

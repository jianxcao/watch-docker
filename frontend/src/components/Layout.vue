<template>
  <n-layout has-sider class="layout-container">
    <!-- 侧边菜单 (大屏幕) -->
    <n-layout-sider v-if="isLargeScreen" :width="240" :collapsed-width="0" collapse-mode="width" bordered
      show-trigger="bar" class="layout-sider">
      <SiderContent />
    </n-layout-sider>

    <!-- 主内容区域 -->
    <n-layout class="main-layout">
      <!-- 顶部栏 (小屏幕) -->
      <n-layout-header bordered class="mobile-header">
        <n-space align="center" justify="space-between" style="height: 100%;">
          <div class="flex items-center justify-center gap-1">
            <n-button text @click="appStore.toggleDrawer" v-if="isSmallScreen">
              <template #icon>
                <n-icon size="20">
                  <MenuOutline />
                </n-icon>
              </template>
            </n-button>
            <n-h3 style="margin: 0;">{{ currentPageTitle }}</n-h3>
          </div>

          <!-- 切换主题 -->
          <n-button quaternary circle size="small" @click="onToggleTheme" class="flex items-center justify-center">
            <template #icon>
              <n-icon :component="isDark ? MoonIcon : SunIcon" />
            </template>
          </n-button>
        </n-space>
      </n-layout-header>

      <!-- 内容区域 -->
      <n-layout-content class="layout-content" position="static">
        <router-view />
      </n-layout-content>
      <n-el id="footer"></n-el>
    </n-layout>
  </n-layout>

  <!-- 移动端抽屉菜单 (仅小屏幕显示) -->
  <MobileDrawer v-if="isSmallScreen" />

</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { useResponsive } from '@/hooks/useResponsive'
import { healthApi } from '@/common/api'
import SiderContent from './SiderContent.vue'
import MobileDrawer from './MobileDrawer.vue'
import { MenuOutline } from '@vicons/ionicons5'
import { useSettingStore } from '@/store/setting'
import { Moon as MoonIcon, Sunny as SunIcon } from '@vicons/ionicons5'

const route = useRoute()
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()
const { isLargeScreen, isSmallScreen } = useResponsive()
const settingStore = useSettingStore()
const isDark = computed(() => settingStore.setting.theme === 'dark')
function onToggleTheme() {
  settingStore.setTheme(isDark.value ? 'light' : 'dark')
}
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
@import '@/styles/mix.less';

.layout-container {
  height: 100vh;


  :deep(.n-layout-scroll-container) {
    .scrollbar();
  }
}

.layout-content {
  padding: 16px;
}

#footer {
  position: sticky;
  bottom: 0;
  z-index: 100;
}


.mobile-header {
  height: 56px;
  box-sizing: border-box;
  padding: 0 16px;
  position: sticky;
  top: 0;
  z-index: 100;
}

// 响应式调整
@media (max-width: 768px) {
  .layout-content {
    padding: 8px;
  }
}
</style>

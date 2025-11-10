<template>
  <n-layout has-sider class="layout-container">
    <!-- 侧边菜单 (大屏幕) -->
    <n-layout-sider
      v-if="isLargeScreen"
      :width="240"
      :collapsed-width="0"
      collapse-mode="width"
      bordered
      show-trigger="bar"
      class="layout-sider"
    >
      <SiderContent />
    </n-layout-sider>

    <!-- 主内容区域 -->
    <n-layout class="main-layout" :class="layoutClass">
      <n-layout-header bordered class="header-wrap">
        <div class="header-content">
          <n-button text @click="appStore.toggleDrawer" v-if="isSmallScreen">
            <template #icon>
              <n-icon size="20">
                <MenuOutline />
              </n-icon>
            </template>
          </n-button>

          <div id="header"></div>
          <n-button
            quaternary
            circle
            size="small"
            @click="onToggleTheme"
            class="flex items-center justify-center"
          >
            <template #icon>
              <n-icon :component="isDark ? MoonIcon : SunIcon" />
            </template>
          </n-button>
        </div>
      </n-layout-header>

      <!-- 内容区域 -->
      <n-layout-content class="layout-content" position="static">
        <router-view class="h-full" />
      </n-layout-content>
      <n-el id="footer"></n-el>
    </n-layout>
  </n-layout>

  <!-- 移动端抽屉菜单 (仅小屏幕显示) -->
  <MobileDrawer v-if="isSmallScreen" />
</template>

<script setup lang="ts">
import { useResponsive } from '@/hooks/useResponsive'
import { useSettingStore } from '@/store/setting'
import { useAppStore } from '@/store/app'
import { Moon as MoonIcon, Sunny as SunIcon, MenuOutline } from '@vicons/ionicons5'
import { computed } from 'vue'
import MobileDrawer from './MobileDrawer.vue'
import SiderContent from './SiderContent.vue'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { sleep } from '@/common/utils'
const route = useRoute()

const layoutClass = computed(() => route.meta.layoutClass)

const appStore = useAppStore()
const { isLargeScreen, isSmallScreen } = useResponsive()
const settingStore = useSettingStore()
const isDark = computed(() => settingStore.setting.theme === 'dark')
function onToggleTheme() {
  settingStore.setTheme(isDark.value ? 'light' : 'dark')
}

const containerStore = useContainerStore()
const imageStore = useImageStore()

watchEffect(() => {
  document.body.setAttribute('data-theme', settingStore.setting.theme)
})

onMounted(async () => {
  await appStore.checkHealth()
  Promise.all([
    settingStore.fetchSystemInfo(),
    containerStore.fetchContainers(true, false),
    imageStore.fetchImages(),
    containerStore.statsWebSocket.connect(),
  ])
  imageStore.startImagesPolling()
})

onUnmounted(() => {
  imageStore.stopImagesPolling()

  containerStore.statsWebSocket.disconnect()
})

async function refresh() {
  if (appStore.systemHealth === 'unhealthy') {
    await appStore.checkHealth()
  }
  containerStore.fetchContainers(true, false)
  imageStore.fetchImages()
  if (!containerStore.statsWebSocket.isConnected) {
    containerStore.statsWebSocket.connect()
  }
}

const visibility = useDocumentVisibility()

watch(visibility, (newVal) => {
  console.debug('visibility', newVal)
  if (newVal === 'visible') {
    sleep(1000).then(() => {
      console.debug('页面可见重新刷新')
      refresh()
    })
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
  padding: var(--layout-padding);
  box-sizing: border-box;
}

#footer {
  position: sticky;
  bottom: 0;
  z-index: 100;
  padding-top: 8px;
  background: color-mix(in srgb, var(--card-color) 30%, transparent);
  border-top: 1px solid var(--border-color);
  z-index: 100;
  padding-bottom: max(var(--bottom-inset), 8px);
}

#footer:empty {
  display: none !important;
}

.header-wrap {
  padding-top: var(--top-inset);
  position: sticky;
  top: 0;
  z-index: 100;
}

.header-content {
  padding: 0 16px;
  // 加下 border 是56px
  height: 55px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;

  #header {
    flex: 1;
    height: 100%;
  }
}

@supports (backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)) {
  .header-wrap,
  #footer {
    background-color: transparent;
    -webkit-backdrop-filter: blur(10px);
    backdrop-filter: blur(10px);
  }
}
</style>

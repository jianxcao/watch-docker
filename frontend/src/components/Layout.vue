<template>
  <n-layout has-sider class="layout-container">
    <!-- 侧边菜单 (大屏幕) -->
    <n-layout-sider v-if="isLargeScreen" :width="240" :collapsed-width="0" collapse-mode="width" bordered
      show-trigger="bar" class="layout-sider">
      <SiderContent />
    </n-layout-sider>

    <!-- 主内容区域 -->
    <n-layout class="main-layout">
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
          <n-button quaternary circle size="small" @click="onToggleTheme" class="flex items-center justify-center">
            <template #icon>
              <n-icon :component="isDark ? MoonIcon : SunIcon" />
            </template>
          </n-button>
        </div>
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
import { useResponsive } from '@/hooks/useResponsive'
import { useSettingStore } from '@/store/setting'
import { useAppStore } from '@/store/app'
import { Moon as MoonIcon, Sunny as SunIcon, MenuOutline } from '@vicons/ionicons5'
import { computed } from 'vue'
import MobileDrawer from './MobileDrawer.vue'
import SiderContent from './SiderContent.vue'

const appStore = useAppStore()
const { isLargeScreen, isSmallScreen } = useResponsive()
const settingStore = useSettingStore()
const isDark = computed(() => settingStore.setting.theme === 'dark')
function onToggleTheme() {
  settingStore.setTheme(isDark.value ? 'light' : 'dark')
}

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
  height: 56px;
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

// 响应式调整
@media (max-width: 768px) {
  .layout-content {
    padding: 8px;
  }
}
</style>

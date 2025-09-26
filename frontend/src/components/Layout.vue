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
        <div id="header"></div>
        <n-button quaternary circle size="small" @click="onToggleTheme" class="flex items-center justify-center">
          <template #icon>
            <n-icon :component="isDark ? MoonIcon : SunIcon" />
          </template>
        </n-button>
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
import { Moon as MoonIcon, Sunny as SunIcon } from '@vicons/ionicons5'
import { computed } from 'vue'
import MobileDrawer from './MobileDrawer.vue'
import SiderContent from './SiderContent.vue'

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
}

.header-wrap {
  height: 66px;
  box-sizing: border-box;
  padding: 0 16px;
  position: sticky;
  display: flex;
  align-items: center;
  justify-content: space-between;
  top: 0;
  z-index: 100;
  gap: 12px;

  #header {
    flex: 1;
    height: 100%;
  }
}

// 响应式调整
@media (max-width: 768px) {
  .layout-content {
    padding: 8px;
  }
}
</style>

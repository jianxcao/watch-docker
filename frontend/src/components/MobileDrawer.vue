<template>
  <n-drawer
    v-model:show="drawerVisible"
    :width="280"
    placement="left"
    :trap-focus="false"
    :block-scroll="false"
    class="mobile-drawer-menu"
  >
    <SiderContent :on-menu-select="handleMenuSelect" />
  </n-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/store/app'
import SiderContent from './SiderContent.vue'

const appStore = useAppStore()

// 抽屉可见性
const drawerVisible = computed({
  get: () => appStore.drawerVisible,
  set: (value) => {
    if (!value) {
      appStore.closeDrawer()
    }
  },
})

// 处理菜单选择（关闭抽屉）
const handleMenuSelect = () => {
  appStore.closeDrawer()
}
</script>

<style lang="less">
.mobile-drawer-menu {
  background-color: color-mix(in srgb, var(--n-color) 50%, transparent);
  backdrop-filter: blur(30px) brightness(95%);
}
</style>

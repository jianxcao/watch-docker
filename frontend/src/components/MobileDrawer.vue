<template>
  <n-drawer
    v-model:show="drawerVisible"
    :width="280"
    placement="left"
    :trap-focus="false"
    :block-scroll="false"
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

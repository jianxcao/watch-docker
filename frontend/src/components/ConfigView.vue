<script setup lang="ts">
import { useThemeVars, useMessage } from 'naive-ui'
import { useSettingStore } from '@/store/setting'
window.$message = useMessage()
const settingStore = useSettingStore()
const theme = useThemeVars()
watch(
  theme,
  () => {
    settingStore.setThemeVars(theme.value)
    // window.THEME_VARS = theme.value
  },
  {
    immediate: true,
    deep: true,
  },
)
</script>

<template>
  <slot />
</template>

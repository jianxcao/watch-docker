<template>
  <n-config-provider :theme="theme">
    <n-global-style />
    <n-el :class="$style.container">
      <n-modal-provider>
        <n-dialog-provider>
          <n-message-provider placement="bottom-right">
            <config-view> <router-view /></config-view>
          </n-message-provider>
        </n-dialog-provider>
      </n-modal-provider>
    </n-el>
  </n-config-provider>
</template>

<script setup lang="ts">
import { useSettingStore } from '@/store/setting'
import { darkTheme } from 'naive-ui'

const settingStore = useSettingStore()
const theme = computed(() => (settingStore.setting.theme === 'dark' ? darkTheme : null))

watchEffect(() => {
  document.body.setAttribute('data-theme', settingStore.setting.theme)
})
</script>

<style lang="less" module>
.container {
  height: 100vh;
  width: 100vw;
  box-sizing: border-box;
}
</style>

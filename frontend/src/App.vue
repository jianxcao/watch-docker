<script setup lang="ts">
import { darkTheme } from 'naive-ui'
import { useSettingStore } from '@/store/setting'
import { healthApi } from './common/api'
import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'

const settingStore = useSettingStore()
const theme = computed(() => (settingStore.setting.theme === 'dark' ? darkTheme : null))
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()

watchEffect(() => {
  document.documentElement.setAttribute('data-theme', settingStore.setting.theme)
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

onMounted(async () => {
  // console.log('App mounted')
  await checkHealth()
  await Promise.all([
    containerStore.fetchContainers(true, false),
    imageStore.fetchImages(),
  ])
})
</script>

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

<style lang="less" module>
.container {
  height: 100vh;
  width: 100vw;
  height: 100dvh;
  width: 100dvw;
  box-sizing: border-box;
  padding-top: var(--top-inset);
  padding-bottom: var(--bottom-inset);
}
</style>

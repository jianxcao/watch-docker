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
import { darkTheme } from 'naive-ui'
import { useSettingStore } from '@/store/setting'

import { useAppStore } from '@/store/app'
import { useContainerStore } from '@/store/container'
import { useImageStore } from '@/store/image'
import { sleep } from './common/utils'

const settingStore = useSettingStore()
const theme = computed(() => (settingStore.setting.theme === 'dark' ? darkTheme : null))
const appStore = useAppStore()
const containerStore = useContainerStore()
const imageStore = useImageStore()

watchEffect(() => {
  document.body.setAttribute('data-theme', settingStore.setting.theme)
})


onMounted(async () => {
  await appStore.checkHealth()
  Promise.all([
    containerStore.fetchContainers(true, false),
    imageStore.fetchImages(),
    containerStore.startStatsWebSocket(),
  ])
  imageStore.startImagesPolling()
})

onUnmounted(() => {
  imageStore.stopImagesPolling()
  containerStore.stopStatsWebSocket()
})


async function refresh() {
  if (appStore.systemHealth === 'unhealthy') {
    await appStore.checkHealth()
  }
  containerStore.fetchContainers(true, false)
  imageStore.fetchImages()
  if (!containerStore.statsWebSocket.isConnected) {
    containerStore.startStatsWebSocket()
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



<style lang="less" module>
.container {
  height: 100vh;
  width: 100vw;
  box-sizing: border-box;
}
</style>

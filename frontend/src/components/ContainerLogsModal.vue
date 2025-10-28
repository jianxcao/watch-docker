<template>
  <n-modal
    v-model:show="show"
    :icon="getIcon()"
    display-directive="if"
    preset="dialog"
    :title="title"
    class="compose-logs-modal"
    contentClass="compose-logs-modal-content"
    :style="{
      padding: '8px',
      width: '90vw',
      maxWidth: '1200px',
      height: '90vh',
    }"
    :mask-closable="false"
    :closable="true"
    @after-leave="handleClose"
  >
    <logs-stream-view ref="logsStreamViewRef" :socket-url="socketUrl" />
    <template #action>
      <n-space justify="end">
        <n-button @click="handleReconnect" :disabled="isConnecting || isConnected">
          <template #icon>
            <n-icon>
              <RefreshOutline />
            </n-icon>
          </template>
          重新连接
        </n-button>
        <n-button @click="handleClearLogs">
          <template #icon>
            <n-icon>
              <TrashOutline />
            </n-icon>
          </template>
          清空日志
        </n-button>
        <n-button @click="show = false">关闭</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import ComposeIcon from '@/assets/svg/compose.svg?component'
import type { ContainerStatus } from '@/common/types'
import { renderIcon } from '@/common/utils'
import LogsStreamView from '@/components/LogsStreamView.vue'
import { useSettingStore } from '@/store/setting'
import { RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import { useThemeVars } from 'naive-ui'
import { computed } from 'vue'

interface Props {
  container: ContainerStatus | null
}

const props = defineProps<Props>()
const show = defineModel<boolean>('show')
const settingStore = useSettingStore()
const theme = useThemeVars()
const logsStreamViewRef = ref<InstanceType<typeof LogsStreamView>>()
const isConnecting = computed(() => logsStreamViewRef.value?.isConnecting)
const isConnected = computed(() => logsStreamViewRef.value?.isConnected)

const title = computed(() => {
  if (!props.container?.name) {
    return '容器日志'
  }
  return `容器日志 - ${props.container.name}`
})

const getIcon = () => {
  return renderIcon(ComposeIcon, {
    color: theme.value.primaryColor,
    size: 20,
  })
}

const socketUrl = computed(() => {
  if (!props.container) {
    return undefined
  }
  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1/containers/logs/${props.container.id}/ws?token=${token}&id=${encodeURIComponent(props.container.id)}&projectName=${encodeURIComponent(props.container.name)}`
})

const handleClose = () => {
  logsStreamViewRef.value?.close()
}

const handleReconnect = () => {
  logsStreamViewRef.value?.handleReconnect()
}

const handleClearLogs = () => {
  logsStreamViewRef.value?.handleClearLogs()
}
</script>

<style lang="less">
.compose-logs-modal {
  .n-dialog__content {
    margin-bottom: 0;
    margin-top: 0;
  }
  .n-dialog__close {
    top: -8px;
  }
}
</style>

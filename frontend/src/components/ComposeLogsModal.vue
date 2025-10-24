<template>
  <n-modal
    v-model:show="show"
    :icon="getIcon()"
    display-directive="if"
    preset="dialog"
    :title="title"
    class="compose-logs-modal"
    :style="{
      padding: '12px',
      width: '90vw',
      maxWidth: '1200px',
      height: '80vh',
    }"
    :mask-closable="false"
    :closable="true"
    @after-leave="handleClose"
  >
    <div class="logs-container">
      <Term
        ref="termRef"
        :config="termConfig"
        @ready="handleTermReady"
        height="calc(80vh - 42px)"
      />
    </div>
    <template #footer>
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
import type { ComposeProject } from '@/common/types'
import { renderIcon } from '@/common/utils'
import { useSettingStore } from '@/store/setting'
import { RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import { useWebSocket } from '@vueuse/core'
import { useMessage, useThemeVars } from 'naive-ui'
import { computed, ref } from 'vue'
import Term, { type TermConfig } from './Term/TermView.vue'

interface Props {
  project: ComposeProject | null
}

const props = defineProps<Props>()
const show = defineModel<boolean>('show')

const message = useMessage()
const settingStore = useSettingStore()
const termRef = ref<InstanceType<typeof Term>>()
const theme = useThemeVars()

const title = computed(() => {
  if (!props.project) {
    return 'Compose 日志'
  }
  return `Compose 日志 - ${props.project.name}`
})

const getIcon = () => {
  return renderIcon(ComposeIcon, {
    color: theme.value.primaryColor,
    size: 20,
  })
}

// 终端配置（日志查看模式）
const termConfig: TermConfig = {
  disableStdin: navigator.maxTouchPoints > 0,
  cursorBlink: false,
  fontSize: 13,
  scrollback: 1000,
  convertEol: true,
}

const socketUrl = computed(() => {
  if (!props.project) {
    return undefined
  }
  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1/compose/logs/${props.project.name}/ws?token=${token}&composeFile=${encodeURIComponent(props.project.composeFile)}&projectName=${encodeURIComponent(props.project.name)}`
})

// 使用 VueUse 的 useWebSocket
const { status, close, open } = useWebSocket(socketUrl, {
  autoReconnect: false,
  immediate: false,
  autoConnect: false,

  // 直接处理消息事件，支持二进制数据
  onMessage: (_ws, event) => {
    if (termRef.value && event.data) {
      // 处理二进制消息
      if (event.data instanceof ArrayBuffer) {
        termRef.value.write(new Uint8Array(event.data))
      } else if (typeof event.data === 'string') {
        // 兼容文本消息
        termRef.value.write(event.data)
      }
    }
  },
  onConnected: (_ws) => {
    termRef.value?.writeln('\x1b[32m已连接到日志流\x1b[0m\r\n')
    // 设置二进制类型为 arraybuffer
    if (_ws) {
      _ws.binaryType = 'arraybuffer'
    }
  },
  onDisconnected: () => {
    termRef.value?.writeln('\r\n\x1b[33m日志流已断开\x1b[0m\r\n')
  },
  onError: () => {
    termRef.value?.writeln('\r\n\x1b[31m连接错误\x1b[0m\r\n')
    message.error('日志连接失败')
  },
})

// 连接状态
const isConnecting = computed(() => status.value === 'CONNECTING')
const isConnected = computed(() => status.value === 'OPEN')

// 终端就绪回调
const handleTermReady = () => {
  // 终端初始化完成，可以开始连接
  termRef.value?.writeln('\r\n\x1b[33m正在连接日志流...\x1b[0m\r\n')
  open()
}

// 重新连接
const handleReconnect = () => {
  close()
  termRef.value?.clear()
  termRef.value?.writeln('\r\n\x1b[33m正在重新连接日志流...\x1b[0m\r\n')
  // 等待关闭完成后再打开
  setTimeout(() => {
    open()
  }, 100)
}

// 清空日志
const handleClearLogs = () => {
  termRef.value?.clear()
}

// 弹窗关闭后清理
const handleClose = () => {
  close()
}
</script>

<style scoped lang="less">
.compose-logs-modal {
  :deep(.n-scrollbar-container) {
    overflow: hidden !important;
  }
}

.logs-container {
  flex: 1;
  overflow: hidden;
  padding-block: 8px;
}

// 浅色主题适配
:deep([data-theme='light']) {
  .logs-container {
    background: #f8f9fa;
  }
}
</style>

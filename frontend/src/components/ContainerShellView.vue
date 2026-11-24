<template>
  <div class="container-shell-view">
    <div class="shell-toolbar">
      <n-space align="center">
        <n-text>Shell 类型:</n-text>
        <n-select
          v-model:value="selectedShell"
          :options="shellOptions"
          size="small"
          style="width: 120px"
          :disabled="isConnected || isConnecting"
        />
        <n-button
          v-if="!isConnected"
          type="primary"
          size="small"
          @click="connect"
          :loading="isConnecting"
          :disabled="!isRunning"
        >
          连接
        </n-button>
        <n-button v-else type="error" size="small" @click="() => disconnect()"> 断开 </n-button>
        <n-tag :type="connectionStatusType" size="small">
          {{ connectionStatusText }}
        </n-tag>
      </n-space>
    </div>

    <div class="terminal-container" :style="{ height: terminalHeight }">
      <div v-if="!isRunning" class="terminal-message">
        <n-empty description="容器未运行，无法连接 Shell" />
      </div>
      <div v-else-if="errorMessage" class="terminal-message error">
        <n-result status="error" :title="errorMessage">
          <template #footer>
            <n-button @click="connect">重新连接</n-button>
          </template>
        </n-result>
      </div>
      <div v-else-if="!isConnected && !isConnecting" class="terminal-message">
        <n-empty description="点击连接按钮开始使用 Shell" />
      </div>
      <TermView
        v-else
        ref="terminalRef"
        :config="termConfig"
        :auto-fit="true"
        height="100%"
        @ready="handleTerminalReady"
        @data="handleTerminalData"
        @resize="handleTerminalResize"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useWebSocket } from '@vueuse/core'
import TermView from '@/components/Term/TermView.vue'
import type { Terminal } from '@xterm/xterm'
import { useSettingStore } from '@/store/setting'
import { useAuthStore } from '@/store/auth'
import { API_ENDPOINTS } from '@/constants/api'

interface Props {
  containerId: string
  containerName: string
  isRunning: boolean
}

const props = defineProps<Props>()
const message = useMessage()
const settingStore = useSettingStore()

// 状态
const terminalRef = ref<InstanceType<typeof TermView>>()
const terminal = ref<Terminal>()
const errorMessage = ref('')
const selectedShell = ref('sh')
const wsUrl = ref('')
const shellState = ref<'CLOSED' | 'CONNECTING' | 'OPEN'>('CLOSED')
let isUnmounting = false

// Shell 选项
const shellOptions = [
  { label: 'sh', value: 'sh' },
  { label: 'bash', value: 'bash' },
  { label: 'zsh', value: 'zsh' },
  { label: 'ash', value: 'ash' },
]

// 终端配置
const termConfig = {
  cursorBlink: true,
  disableStdin: false,
  fontSize: 13,
}

// 终端高度
const terminalHeight = computed(() => {
  return 'calc(100% - 48px)'
})

const authStore = useAuthStore()

// WebSocket URL
const getWebSocketUrl = () => {
  const token = settingStore.getToken()
  if (authStore.authEnabled && !token) {
    throw new Error('未登录，请先登录后再连接 Shell')
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const baseUrl = API_ENDPOINTS.CONTAINER_SHELL_WS(props.containerId, selectedShell.value)
  const fullBase = `${protocol}//${host}/api/v1${baseUrl}`
  return token ? `${fullBase}&token=${encodeURIComponent(token)}` : fullBase
}

const pendingChunks: Array<string | Uint8Array> = []

const appendShellChunk = (chunk: string | Uint8Array) => {
  if (!terminal.value) {
    pendingChunks.push(chunk)
    return
  }
  terminal.value.write(chunk)
}

// 使用 VueUse 的 useWebSocket
const { ws, open, close } = useWebSocket(wsUrl, {
  immediate: false, // 不自动连接，等用户点击
  autoReconnect: false, // 禁用自动重连
  heartbeat: false, // 终端不需要心跳
  autoConnect: false,
  onMessage: (_ws, event) => {
    if (isUnmounting || _ws !== ws.value) {
      return
    }
    const newData = event.data
    if (!newData) {
      return
    }
    // 处理接收到的数据（TermView 尚未 @ready 时先缓冲，避免丢失初始提示符）
    if (newData instanceof ArrayBuffer) {
      appendShellChunk(new Uint8Array(newData))
    } else if (newData instanceof Blob) {
      void newData.arrayBuffer().then((buffer) => {
        if (isUnmounting || _ws !== ws.value) {
          return
        }
        appendShellChunk(new Uint8Array(buffer))
      })
    } else if (typeof newData === 'string') {
      appendShellChunk(newData)
    }
  },
  onConnected: (_ws) => {
    if (isUnmounting || _ws !== ws.value) {
      return
    }
    shellState.value = 'OPEN'
    _ws.binaryType = 'arraybuffer'
    errorMessage.value = ''
    message.success('Shell 连接成功')

    // 连接成功后，发送初始大小
    if (terminal.value) {
      handleTerminalResize({
        cols: terminal.value.cols,
        rows: terminal.value.rows,
      })
    }
  },
  onError: (_ws, event) => {
    if (isUnmounting || _ws !== ws.value) {
      return
    }
    shellState.value = 'CLOSED'
    console.error('WebSocket error:', event)
    errorMessage.value = 'WebSocket 连接错误'
    message.error('Shell 连接失败')
  },
  onDisconnected: (_ws) => {
    if (isUnmounting || _ws !== ws.value) {
      return
    }
    shellState.value = 'CLOSED'
    terminal.value = undefined
    pendingChunks.length = 0
    if (!errorMessage.value) {
      message.info('Shell 连接已关闭')
    }
  },
})

// 连接状态
const isConnected = computed(() => shellState.value === 'OPEN')
const isConnecting = computed(() => shellState.value === 'CONNECTING')

const connectionStatusType = computed(() => {
  if (isConnected.value) {
    return 'success'
  }
  if (isConnecting.value) {
    return 'warning'
  }
  return 'default'
})

const connectionStatusText = computed(() => {
  if (isConnected.value) {
    return '已连接'
  }
  if (isConnecting.value) {
    return '连接中'
  }
  return '未连接'
})

// 连接 WebSocket
const connect = () => {
  if (!props.isRunning) {
    message.warning('容器未运行，无法连接 Shell')
    return
  }
  if (isConnecting.value) {
    return
  }

  try {
    errorMessage.value = ''
    pendingChunks.length = 0
    wsUrl.value = getWebSocketUrl()
    console.debug('wsUrl', wsUrl.value)
    shellState.value = 'CONNECTING'
    open()
  } catch (error: any) {
    shellState.value = 'CLOSED'
    console.error('Failed to connect WebSocket:', error)
    errorMessage.value = error.message || '连接失败'
    message.error('Shell 连接失败: ' + error.message)
  }
}

// 断开连接
const disconnect = (silent = false) => {
  const wasConnected = shellState.value === 'OPEN'
  shellState.value = 'CLOSED'
  terminal.value = undefined
  pendingChunks.length = 0
  close()
  if (wasConnected && !silent && !isUnmounting) {
    message.info('Shell 连接已关闭')
  }
}

// 终端就绪
const handleTerminalReady = (term: Terminal) => {
  terminal.value = term

  while (pendingChunks.length > 0) {
    const chunk = pendingChunks.shift()
    if (chunk !== undefined) {
      term.write(chunk)
    }
  }

  // 如果已经连接，发送终端大小
  if (isConnected.value) {
    handleTerminalResize({
      cols: term.cols,
      rows: term.rows,
    })
  }
}

// 终端数据输入
const handleTerminalData = (data: string) => {
  if (ws.value?.readyState === WebSocket.OPEN) {
    ws.value.send(data)
  }
}

// 终端大小变化
const handleTerminalResize = (size: { cols: number; rows: number }) => {
  if (ws.value?.readyState === WebSocket.OPEN) {
    // 发送 resize 消息
    const resizeMsg = JSON.stringify({
      type: 'resize',
      rows: size.rows,
      cols: size.cols,
    })
    ws.value.send(resizeMsg)
  }
}

// 监听容器运行状态与容器 ID 变化
watch(
  () => props.isRunning,
  (newValue) => {
    if (!newValue && (isConnected.value || isConnecting.value)) {
      const wasConnected = isConnected.value
      disconnect(true)
      if (wasConnected) {
        message.warning('容器已停止，Shell 连接已断开')
      }
    }
  },
)

watch(
  () => props.containerId,
  () => {
    errorMessage.value = ''
    disconnect(true)
  },
)

onBeforeUnmount(() => {
  isUnmounting = true
  disconnect(true)
})
</script>

<style scoped lang="less">
.container-shell-view {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--n-color-embedded);

  .shell-toolbar {
    padding: 8px 16px;
    border-bottom: 1px solid var(--n-border-color);
    background: var(--n-color);
  }

  .terminal-container {
    flex: 1;
    position: relative;
    overflow: hidden;

    .terminal-message {
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100%;

      &.error {
        background: var(--n-color);
      }
    }
  }
}
</style>

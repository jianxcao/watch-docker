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
          :disabled="isConnected"
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
        <n-button v-else type="error" size="small" @click="disconnect"> 断开 </n-button>
        <n-tag :type="connectionStatusType" size="small">
          {{ connectionStatusText }}
        </n-tag>
      </n-space>
    </div>

    <div class="terminal-container" :style="{ height: terminalHeight }">
      <div v-if="!isRunning" class="terminal-message">
        <n-empty description="容器未运行，无法连接 Shell" />
      </div>
      <div v-else-if="!isConnected && !isConnecting" class="terminal-message">
        <n-empty description="点击连接按钮开始使用 Shell" />
      </div>
      <div v-else-if="errorMessage" class="terminal-message error">
        <n-result status="error" :title="errorMessage">
          <template #footer>
            <n-button @click="connect">重新连接</n-button>
          </template>
        </n-result>
      </div>
      <TermView
        v-else
        ref="terminalRef"
        :config="termConfig"
        :auto-fit="true"
        :height="terminalHeight"
        @ready="handleTerminalReady"
        @data="handleTerminalData"
        @resize="handleTerminalResize"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useWebSocket } from '@vueuse/core'
import TermView from '@/components/Term/TermView.vue'
import type { Terminal } from '@xterm/xterm'
import { useSettingStore } from '@/store/setting'
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

// WebSocket URL
const getWebSocketUrl = () => {
  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const baseUrl = API_ENDPOINTS.CONTAINER_SHELL_WS(props.containerId, selectedShell.value)
  return `${protocol}//${host}/api/v1${baseUrl}&token=${token}`
}

// 使用 VueUse 的 useWebSocket
const { status, send, open, close } = useWebSocket(wsUrl, {
  immediate: false, // 不自动连接，等用户点击
  autoReconnect: false, // 禁用自动重连
  heartbeat: false, // 终端不需要心跳
  autoConnect: false,
  onMessage: (_ws, event) => {
    const newData = event.data
    if (!newData || !terminal.value) {
      return
    }
    // 处理接收到的数据
    if (newData instanceof Blob) {
      // 二进制数据
      newData.arrayBuffer().then((buffer) => {
        terminal.value?.write(new Uint8Array(buffer))
      })
    } else if (typeof newData === 'string') {
      // 文本数据
      terminal.value.write(newData)
    }
  },
  onConnected: () => {
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
    console.error('WebSocket error:', event)
    errorMessage.value = 'WebSocket 连接错误'
    message.error('Shell 连接失败')
  },
  onDisconnected: () => {
    if (!errorMessage.value) {
      message.info('Shell 连接已关闭')
    }
  },
})

// 连接状态
const isConnected = computed(() => status.value === 'OPEN')
const isConnecting = computed(() => status.value === 'CONNECTING')

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

  try {
    errorMessage.value = ''
    wsUrl.value = getWebSocketUrl()
    console.debug('wsUrl', wsUrl.value)
    open()
  } catch (error: any) {
    console.error('Failed to connect WebSocket:', error)
    errorMessage.value = error.message || '连接失败'
    message.error('Shell 连接失败: ' + error.message)
  }
}

// 断开连接
const disconnect = () => {
  close()
}

// 终端就绪
const handleTerminalReady = (term: Terminal) => {
  terminal.value = term

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
  if (isConnected.value) {
    send(data)
  }
}

// 终端大小变化
const handleTerminalResize = (size: { cols: number; rows: number }) => {
  if (isConnected.value) {
    // 发送 resize 消息
    const resizeMsg = JSON.stringify({
      type: 'resize',
      rows: size.rows,
      cols: size.cols,
    })
    send(resizeMsg)
  }
}

// 监听容器运行状态变化
watch(
  () => props.isRunning,
  (newValue) => {
    if (!newValue && isConnected.value) {
      disconnect()
      message.warning('容器已停止，Shell 连接已断开')
    }
  },
)
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

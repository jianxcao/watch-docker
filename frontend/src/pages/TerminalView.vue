<template>
  <div class="terminal-page">
    <div class="terminal-container">
      <div v-if="connectionError" class="terminal-error">
        <n-icon size="48" :color="theme.errorColor">
          <CloseCircleOutline />
        </n-icon>
        <n-text>{{ connectionError }}</n-text>
        <n-button type="primary" size="small" @click="reconnect" :loading="connecting">
          重新连接
        </n-button>
      </div>
      <TermView
        v-if="!connectionError"
        ref="termRef"
        :config="termConfig"
        :height="termHeight"
        @ready="handleTermReady"
        @data="handleTermData"
        @resize="handleTermResize"
      />
    </div>

    <!-- 页面标题信息 -->
    <Teleport to="#header" defer>
      <div class="welcome-card">
        <div class="terminal-title">
          <n-icon :color="theme.primaryColor">
            <TerminalOutline />
          </n-icon>
          <span>终端</span>
        </div>
        <div class="terminal-actions">
          <n-button circle size="tiny" @click="reconnect" :loading="connecting">
            <template #icon>
              <n-icon>
                <RefreshOutline />
              </n-icon>
            </template>
          </n-button>
          <n-button circle size="tiny" @click="clearTerminal">
            <template #icon>
              <n-icon>
                <TextClearIcon />
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useWebSocket } from '@vueuse/core'
import { useMessage, useThemeVars } from 'naive-ui'
import { RefreshOutline, CloseCircleOutline, TerminalOutline } from '@vicons/ionicons5'
import TextClearIcon from '@/assets/svg/textClear.svg?component'
import TermView from '@/components/Term/TermView.vue'
import type { Terminal } from '@xterm/xterm'
import { useSettingStore } from '@/store/setting'

const theme = useThemeVars()
const message = useMessage()
const settingStore = useSettingStore()
const termRef = ref<InstanceType<typeof TermView>>()
const terminal = ref<Terminal>()
const connectionError = ref<string>('')
const connecting = ref(false)
const isTerminalReady = ref(false)

// 终端配置
const termConfig = {
  disableStdin: false, // 启用输入
  cursorBlink: true,
  convertEol: true,
  scrollback: 10000,
  fontSize: 14,
  // 添加支持中文的字体 - 等宽字体优先，中文字体作为后备
  fontFamily:
    'Menlo, Monaco, "Courier New", monospace, "Microsoft YaHei", "微软雅黑", "PingFang SC", "Hiragino Sans GB", "Heiti SC", "WenQuanYi Micro Hei", sans-serif',
  fontWeight: 'normal',
  fontWeightBold: 'bold',
  letterSpacing: 0,
  lineHeight: 1.2,
}

// 计算终端高度
const termHeight = computed(() => {
  return `calc(100vh - ${settingStore.contentSafeTop + settingStore.contentSafeBottom}px)`
})

const socketUrl = computed(() => {
  const token = settingStore.getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1/shell?token=${token}`
})

// 使用 VueUse 的 useWebSocket
const {
  status,
  send,
  open: openWs,
  close: closeWs,
} = useWebSocket(socketUrl, {
  autoReconnect: false,
  immediate: false,
  // 直接处理消息事件，支持二进制数据
  onMessage: (_ws, event) => {
    if (terminal.value && event.data) {
      terminal.value.write(new Uint8Array(event.data))
    }
  },
  onConnected: (_ws) => {
    connecting.value = false
    connectionError.value = ''
    // 设置二进制类型为 arraybuffer
    if (_ws) {
      _ws.binaryType = 'arraybuffer'
    }
  },
  onDisconnected: () => {
    connecting.value = false
    if (isTerminalReady.value && !connectionError.value) {
      message.warning('终端连接已断开')
    }
  },
  onError: () => {
    connecting.value = false
    connectionError.value = 'WebSocket 连接失败'
    message.error('无法连接到终端')
  },
})

// 终端准备就绪
const handleTermReady = (term: Terminal) => {
  terminal.value = term
  isTerminalReady.value = true

  // 显示欢迎消息
  term.writeln('\x1b[1;32m=== 欢迎使用 Web Terminal ===\x1b[0m')
  term.writeln('\x1b[90m正在连接到远程 Shell...\x1b[0m')
  term.writeln('')

  // 连接 WebSocket
  connecting.value = true
  openWs()
}

// 处理用户输入
const handleTermData = (data: string) => {
  if (status.value === 'OPEN') {
    send(data)
  }
}

// 处理终端大小变化
const handleTermResize = (size: { cols: number; rows: number }) => {
  // 如果需要通知后端终端大小变化，可以在这里发送消息
  console.log('Terminal resized:', size)
}

// 重新连接
const reconnect = () => {
  if (connecting.value) {
    return
  }
  connecting.value = true
  connectionError.value = ''

  // 关闭现有连接
  closeWs()

  // 清空终端
  if (termRef.value) {
    termRef.value.clear()
  }

  // 重新连接
  if (terminal.value) {
    terminal.value.writeln('\x1b[1;32m=== 重新连接... ===\x1b[0m')
    terminal.value.writeln('')
  }

  setTimeout(() => {
    openWs()
  }, 100)
}

// 清空终端
const clearTerminal = () => {
  if (termRef.value) {
    termRef.value.clear()
  }
}

// 组件卸载时清理
onUnmounted(() => {
  closeWs()
})
</script>
<style lang="less">
.layout-terminal {
  .n-layout-scroll-container {
    overflow: hidden;
  }
}
</style>
<style scoped lang="less">
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-direction: row;
  height: 100%;
  .terminal-title {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-color);
    font-size: 14px;
    font-weight: 500;
  }
}

.terminal-page {
  display: flex;
  flex-direction: column;
}

.terminal-container {
  flex: 1;
  position: relative;
  overflow: hidden;
}

.terminal-actions {
  display: flex;
  gap: 8px;
}

.terminal-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 16px;
  font-size: 14px;
}
</style>

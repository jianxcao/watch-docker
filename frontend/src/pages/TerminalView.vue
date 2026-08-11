<template>
  <div class="terminal-page">
    <div class="terminal-container">
      <div v-show="connectionError" class="terminal-error">
        <n-icon size="48" :color="theme.errorColor">
          <CloseCircleOutline />
        </n-icon>
        <n-text>{{ connectionError }}</n-text>
        <n-button type="primary" size="small" @click="reconnect" :loading="connecting">
          重新连接
        </n-button>
      </div>
      <TermView
        v-show="!connectionError"
        ref="termRef"
        :config="termConfig"
        :height="termHeight"
        @ready="handleTermReady"
        @data="handleTermData"
        @resize="handleTermResize"
      />
    </div>

    <MobileKeyboard
      v-if="!connectionError"
      v-model:expanded="keyboardExpanded"
      @key="handleMobileKey"
      @clear="clearTerminal"
      @height-change="handleKeyboardHeightChange"
    />

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
          <n-tag
            v-if="terminalTarget"
            size="small"
            :type="terminalTarget === 'host' ? 'warning' : 'default'"
          >
            {{ terminalTargetLabel }}
          </n-tag>
          <n-select
            v-model:value="selectedShell"
            class="shell-select"
            size="tiny"
            :options="shellOptions"
            :loading="shellOptionsLoading"
            :disabled="connecting || shellOptionsLoading"
            @update:value="handleShellChange"
          />
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
import { shallowRef, computed, onMounted, onUnmounted } from 'vue'
import { useWebSocket } from '@vueuse/core'
import { useMessage, useThemeVars } from 'naive-ui'
import { RefreshOutline, CloseCircleOutline, TerminalOutline } from '@vicons/ionicons5'
import TextClearIcon from '@/assets/svg/textClear.svg?component'
import TermView from '@/components/Term/TermView.vue'
import MobileKeyboard from '@/components/Term/MobileKeyboard.vue'
import type { Terminal } from '@xterm/xterm'
import { useSettingStore } from '@/store/setting'
import { isMobile, isTablet } from '@/common/utils'
import { shellApi } from '@/common/api'
import type { ShellOption } from '@/common/types'
import {
  buildTerminalSocketUrl,
  rememberShell,
  selectInitialShell,
} from '@/utils/terminal'

const theme = useThemeVars()
const message = useMessage()
const settingStore = useSettingStore()
const termRef = shallowRef<InstanceType<typeof TermView>>()
const terminal = shallowRef<Terminal>()
const connectionError = shallowRef('')
const connecting = shallowRef(false)
const isTerminalReady = shallowRef(false)
const isMobileDevice = shallowRef(false)
const keyboardExpanded = shallowRef(false)
const keyboardHeight = shallowRef(0)
const shellOptions = shallowRef<ShellOption[]>([])
const selectedShell = shallowRef('')
const terminalTarget = shallowRef<'local' | 'host' | ''>('')
const shellOptionsLoading = shallowRef(false)
let shellOptionsRequest: Promise<void> | undefined
let intentionalDisconnects = 0

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
  const baseHeight = settingStore.contentSafeTop + settingStore.contentSafeBottom
  const panelHeight = keyboardExpanded.value ? keyboardHeight.value : 0
  return `calc(100vh - ${baseHeight + panelHeight}px)`
})

const terminalTargetLabel = computed(() => {
  if (terminalTarget.value === 'host') {
    return 'Docker 宿主机'
  }
  if (terminalTarget.value === 'local') {
    return '当前运行环境'
  }
  return ''
})

const socketUrl = computed(() => {
  const token = settingStore.getToken()
  return buildTerminalSocketUrl(window.location, selectedShell.value, token)
})

const loadShellOptions = async () => {
  shellOptionsLoading.value = true
  try {
    const response = await shellApi.getOptions()
    if (response.code !== 0) {
      throw new Error(response.msg || '无法读取 Shell 配置')
    }
    terminalTarget.value = response.data.target
    shellOptions.value = response.data.shells
    selectedShell.value = selectInitialShell(
      response.data.target,
      response.data.shells,
      response.data.defaultShell,
      () => window.localStorage,
    )
    if (!selectedShell.value) {
      throw new Error('当前终端目标没有可用的 Shell')
    }
  } finally {
    shellOptionsLoading.value = false
  }
}

const ensureShellOptions = async () => {
  if (selectedShell.value) {
    return
  }
  if (!shellOptionsRequest) {
    shellOptionsRequest = loadShellOptions().catch((error) => {
      shellOptionsRequest = undefined
      throw error
    })
  }
  await shellOptionsRequest
}

const openTerminalConnection = async () => {
  connecting.value = true
  connectionError.value = ''
  try {
    await ensureShellOptions()
    openWs()
  } catch (error: any) {
    connecting.value = false
    connectionError.value = error.message || '无法读取 Shell 配置'
    message.error(connectionError.value)
  }
}

// 使用 VueUse 的 useWebSocket
const {
  status,
  send,
  open: openWs,
  close: closeWs,
} = useWebSocket(socketUrl, {
  autoReconnect: false,
  autoConnect: false,
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
    if (intentionalDisconnects > 0) {
      intentionalDisconnects -= 1
      return
    }
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

  void openTerminalConnection()
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
const reconnect = async () => {
  if (connecting.value) {
    return
  }
  connecting.value = true
  connectionError.value = ''

  // 关闭现有连接
  if (status.value !== 'CLOSED') {
    intentionalDisconnects += 1
  }
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

  await new Promise((resolve) => setTimeout(resolve, 100))
  await openTerminalConnection()
}

const handleShellChange = (shell: string) => {
  if (terminalTarget.value) {
    rememberShell(terminalTarget.value, shell, shellOptions.value, () => window.localStorage)
  }
  if (isTerminalReady.value) {
    void reconnect()
  }
}

// 清空终端
const clearTerminal = () => {
  if (termRef.value) {
    termRef.value.clear()
  }
}

// 处理移动端键盘按键
const handleMobileKey = (sequence: string) => {
  if (status.value === 'OPEN') {
    send(sequence)
  }
  terminal.value?.focus()
}

const handleKeyboardHeightChange = (height: number) => {
  keyboardHeight.value = height
}

// 组件挂载时检测移动设备
onMounted(() => {
  isMobileDevice.value = isMobile() || isTablet()
  keyboardExpanded.value = isMobileDevice.value
  void ensureShellOptions().catch((error: any) => {
    connectionError.value = error.message || '无法读取 Shell 配置'
  })
})

// 组件卸载时清理
onUnmounted(() => {
  if (status.value !== 'CLOSED') {
    intentionalDisconnects += 1
  }
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
  height: calc(100vh - var(--content-safe-top) - var(--content-safe-bottom));
  min-height: calc(100vh - var(--content-safe-top) - var(--content-safe-bottom));
}

.terminal-container {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
}

.terminal-actions {
  display: flex;
  align-items: center;
  gap: 8px;

  .shell-select {
    width: 130px;
  }
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

<template>
  <n-modal
    v-model:show="modalVisible"
    title="创建日志"
    size="huge"
    display-directive="if"
    preset="card"
    class="compose-create-progress-modal"
    :mask-closable="false"
    :closable="canClose"
    :close-on-esc="canClose"
    style="width: 90vw; max-width: 1000px; padding: 12px"
    :on-after-leave="handleModalClose"
  >
    <template #header-extra>
      <n-space align="center" :size="12">
        <n-tag v-if="status === 'connecting'" type="info" :bordered="false">
          <template #icon>
            <n-icon :component="CloudOutline" />
          </template>
          连接中...
        </n-tag>
        <n-tag v-else-if="status === 'creating'" type="warning" :bordered="false">
          <template #icon>
            <n-icon :component="HourglassOutline" />
          </template>
          创建中...
        </n-tag>
        <n-tag v-else-if="status === 'success'" type="success" :bordered="false">
          <template #icon>
            <n-icon :component="CheckmarkCircleOutline" />
          </template>
          创建成功
        </n-tag>
        <n-tag v-else-if="status === 'error'" type="error" :bordered="false">
          <template #icon>
            <n-icon :component="CloseCircleOutline" />
          </template>
          创建失败
        </n-tag>
      </n-space>
    </template>

    <!-- Terminal 日志显示 -->
    <div class="terminal-container">
      <TermView ref="termRef" height="500px" :config="termConfig" @ready="handleTermReady" />
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button
          v-if="canClose"
          @click="handleClose"
          :type="status === 'error' ? 'default' : 'primary'"
        >
          关闭
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import { useWebSocket } from '@vueuse/core'
import { useMessage } from 'naive-ui'
import {
  CloudOutline,
  HourglassOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
} from '@vicons/ionicons5'
import { useSettingStore } from '@/store/setting'
import TermView from '@/components/Term/TermView.vue'
import type { TermConfig } from '@/components/Term/TermView.vue'

interface Props {
  projectName?: string
  yamlContent?: string
  force?: boolean // 是否强制覆盖已存在的项目
}

interface Emits {
  (e: 'success', composeFile: string): void
  (e: 'error', message: string): void
  (e: 'complete'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const message = useMessage()
const settingStore = useSettingStore()
const termRef = ref<InstanceType<typeof TermView>>()

// 弹窗可见性
const modalVisible = defineModel<boolean>('show')

// 状态
const status = ref<'idle' | 'connecting' | 'creating' | 'success' | 'error'>('idle')

// 是否可以关闭弹窗（创建中不允许关闭）
const canClose = computed(() => {
  return status.value === 'success' || status.value === 'error' || status.value === 'idle'
})

// Terminal 配置（禁用输入，仅用于日志显示）
const termConfig: TermConfig = {
  disableStdin: navigator.maxTouchPoints > 0,
  scrollback: 1000,
  cursorBlink: false,
}

// WebSocket URL
const wsUrl = computed(() => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const token = settingStore.getToken()
  let url = `${protocol}//${host}/api/v1/compose/create-and-up/ws`

  if (token) {
    url += `?token=${encodeURIComponent(token)}`
  }

  return url
})

// 终端就绪与消息缓冲（避免 @ready 前日志丢失）
const isTermReady = ref(false)
const pendingChunks: Array<string | Uint8Array> = []

const appendTermChunk = (chunk: string | Uint8Array) => {
  if (!isTermReady.value || !termRef.value) {
    pendingChunks.push(chunk)
    return
  }
  termRef.value.write(chunk)
  termRef.value.scrollToBottom()
}

const handleTermReady = () => {
  isTermReady.value = true
  while (pendingChunks.length > 0 && termRef.value) {
    const chunk = pendingChunks.shift()
    if (chunk !== undefined) {
      termRef.value.write(chunk)
    }
  }
  termRef.value?.scrollToBottom()
}

const handleBinaryFrame = (data: Uint8Array) => {
  // 检查是否是状态消息（以 \x00JSON: 开头）
  if (
    data.length > 6 &&
    data[0] === 0 &&
    String.fromCharCode(...data.slice(1, 6)) === 'JSON:'
  ) {
    try {
      const jsonStr = new TextDecoder().decode(data.slice(6))
      const statusMsg = JSON.parse(jsonStr)
      console.debug('收到状态消息:', statusMsg)

      if (statusMsg.status === 'success') {
        status.value = 'success'
        emit('success', statusMsg.message)
        emit('complete')
      } else if (statusMsg.status === 'error') {
        status.value = 'error'
        emit('error', statusMsg.message)
      }
    } catch (e) {
      console.error('Failed to parse status message:', e)
    }
  } else {
    appendTermChunk(data)
  }
}

// 使用 VueUse 的 useWebSocket（保持响应式 wsUrl，禁用自动连接与无响应应用层心跳）
const { send, open, close } = useWebSocket(wsUrl, {
  immediate: false,
  autoConnect: false,
  autoReconnect: false,
  heartbeat: false,
  onMessage: (_ws, event) => {
    if (!event.data) {
      return
    }
    if (event.data instanceof ArrayBuffer) {
      handleBinaryFrame(new Uint8Array(event.data))
    } else if (event.data instanceof Blob) {
      event.data.arrayBuffer().then((buffer) => {
        handleBinaryFrame(new Uint8Array(buffer))
      })
    } else if (typeof event.data === 'string') {
      appendTermChunk(event.data)
    }
  },
  onConnected: (_ws) => {
    if (status.value !== 'connecting') {
      return
    }
    status.value = 'creating'
    writeLine('\x1b[32m=== 连接成功，开始创建项目 ===\x1b[0m\r\n')
    if (_ws) {
      _ws.binaryType = 'arraybuffer'
    }
    // 发送创建请求
    if (props.projectName && props.yamlContent) {
      send(
        JSON.stringify({
          name: props.projectName,
          yamlContent: props.yamlContent,
          force: props.force || false, // 默认不强制覆盖，由调用方控制
        }),
      )
    }
  },
  onError: () => {
    status.value = 'error'
    writeLine('\x1b[31m✗ WebSocket 连接错误\x1b[0m\r\n')
    message.error('连接失败')
    emit('error', 'WebSocket 连接错误')
  },
  onDisconnected: () => {
    if (status.value === 'creating') {
      status.value = 'error'
      writeLine('\x1b[31m✗ 连接异常断开\x1b[0m\r\n')
      emit('error', '连接异常断开')
    }
  },
})

// 写入日志到 Terminal
const writeLine = (text: string | Uint8Array) => {
  nextTick(() => {
    appendTermChunk(text)
  })
}

// 开始创建
const start = () => {
  if (!props.projectName || !props.yamlContent) {
    message.error('项目名称和配置不能为空')
    return
  }

  pendingChunks.length = 0
  status.value = 'connecting'
  modalVisible.value = true
  termRef.value?.clear()
  writeLine('\x1b[36mℹ 正在连接服务器...\x1b[0m\r\n')
  open()
}

// 重置
const reset = () => {
  status.value = 'idle'
  isTermReady.value = false
  pendingChunks.length = 0
  modalVisible.value = false
  termRef.value?.clear()
  close()
}

// 关闭弹窗
const handleClose = () => {
  modalVisible.value = false
}

// 弹窗关闭后的回调
const handleModalClose = () => {
  isTermReady.value = false
  // 弹窗关闭后清理
  if (status.value === 'success' || status.value === 'error') {
    reset()
  }
}

// 暴露方法
defineExpose({
  start,
  reset,
  status,
})
</script>

<style scoped lang="less">
.terminal-container {
  padding: 0;
  overflow: hidden;
  border-radius: 4px;
}

.compose-create-progress-modal {
  :deep(.n-card__content) {
    padding: 0;
    overflow: hidden;
  }
}
</style>

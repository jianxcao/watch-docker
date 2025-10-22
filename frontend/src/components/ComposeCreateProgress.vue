<template>
  <n-modal
    v-model:show="modalVisible"
    title="创建日志"
    size="huge"
    display-directive="if"
    preset="dialog"
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
      <TermView ref="termRef" height="500px" :config="termConfig" />
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
import { ref, computed, watch, nextTick } from 'vue'
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

// 使用 VueUse 的 useWebSocket
const {
  data: wsData,
  send,
  open,
  close,
} = useWebSocket(wsUrl.value, {
  immediate: false,
  autoReconnect: false,
  heartbeat: {
    message: 'ping',
    interval: 30000,
  },
  onConnected: () => {
    status.value = 'creating'
    writeLine('\x1b[32m=== 连接成功，开始创建项目 ===\x1b[0m\r\n')

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
    termRef.value?.write(text)
    termRef.value?.scrollToBottom()
  })
}

// 监听 WebSocket 消息
watch(wsData, (data) => {
  if (!data) {
    return
  }

  try {
    const message = JSON.parse(data)
    const { type, message: msg } = message

    switch (type) {
      case 'INFO':
        writeLine(`\x1b[36mℹ ${msg}\x1b[0m`)
        break
      case 'SUCCESS':
        writeLine(`\x1b[32m✓ ${msg}\x1b[0m`)
        break
      case 'ERROR':
        status.value = 'error'
        writeLine(`\x1b[31m✗ ${msg}\x1b[0m`)
        emit('error', msg)
        break
      case 'LOG':
        writeLine(msg)
        break
      case 'COMPLETE':
        status.value = 'success'
        writeLine('\r\n\x1b[32m✓ 项目创建并启动成功！\x1b[0m\r\n')
        emit('success', msg)
        emit('complete')
        // 关闭 WebSocket 连接
        setTimeout(() => {
          close()
        }, 1000)
        break
    }
  } catch (error) {
    console.error('解析 WebSocket 消息失败:', error)
  }
})

// 开始创建
const start = () => {
  if (!props.projectName || !props.yamlContent) {
    message.error('项目名称和配置不能为空')
    return
  }

  status.value = 'connecting'
  modalVisible.value = true
  termRef.value?.clear()
  writeLine('\x1b[36mℹ 正在连接服务器...\x1b[0m\r\n')
  open()
}

// 重置
const reset = () => {
  status.value = 'idle'
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

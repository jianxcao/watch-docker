<template>
  <div>
    <div class="log-container" ref="containerRef" :style="{ height: containerHeight }">
      <n-virtual-list
        ref="virtualListRef"
        class="virtual-log-list"
        :items="logs"
        :item-size="estimatedItemSize"
        key-field="_k"
        item-resizable
      >
        <template #default="{ item: log }">
          <div class="log-line">
            <div class="log-line-left">
              <n-tag class="level-chip" size="small" :type="levelColor(log.level)" label>{{
                log.level
              }}</n-tag>
              <span class="timestamp">{{ log.time }}</span>
            </div>
            <span class="message">
              <span>{{ log.msg }}</span>
              <span v-if="getExtraFields(log)">{{ getExtraFields(log) }}</span>
            </span>
          </div>
        </template>
      </n-virtual-list>
    </div>

    <Teleport to="#header" defer>
      <div class="h-full flex items-center justify-between w-full pr-2">
        <n-h2 class="m-0 text-lg">日志</n-h2>
        <n-tag v-if="connectionError" type="error" size="small">{{ connectionError }}</n-tag>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useSettingStore } from '@/store/setting'
import { useAuthStore } from '@/store/auth'

const message = useMessage()
const settingStore = useSettingStore()
const authStore = useAuthStore()
const token = computed(() => settingStore.getToken())

interface LogEntry {
  level: string
  time: string
  msg: string
  _k: number
  [key: string]: any
}

const logs = ref<LogEntry[]>([])
const connectionError = ref('')
const virtualListRef = ref()
const containerRef = ref<HTMLDivElement | null>(null)
const containerHeight = computed(
  () => `calc(100vh - ${settingStore.contentSafeTop + settingStore.contentSafeBottom}px)`,
)

// 估算的每个日志项高度（像素）
const estimatedItemSize = 85

let eventSource: EventSource | null = null
let nextId = 1

function getExtraFields(log: LogEntry): Record<string, any> | null {
  const extra: Record<string, any> = {}
  let hasExtra = false
  for (const [k, v] of Object.entries(log)) {
    if (k !== 'level' && k !== 'time' && k !== 'msg' && k !== '_k') {
      extra[k] = v
      hasExtra = true
    }
  }
  return hasExtra ? extra : null
}

const scrollToBottom = async () => {
  await nextTick()
  if (!isNearBottom()) {
    return
  }
  if (virtualListRef.value && logs.value.length > 0) {
    virtualListRef.value.scrollTo({ index: logs.value.length - 1, behavior: 'auto' })
  }
}

const isNearBottom = (): boolean => {
  const containerEl = containerRef.value?.querySelector('.n-vl') as HTMLElement | null
  if (!containerEl) {
    return true
  }
  const distance = containerEl.scrollHeight - containerEl.scrollTop - containerEl.clientHeight
  return distance < 80
}

function levelColor(level: string): 'default' | 'primary' | 'info' | 'success' | 'warning' | 'error' {
  switch ((level || '').toUpperCase()) {
    case 'DEBUG':
      return 'primary'
    case 'INFO':
      return 'info'
    case 'WARN':
    case 'WARNING':
      return 'warning'
    case 'ERROR':
    case 'FATAL':
    case 'PANIC':
      return 'error'
    default:
      return 'default'
  }
}

const connectEventSource = () => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  if (authStore.authEnabled && !token.value) {
    return
  }
  const url = token.value
    ? `/api/v1/logs?token=${encodeURIComponent(token.value)}`
    : '/api/v1/logs'
  const es = new EventSource(url)
  eventSource = es

  es.onopen = () => {
    connectionError.value = ''
  }
  es.onmessage = (e) => {
    try {
      const logArray = JSON.parse(e.data)
      if (!Array.isArray(logArray)) {
        return
      }
      logArray.forEach((logEntry: any) => {
        logEntry.time = new Date(logEntry.time).toLocaleString()
        logEntry._k = nextId++
      })

      const shouldScroll = isNearBottom()
      const newLogs = [...logs.value, ...logArray]
      if (newLogs.length > 500) {
        newLogs.splice(0, newLogs.length - 500)
      }
      logs.value = newLogs
      if (shouldScroll) {
        void scrollToBottom()
      }
    } catch (err) {
      console.error('Failed to parse log SSE message:', err)
    }
  }
  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      connectionError.value = '日志流连接已关闭'
      if (authStore.authEnabled) {
        const headers: Record<string, string> = token.value
          ? { Authorization: `Bearer ${token.value}` }
          : {}
        void fetch('/api/v1/config', { headers })
          .then((res) => {
            if (res.status === 401 || res.status === 403) {
              authStore.forceLogout()
            }
          })
          .catch(() => {})
      }
      return
    }
    if (!connectionError.value) {
      connectionError.value = '日志流连接中断，正在重试...'
      message.warning('日志流连接中断')
    }
  }
}

watch(token, (newToken, oldToken) => {
  if (newToken !== oldToken) {
    connectEventSource()
  }
})

onMounted(() => {
  connectEventSource()
})

onBeforeUnmount(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})
</script>
<style lang="less">
.layout-logs {
  .n-layout-scroll-container {
    overflow: hidden;
  }
}
</style>
<style scoped lang="less">
@import '@/styles/mix.less';

.header {
  padding: 16px;
  font-size: 1.2rem;
  font-weight: 500;
}

.log-container {
  overflow: hidden;
}

.virtual-log-list {
  height: 100%;
}

.log-line {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding-block: 12px;
  font-size: 0.9rem;
  line-height: 1.4;
  border-bottom: 1px solid var(--border-color);

  .level-chip {
    width: 70px;
    justify-content: center;
    width: 70px;
    flex: 0 0 70px;
  }

  .timestamp {
    color: gray;
    margin: 0 4px;
    flex: 0 0 150px;
  }

  .message {
    word-break: break-all;
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
  }
}
</style>

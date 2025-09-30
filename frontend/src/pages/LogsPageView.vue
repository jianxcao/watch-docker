<template>
  <div>
    <div class="log-container" ref="containerRef">
      <div v-for="log in logs" :key="log._k" class="log-line">
        <div class="log-line-left">
          <n-tag class="level-chip" size="small" :type="levelColor(log.level)" label>{{ log.level }}</n-tag>
          <span class="timestamp">{{ log.time }}</span>
        </div>
        <span class="message">
          <span>{{ log.msg }}</span>
          <span>{{ _.omit(log, ['level', 'time', 'msg', '_k']) }}</span>
        </span>
      </div>
    </div>

    <Teleport to="#header" defer>
      <div class="h-full flex items-center">
        <n-h2 class="m-0 text-lg">日志</n-h2>
      </div>
    </Teleport>
  </div>


</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import * as _ from 'lodash-es'
import { useSettingStore } from '@/store/setting'

const token = computed(() => useSettingStore().getToken())


interface LogEntry {
  level: string
  time: string
  msg: string
  [key: string]: any
}

const logs = ref<LogEntry[]>([])
let eventSource: EventSource | null = null
const containerRef = ref<HTMLDivElement | null>(null)

let nextId = 1



function levelColor(level: string) {
  switch (level) {
    case 'DEBUG':
      return 'indigo'
    case 'INFO':
      return 'info'
    case 'WARN':
      return 'warning'
    case 'ERROR':
      return 'error'
    default:
      return ''
  }
}

onMounted(() => {
  eventSource = new EventSource(`/api/v1/logs?token=${token.value}`)
  eventSource.onmessage = (e) => {
    // 统一处理数组格式的消息
    const logArray = JSON.parse(e.data)

    // 处理数组中的每条日志
    logArray.forEach((logEntry: any) => {
      logEntry.time = new Date(logEntry.time).toLocaleString()
      logEntry._k = nextId++
    })

    const newLogs = [...logs.value, ...logArray]
    // 控制日志数量上限
    if (newLogs.length > 500) {
      const removeCount = newLogs.length - 500
      newLogs.splice(0, removeCount)
    }
    // 直接添加所有日志
    logs.value = newLogs
  }
})

onBeforeUnmount(() => {
  if (eventSource) {
    eventSource.close()
  }
})
</script>

<style scoped lang="less">
@import '@/styles/mix.less';

.header {
  padding: 16px;
  font-size: 1.2rem;
  font-weight: 500;
}

.log-container {
  display: flex;
  flex-direction: column-reverse;
  overflow-y: auto;
  overflow-x: hidden;
  .scrollbar();
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

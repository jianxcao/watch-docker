<template>
  <div>
    <div class="log-container" ref="containerRef" :style="{ height: containerHeight }">
      <n-virtual-list
        ref="virtualListRef"
        class="virtual-log-list"
        :items="reversedLogs"
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
              <span v-if="Object.keys(_.omit(log, ['level', 'time', 'msg', '_k'])).length > 0">{{
                _.omit(log, ['level', 'time', 'msg', '_k'])
              }}</span>
            </span>
          </div>
        </template>
      </n-virtual-list>
    </div>

    <Teleport to="#header" defer>
      <div class="h-full flex items-center">
        <n-h2 class="m-0 text-lg">日志</n-h2>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import * as _ from 'lodash-es'
import { useSettingStore } from '@/store/setting'

const settingStore = useSettingStore()
const token = computed(() => settingStore.getToken())

interface LogEntry {
  level: string
  time: string
  msg: string
  _k: number
  [key: string]: any
}

const logs = ref<LogEntry[]>([])
const virtualListRef = ref()
const containerRef = ref<HTMLDivElement | null>(null)
const containerHeight = computed(
  () => `calc(100vh - ${settingStore.contentSafeTop + settingStore.contentSafeBottom}px)`,
)

// 估算的每个日志项高度（像素）
const estimatedItemSize = 85

// 反转日志顺序，使新日志显示在底部
const reversedLogs = computed(() => [...logs.value].reverse())

let eventSource: EventSource | null = null
let nextId = 1

// // 自动滚动到底部的函数
// const scrollToBottom = async () => {
//   await nextTick()
//   if (virtualListRef.value && logs.value.length > 0) {
//     // 滚动到最后一项（reversedLogs中的最后一项对应原logs的第一项）
//     virtualListRef.value.scrollTo({ index: reversedLogs.value.length - 1, behavior: 'smooth' })
//   }
// }

// // 监听日志变化，自动滚动到底部
// watch(logs, () => {
//   scrollToBottom()
// }, { deep: true })

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

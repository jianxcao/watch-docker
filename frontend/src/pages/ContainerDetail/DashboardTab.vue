<template>
  <div class="tab-content dashboard-tab">
    <div v-if="!isRunning" class="empty-container">
      <n-empty description="容器未运行，无法获取资源使用数据" />
    </div>
    <div v-else-if="loading" class="loading-container">
      <n-spin size="large" />
      <div style="margin-top: 16px">正在加载资源数据...</div>
    </div>
    <div v-else-if="error" class="error-container">
      <n-result status="error" title="连接失败" :description="error">
        <template #footer>
          <n-button @click="reconnect">重新连接</n-button>
        </template>
      </n-result>
    </div>
    <div v-else-if="detailStats" class="dashboard-content">
      <!-- 顶部资源概览 -->
      <n-card title="资源仪表盘" class="overview-card">
        <template #header-extra>
          <n-text depth="3">实时性能指标和资源利用情况</n-text>
        </template>
        <div class="stats-overview">
          <div class="overview-item">
            <div class="overview-label">CPU使用率</div>
            <div class="overview-value">{{ cpuPercent.toFixed(2) }}%</div>
            <n-progress
              type="line"
              :percentage="cpuPercent"
              :show-indicator="false"
              :color="cpuPercent > 80 ? '#f5222d' : cpuPercent > 60 ? '#fa8c16' : '#52c41a'"
            />
          </div>
          <div class="overview-item">
            <div class="overview-label">内存使用率</div>
            <div class="overview-value">
              {{ memoryPercent.toFixed(2) }}%
              <n-text depth="3" style="font-size: 12px; margin-left: 8px">
                {{ formatBytes(detailStats.memory_stats.usage) }} /
                {{ formatBytes(detailStats.memory_stats.limit) }}
              </n-text>
            </div>
            <n-progress
              type="line"
              :percentage="memoryPercent"
              :show-indicator="false"
              :color="memoryPercent > 80 ? '#f5222d' : memoryPercent > 60 ? '#fa8c16' : '#18a058'"
            />
          </div>
        </div>
      </n-card>

      <div class="stats-grid">
        <!-- CPU 详细信息 -->
        <n-card title="CPU 详细信息" class="stat-card">
          <div class="detail-section">
            <div class="detail-item">
              <span class="detail-label">CPU 限制:</span>
              <span class="detail-value">{{ detailStats.cpu_stats.online_cpus }} CPU</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">系统:</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.system_cpu_usage / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">用户模式:</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.cpu_usage.usage_in_usermode / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">内核模式:</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.cpu_usage.usage_in_kernelmode / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">CPU限流周期:</span>
              <span class="detail-value">{{ detailStats.cpu_stats.throttling_data.periods }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">CPU限流时间:</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.throttling_data.throttled_time / 1000000000)
              }}</span>
            </div>
          </div>
        </n-card>

        <!-- 内存详细信息 -->
        <n-card title="内存详细信息" class="stat-card">
          <div class="detail-section">
            <div class="detail-item">
              <span class="detail-label">使用情况:</span>
              <span class="detail-value">{{ formatBytes(detailStats.memory_stats.usage) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">限制:</span>
              <span class="detail-value">{{ formatBytes(detailStats.memory_stats.limit) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">缓存:</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.cache || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">活动文件:</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.active_file || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">非活动文件:</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.inactive_file || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">RSS:</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.anon || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">页面故障:</span>
              <span class="detail-value">{{
                formatNumber(detailStats.memory_stats.stats.pgfault || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">主要故障:</span>
              <span class="detail-value">{{
                formatNumber(detailStats.memory_stats.stats.pgmajfault || 0)
              }}</span>
            </div>
          </div>
        </n-card>

        <!-- 进程数量 -->
        <n-card title="进程数量" class="stat-card">
          <div class="large-stat">
            <div class="large-stat-value">{{ detailStats.pids_stats.current }}</div>
            <div class="large-stat-label">运行中</div>
          </div>
        </n-card>

        <!-- 网络 I/O -->
        <n-card title="网络 I/O" class="stat-card">
          <div class="detail-section">
            <div class="detail-item highlight">
              <span class="detail-label">已接收</span>
              <span class="detail-value">{{ formatBytes(totalNetworkRx) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label sub">数据包</span>
              <span class="detail-value">{{ formatNumber(totalNetworkRxPackets) }} 数据包</span>
            </div>
            <div class="detail-item highlight">
              <span class="detail-label">已传输</span>
              <span class="detail-value">{{ formatBytes(totalNetworkTx) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label sub">数据包</span>
              <span class="detail-value">{{ formatNumber(totalNetworkTxPackets) }} 数据包</span>
            </div>
          </div>
        </n-card>

        <!-- 块 I/O -->
        <n-card title="块 I/O" class="stat-card">
          <div class="detail-section">
            <div class="detail-item highlight">
              <span class="detail-label">读取</span>
              <span class="detail-value">{{ formatBytes(blockRead) }}</span>
            </div>
            <div class="detail-item highlight">
              <span class="detail-label">写入</span>
              <span class="detail-value">{{ formatBytes(blockWrite) }}</span>
            </div>
          </div>
        </n-card>
      </div>

      <!-- 网络接口 -->
      <n-card
        v-if="Object.keys(detailStats.networks).length > 0"
        title="网络接口"
        class="network-card"
      >
        <div class="network-interfaces">
          <div
            v-for="(network, name) in detailStats.networks"
            :key="name"
            class="network-interface"
          >
            <div class="interface-name">{{ name }}</div>
            <div class="interface-stats">
              <div class="interface-stat">
                <span class="stat-label">RX:</span>
                <span class="stat-value">{{ formatBytes(network.rx_bytes) }}</span>
              </div>
              <div class="interface-stat">
                <span class="stat-label">TX:</span>
                <span class="stat-value">{{ formatBytes(network.tx_bytes) }}</span>
              </div>
            </div>
          </div>
        </div>
      </n-card>
    </div>
    <div v-else-if="!isConnected" class="error-container">
      <n-result status="info" title="链接断开">
        <template #footer>
          <n-button @click="reconnect">重新连接</n-button>
        </template>
      </n-result>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ContainerDetailStats } from '@/common/types'
import { API_ENDPOINTS } from '@/constants/api'
import { useSettingStore } from '@/store/setting'
import { useWebSocket } from '@vueuse/core'
import { useMessage } from 'naive-ui'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

interface Props {
  isRunning: boolean
  containerId: string
}

const props = defineProps<Props>()
const message = useMessage()
const settingStore = useSettingStore()

const loading = ref(true)
const error = ref('')
const detailStats = ref<ContainerDetailStats | null>(null)

// 计算 WebSocket URL
const wsUrl = computed(() => {
  if (!props.isRunning) {
    return undefined
  }

  const token = settingStore.getToken()
  if (!token) {
    return undefined
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/api/v1${API_ENDPOINTS.CONTAINER_STATS_WS(props.containerId)}?token=${token}`
})

// 使用 VueUse 的 useWebSocket
const { status, open, close } = useWebSocket(wsUrl, {
  // 自动重连配置
  autoReconnect: {
    retries: 5,
    delay: 3000,
    onFailed() {
      console.error('容器统计 WebSocket 重连失败，已达到最大重试次数')
      error.value = '连接失败，已达到最大重试次数'
      message.error('无法连接到容器统计服务')
    },
  },
  // 禁用心跳（统计数据本身就是持续推送的）
  heartbeat: false,
  // 不立即连接，等容器运行时再连接
  immediate: false,
  // URL变化时自动重连
  autoConnect: false,
  // 页面卸载时自动关闭
  autoClose: true,

  // 连接成功回调
  onConnected(ws) {
    console.log('容器统计 WebSocket 连接已建立')
    loading.value = false
    error.value = ''
    // 设置二进制类型
    if (ws) {
      ws.binaryType = 'arraybuffer'
    }
  },

  // 连接断开回调
  onDisconnected(_, event) {
    console.log('容器统计 WebSocket 连接已断开:', event.code, event.reason)
    if (!error.value) {
      error.value = ''
    }
  },

  // 错误回调
  onError(_, event) {
    console.error('容器统计 WebSocket 错误:', event)
    error.value = 'WebSocket 连接错误'
    loading.value = false
    message.error('容器统计连接失败')
  },

  // 消息回调
  onMessage(_, event) {
    try {
      const data = JSON.parse(event.data)
      detailStats.value = data as ContainerDetailStats
      // 数据接收成功，清除错误
      if (error.value) {
        error.value = ''
      }
    } catch (err) {
      console.error('解析统计数据失败:', err)
    }
  },
})

// 连接状态
const isConnected = computed(() => status.value === 'OPEN')

// 启动连接
const connect = () => {
  if (isConnected.value || !props.isRunning) {
    return
  }
  if (wsUrl.value) {
    loading.value = true
    error.value = ''
    open()
  }
}

// 断开连接
const disconnect = () => {
  close()
  detailStats.value = null
}

// 重新连接
const reconnect = () => {
  if (isConnected.value) {
    disconnect()
  }
  setTimeout(() => {
    connect()
  }, 100)
}

// 计算 CPU 使用率
const cpuPercent = computed(() => {
  if (!detailStats.value) {
    return 0
  }

  const cpuDelta =
    detailStats.value.cpu_stats.cpu_usage.total_usage -
    detailStats.value.precpu_stats.cpu_usage.total_usage
  const systemDelta =
    detailStats.value.cpu_stats.system_cpu_usage - detailStats.value.precpu_stats.system_cpu_usage

  if (systemDelta > 0 && cpuDelta >= 0) {
    return (cpuDelta / systemDelta) * 100.0
  }

  return 0
})

// 计算内存使用率
const memoryPercent = computed(() => {
  if (!detailStats.value || !detailStats.value.memory_stats.limit) {
    return 0
  }

  const usage = detailStats.value.memory_stats.usage
  const limit = detailStats.value.memory_stats.limit

  // 减去缓存
  const cache =
    detailStats.value.memory_stats.stats.inactive_file ||
    detailStats.value.memory_stats.stats.total_cache ||
    detailStats.value.memory_stats.stats.cache ||
    0

  const actualUsage = usage > cache ? usage - cache : usage

  return (actualUsage / limit) * 100
})

// 计算网络总接收字节
const totalNetworkRx = computed(() => {
  if (!detailStats.value) {
    return 0
  }
  return Object.values(detailStats.value.networks).reduce((sum, net) => sum + net.rx_bytes, 0)
})

// 计算网络总接收包数
const totalNetworkRxPackets = computed(() => {
  if (!detailStats.value) {
    return 0
  }
  return Object.values(detailStats.value.networks).reduce((sum, net) => sum + net.rx_packets, 0)
})

// 计算网络总发送字节
const totalNetworkTx = computed(() => {
  if (!detailStats.value) {
    return 0
  }
  return Object.values(detailStats.value.networks).reduce((sum, net) => sum + net.tx_bytes, 0)
})

// 计算网络总发送包数
const totalNetworkTxPackets = computed(() => {
  if (!detailStats.value) {
    return 0
  }
  return Object.values(detailStats.value.networks).reduce((sum, net) => sum + net.tx_packets, 0)
})

// 计算块读取字节
const blockRead = computed(() => {
  if (!detailStats.value || !detailStats.value.blkio_stats.io_service_bytes_recursive) {
    return 0
  }
  return detailStats.value.blkio_stats.io_service_bytes_recursive
    .filter((item) => item.op === 'read' || item.op === 'Read')
    .reduce((sum, item) => sum + item.value, 0)
})

// 计算块写入字节
const blockWrite = computed(() => {
  if (!detailStats.value || !detailStats.value.blkio_stats.io_service_bytes_recursive) {
    return 0
  }
  return detailStats.value.blkio_stats.io_service_bytes_recursive
    .filter((item) => item.op === 'write' || item.op === 'Write')
    .reduce((sum, item) => sum + item.value, 0)
})

// 格式化字节
const formatBytes = (bytes: number) => {
  if (bytes === 0) {
    return '0 B'
  }
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

// 格式化数字
const formatNumber = (num: number) => {
  return num.toLocaleString()
}

// 格式化时长（秒）
const formatDuration = (seconds: number) => {
  if (seconds < 1) {
    return seconds.toFixed(2) + 's'
  }

  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  const parts = []
  if (days > 0) {
    parts.push(`${days}天`)
  }
  if (hours > 0) {
    parts.push(`${hours}小时`)
  }
  if (minutes > 0) {
    parts.push(`${minutes}分`)
  }
  if (secs > 0 || parts.length === 0) {
    parts.push(`${secs}秒`)
  }

  return parts.join(' ')
}

// 监听运行状态变化
watch(
  () => props.isRunning,
  (newVal) => {
    if (newVal) {
      connect()
    } else {
      disconnect()
    }
  },
  { immediate: false }, // 立即执行一次
)

onMounted(() => {
  connect()
})

onUnmounted(() => {
  disconnect()
})
</script>

<style scoped lang="less">
@import './styles.less';
</style>

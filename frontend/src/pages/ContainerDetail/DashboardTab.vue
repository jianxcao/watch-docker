<template>
  <div class="tab-content dashboard-tab">
    <div v-if="!isRunning" class="empty-container">
      <n-empty description="容器未运行，无法获取资源使用数据" />
    </div>
    <div v-else-if="loading" class="loading-container">
      <n-spin size="large" />
      <div style="mt-4">正在加载资源数据...</div>
    </div>
    <div v-else-if="error" class="error-container">
      <n-result status="error" title="连接失败" :description="error">
        <template #footer>
          <n-button @click="reconnect">重新连接</n-button>
        </template>
      </n-result>
    </div>
    <div v-else-if="detailStats" class="dashboard-content">
      <!-- 顶部资源概览卡片 -->
      <div class="overview-cards">
        <!-- CPU 使用率卡片 -->
        <div class="overview-card cpu-card">
          <div class="card-header">
            <div class="header-left">
              <div class="overview-icon">
                <CpuUsageIcon />
              </div>
              <div class="overview-title">CPU使用率</div>
            </div>
            <div class="overview-badge">实时</div>
          </div>
          <div class="card-content">
            <div class="overview-value">{{ cpuPercent.toFixed(2) }}%</div>
            <div class="overview-progress">
              <div class="progress-fill" :style="{ width: cpuPercent + '%' }"></div>
            </div>
          </div>
        </div>

        <!-- 内存使用率卡片 -->
        <div class="overview-card memory-card">
          <div class="card-header">
            <div class="header-left">
              <div class="overview-icon">
                <MemoryUsageIcon />
              </div>
              <div class="overview-title">内存使用率</div>
            </div>
            <div class="memory-info">
              {{ formatBytes(detailStats.memory_stats.usage) }} /
              {{ formatBytes(detailStats.memory_stats.limit) }}
            </div>
          </div>
          <div class="card-content">
            <div class="overview-value">{{ memoryPercent.toFixed(2) }}%</div>
            <div class="overview-progress">
              <div class="progress-fill" :style="{ width: memoryPercent + '%' }"></div>
            </div>
          </div>
        </div>
      </div>

      <div class="stats-grid">
        <!-- CPU 详细信息 -->
        <div class="stat-card">
          <div class="card-header">
            <CpuUsageIcon class="card-icon" />
            <div class="card-title">CPU 详细信息</div>
          </div>
          <div class="detail-section">
            <div class="detail-item">
              <span class="detail-label">CPU 型号</span>
              <span class="detail-value">{{ detailStats.cpu_stats.online_cpus }} CPU</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">系统</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.system_cpu_usage / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">用户模式</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.cpu_usage.usage_in_usermode / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">空闲模式</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.cpu_usage.usage_in_kernelmode / 1000000000)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">CPU线程数(中)</span>
              <span class="detail-value">{{ detailStats.cpu_stats.throttling_data.periods }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">CPU线程数(外)</span>
              <span class="detail-value">{{
                formatDuration(detailStats.cpu_stats.throttling_data.throttled_time / 1000000000)
              }}</span>
            </div>
          </div>
        </div>

        <!-- 内存详细信息 -->
        <div class="stat-card">
          <div class="card-header">
            <MemoryUsageIcon class="card-icon" />
            <div class="card-title">内存详细信息</div>
          </div>
          <div class="detail-section">
            <div class="detail-item">
              <span class="detail-label">使用中</span>
              <span class="detail-value">{{ formatBytes(detailStats.memory_stats.usage) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">空闲</span>
              <span class="detail-value">{{ formatBytes(detailStats.memory_stats.limit) }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">缓存</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.cache || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">活跃文件</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.active_file || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">非活跃文件</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.inactive_file || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">页进</span>
              <span class="detail-value">{{
                formatBytes(detailStats.memory_stats.stats.anon || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">主要缺页</span>
              <span class="detail-value">{{
                formatNumber(detailStats.memory_stats.stats.pgfault || 0)
              }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">次要缺页</span>
              <span class="detail-value">{{
                formatNumber(detailStats.memory_stats.stats.pgmajfault || 0)
              }}</span>
            </div>
          </div>
        </div>

        <!-- 连接数量 -->
        <div class="stat-card process-card">
          <div class="card-header">
            <ConnectionIcon class="card-icon" />
            <div class="card-title">连接数量</div>
          </div>
          <div class="large-stat">
            <div class="large-stat-value">{{ detailStats.pids_stats.current }}</div>
            <div class="large-stat-label">当前连接</div>
          </div>
        </div>

        <!-- 网络 I/O -->
        <div class="stat-card">
          <div class="card-header">
            <NetworkIOIcon class="card-icon" />
            <div class="card-title">网络 I/O</div>
          </div>
          <div class="detail-section">
            <div class="detail-item highlight">
              <span class="detail-label">已接受</span>
              <span class="detail-value">{{ formatBytes(totalNetworkRx) }}</span>
            </div>
            <div class="detail-item highlight">
              <span class="detail-label">数据包</span>
              <span class="detail-value">{{ formatNumber(totalNetworkRxPackets) }} </span>
            </div>
            <div class="detail-item highlight">
              <span class="detail-label">已传输</span>
              <span class="detail-value">{{ formatBytes(totalNetworkTx) }}</span>
            </div>
            <div class="detail-item highlight">
              <span class="detail-label">数据包</span>
              <span class="detail-value">{{ formatNumber(totalNetworkTxPackets) }} </span>
            </div>
          </div>
        </div>

        <!-- 磁盘 I/O -->
        <div class="stat-card">
          <div class="card-header">
            <DiskIOIcon class="card-icon" />
            <div class="card-title">磁盘 I/O</div>
          </div>
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
        </div>
      </div>

      <!-- 网络接口 -->
      <div v-if="Object.keys(detailStats.networks).length > 0" class="network-card">
        <div class="card-header">
          <NetworkInterfaceIcon class="card-icon" />
          <div class="card-title">网络接口</div>
        </div>
        <div class="network-interfaces">
          <div
            v-for="(network, name) in detailStats.networks"
            :key="name"
            class="network-interface"
          >
            <div class="interface-header">
              <div class="interface-badge">{{ name }}</div>
            </div>
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
      </div>
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

// 导入 SVG 图标
import CpuUsageIcon from '@/assets/svg/cpuUsage.svg?component'
import MemoryUsageIcon from '@/assets/svg/memoryUsage.svg?component'
import ConnectionIcon from '@/assets/svg/connection.svg?component'
import NetworkIOIcon from '@/assets/svg/networkIO.svg?component'
import DiskIOIcon from '@/assets/svg/diskIO.svg?component'
import NetworkInterfaceIcon from '@/assets/svg/networkInterface.svg?component'
import { formatBytes, formatDuration, formatNumber } from '@/common/utils'

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
@import './DashboardTab.less';
</style>

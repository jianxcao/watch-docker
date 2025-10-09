import type { ContainerStats, ContainerStatus } from '@/common/types'
import { useSettingStore } from '@/store/setting'
import { useWebSocket } from '@vueuse/core'
import { computed, ref } from 'vue'

export interface StatsMessage {
  type: string
  data: {
    stats?: Record<string, ContainerStats>
    containers?: ContainerStatus[]
  }
  timestamp: number
}

export type StatsCallback = (statsMap: Record<string, ContainerStats>) => void
export type ContainersCallback = (containers: ContainerStatus[]) => void

/**
 * 容器统计数据 WebSocket Composable
 * 基于 VueUse 的 useWebSocket 实现
 */
export function useStatsWebSocket() {
  const settingStore = useSettingStore()

  // 响应式状态
  const statsData = ref<Record<string, ContainerStats>>({})
  const containersData = ref<ContainerStatus[]>([])
  const containersCallbacks = ref<Set<ContainersCallback>>(new Set())

  // 计算 WebSocket URL
  const wsUrl = computed(() => {
    const token = settingStore.getToken()
    if (!token) {
      return undefined
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    return `${protocol}//${host}/api/v1/containers/stats/ws?token=${token}`
  })
  console.debug('wsUrl', wsUrl.value)
  // 使用 VueUse 的 useWebSocket
  const { status, send, open, close, ws } = useWebSocket(wsUrl, {
    // 自动重连配置
    autoReconnect: {
      retries: 5,
      delay: 2000,
      onFailed() {
        console.error('WebSocket 重连失败，已达到最大重试次数')
      },
    },
    // autoReconnect: false,
    // 心跳检测
    heartbeat: {
      message: 'ping',
      interval: 30000, // 30秒心跳
      pongTimeout: 5000, // 5秒等待pong响应
    },
    // 立即连接
    immediate: false,
    // URL变化时自动重连
    autoConnect: true,
    // 页面卸载时自动关闭
    autoClose: true,
    // 连接成功回调
    onConnected() {
      console.log('Stats WebSocket 连接已建立')
    },
    // 连接断开回调
    onDisconnected(_, event) {
      console.log('Stats WebSocket 连接已断开:', event.code, event.reason)
    },
    // 连接错误回调
    onError(_, error) {
      console.error('Stats WebSocket 连接错误:', error)
    },
    // 消息接收回调
    onMessage(_, event) {
      try {
        const message: StatsMessage = JSON.parse(event.data)

        if (message.type === 'containers' && message.data.containers) {
          // 更新容器数据
          containersData.value = message.data.containers

          // 通知所有容器回调函数
          containersCallbacks.value.forEach((callback) => {
            try {
              callback(message.data.containers!)
            } catch (error) {
              console.error('容器数据回调执行失败:', error)
            }
          })
        }
      } catch (error) {
        console.error('解析 WebSocket 消息失败:', error)
      }
    },
  })

  // 连接状态映射
  const connectionState = computed(() => {
    switch (status.value) {
      case 'CONNECTING':
        return 'connecting'
      case 'OPEN':
        return 'connected'
      case 'CLOSED':
        return 'disconnected'
      default:
        return 'disconnected'
    }
  })

  // 是否已连接
  const isConnected = computed(() => status.value === 'OPEN')

  // 添加容器数据回调
  const addContainersCallback = (callback: ContainersCallback) => {
    containersCallbacks.value.add(callback)
  }

  // 移除容器数据回调
  const removeContainersCallback = (callback: ContainersCallback) => {
    containersCallbacks.value.delete(callback)
  }

  // 启动连接
  const connect = () => {
    if (isConnected.value) {
      return
    }
    console.debug('connect')
    if (wsUrl.value) {
      open()
    } else {
      console.warn('无法启动 WebSocket 连接：缺少有效的 token')
    }
  }

  // 断开连接
  const disconnect = () => {
    containersCallbacks.value.clear()
    close()
  }

  // 重新连接
  const reconnect = () => {
    console.debug('reconnect')
    disconnect()
    setTimeout(() => {
      connect()
    }, 100)
  }

  return {
    // 状态
    status,
    connectionState,
    isConnected,
    statsData,
    containersData,
    ws,

    // 方法
    connect,
    disconnect,
    reconnect,
    addContainersCallback,
    removeContainersCallback,
    send,
  }
}

// 全局单例实例
let globalStatsWebSocket: ReturnType<typeof useStatsWebSocket> | null = null

/**
 * 获取全局 Stats WebSocket 实例
 */
export function getGlobalStatsWebSocket() {
  if (!globalStatsWebSocket) {
    globalStatsWebSocket = useStatsWebSocket()
  }
  return globalStatsWebSocket
}

/**
 * 销毁全局 Stats WebSocket 实例
 */
export function destroyGlobalStatsWebSocket() {
  if (globalStatsWebSocket) {
    globalStatsWebSocket.disconnect()
    globalStatsWebSocket = null
  }
}

import type { ContainerStats } from '@/common/types'
import { useSettingStore } from '@/store/setting'
import { useWebSocket } from '@vueuse/core'
import { computed, ref } from 'vue'

export interface StatsMessage {
  type: string
  data: {
    stats: Record<string, ContainerStats>
  }
  timestamp: number
}

export type StatsCallback = (statsMap: Record<string, ContainerStats>) => void

/**
 * 容器统计数据 WebSocket Composable
 * 基于 VueUse 的 useWebSocket 实现
 */
export function useStatsWebSocket() {
  const settingStore = useSettingStore()

  // 响应式状态
  const statsData = ref<Record<string, ContainerStats>>({})
  const callbacks = ref<Set<StatsCallback>>(new Set())

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
    // autoReconnect: {
    //   retries: 5,
    //   delay: 1000,
    //   onFailed() {
    //     console.error('WebSocket 重连失败，已达到最大重试次数')
    //   },
    // },
    autoReconnect: false,
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
        if (message.type === 'stats' && message.data.stats) {
          // 更新统计数据
          statsData.value = message.data.stats

          // 通知所有回调函数
          callbacks.value.forEach((callback) => {
            try {
              callback(message.data.stats)
            } catch (error) {
              console.error('统计数据回调执行失败:', error)
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

  // 添加统计数据回调
  const addStatsCallback = (callback: StatsCallback) => {
    callbacks.value.add(callback)
  }

  // 移除统计数据回调
  const removeStatsCallback = (callback: StatsCallback) => {
    callbacks.value.delete(callback)
  }

  // 启动连接
  const connect = () => {
    console.debug('connect')
    if (wsUrl.value) {
      open()
    } else {
      console.warn('无法启动 WebSocket 连接：缺少有效的 token')
    }
  }

  // 断开连接
  const disconnect = () => {
    callbacks.value.clear()
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
    ws,

    // 方法
    connect,
    disconnect,
    reconnect,
    addStatsCallback,
    removeStatsCallback,
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

import type { ContainerStats, ContainerStatus } from '@/common/types'
import { useSettingStore } from '@/store/setting'
import { useWebSocket } from '@vueuse/core'
import { computed } from 'vue'
import statsEmitter from '@/evt/containerStats'
export interface StatsMessage {
  type: string
  data: {
    stats?: Record<string, ContainerStats>
    containers?: ContainerStatus[]
  }
  timestamp: number
}

/**
 * 容器统计数据 WebSocket Composable
 * 基于 VueUse 的 useWebSocket 实现
 */
export default function useStatsWebSocket() {
  const settingStore = useSettingStore()

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
    // Safari 兼容性：禁用客户端心跳，依赖服务端的 Ping/Pong 机制
    // Safari 不太支持客户端主动发送的文本心跳消息
    heartbeat: false,
    // 立即连接
    immediate: false,
    // URL变化时自动重连
    autoConnect: true,
    // 页面卸载时自动关闭
    autoClose: true,
    // 连接成功回调
    onConnected(ws) {
      console.log('Stats WebSocket 连接已建立')
      // Safari 兼容性：设置二进制类型
      if (ws) {
        ws.binaryType = 'arraybuffer'
      }
    },
    // 连接断开回调
    onDisconnected(_, event) {
      console.log('Stats WebSocket 连接已断开:', event.code, event.reason)
      // 1006 表示异常关闭，通常是网络问题或服务端问题
      if (event.code === 1006) {
        console.warn('WebSocket 异常关闭 (1006)，可能是网络问题或服务端问题')
      }
    },
    // 连接错误回调
    onError(_, error) {
      console.error('Stats WebSocket 连接错误:', error)
    },
    // 消息接收回调
    onMessage(_, event) {
      try {
        let dataStr: string
        // 处理二进制消息
        if (event.data instanceof ArrayBuffer) {
          const decoder = new TextDecoder('utf-8')
          dataStr = decoder.decode(event.data)
        } else if (typeof event.data === 'string') {
          // 兼容文本消息
          dataStr = event.data
        } else {
          console.error('未知的消息类型:', typeof event.data)
          return
        }
        try {
          const message: StatsMessage = JSON.parse(dataStr)
          if (message.type === 'containers' && message.data.containers) {
            // console.debug('onMessage', message.data.containers)
            statsEmitter.emit('containers', message.data.containers)
          }
        } catch (error) {
          console.error('解析 JSON 消息失败:', error, 'Data:', dataStr)
        }
      } catch (error) {
        console.error('处理 WebSocket 消息失败:', error)
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
    close()
  }

  // 重新连接
  const reconnect = () => {
    console.debug('reconnect')
    if (status.value === 'OPEN') {
      disconnect()
    }
    setTimeout(() => {
      connect()
    }, 100)
  }

  return {
    // 状态
    status,
    connectionState,
    isConnected,
    ws,

    // 方法
    connect,
    disconnect,
    reconnect,
    send,
  }
}

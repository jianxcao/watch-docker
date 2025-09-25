import type { ContainerStats } from '@/common/types'
import type { BaseMessage } from './baseWebSocket'
import { BaseWebSocketService } from './baseWebSocket'

export interface StatsMessage extends BaseMessage {
  type: string
  data: {
    stats: Record<string, ContainerStats>
  }
  timestamp: number
}

export type StatsCallback = (statsMap: Record<string, ContainerStats>) => void

export class StatsWebSocketService extends BaseWebSocketService<StatsMessage> {
  private callbacks: Set<StatsCallback> = new Set()

  constructor(token: string) {
    super(token, {
      maxReconnectAttempts: 5,
      reconnectDelay: 1000,
    })
    this.start()
  }

  protected getWebSocketUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    return `${protocol}//${host}/api/v1/containers/stats/ws?token=${this.token}`
  }

  protected handleMessage(message: StatsMessage): void {
    if (message.type === 'stats' && message.data.stats) {
      // 通知所有回调函数
      this.callbacks.forEach((callback) => {
        try {
          callback(message.data.stats)
        } catch (error) {
          console.error('统计数据回调执行失败:', error)
        }
      })
    }
  }

  public addStatsCallback(callback: StatsCallback): void {
    this.callbacks.add(callback)
  }

  public removeStatsCallback(callback: StatsCallback): void {
    this.callbacks.delete(callback)
  }

  public override disconnect(): void {
    this.callbacks.clear()
    super.disconnect()
  }
}

// 创建单例实例
let statsWebSocketService: StatsWebSocketService | null = null

export function getStatsWebSocketService(token: string): StatsWebSocketService {
  if (!statsWebSocketService) {
    statsWebSocketService = new StatsWebSocketService(token)
  }
  return statsWebSocketService
}

export function destroyStatsWebSocketService(): void {
  if (statsWebSocketService) {
    statsWebSocketService.disconnect()
    statsWebSocketService = null
  }
}

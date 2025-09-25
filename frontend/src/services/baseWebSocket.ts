export interface WebSocketConfig {
  maxReconnectAttempts?: number
  reconnectDelay?: number
  protocols?: string | string[]
}

export interface BaseMessage {
  type: string
  timestamp?: number
}

export abstract class BaseWebSocketService<T extends BaseMessage> {
  protected ws: WebSocket | null = null
  protected reconnectAttempts = 0
  protected maxReconnectAttempts = 5
  protected reconnectDelay = 1000
  protected isConnecting = false
  protected shouldReconnect = true
  protected token: string
  protected protocols?: string | string[]

  constructor(token: string, config?: WebSocketConfig) {
    this.token = token
    if (config) {
      this.maxReconnectAttempts = config.maxReconnectAttempts ?? this.maxReconnectAttempts
      this.reconnectDelay = config.reconnectDelay ?? this.reconnectDelay
      this.protocols = config.protocols
    }
  }

  /**
   * 子类需要实现的抽象方法 - 获取 WebSocket URL
   */
  protected abstract getWebSocketUrl(): string

  /**
   * 子类需要实现的抽象方法 - 处理接收到的消息
   */
  protected abstract handleMessage(message: T): void

  /**
   * 子类可以覆盖的方法 - 连接成功时的处理
   */
  protected onConnected(): void {
    console.log('WebSocket 连接已建立')
  }

  /**
   * 子类可以覆盖的方法 - 连接关闭时的处理
   */
  protected onDisconnected(event: CloseEvent): void {
    console.log('WebSocket 连接已关闭:', event.code, event.reason)
  }

  /**
   * 子类可以覆盖的方法 - 连接错误时的处理
   */
  protected onError(error: Event): void {
    console.error('WebSocket 连接错误:', error)
  }

  /**
   * 建立 WebSocket 连接
   */
  protected connect(): void {
    if (this.isConnecting || this.ws?.readyState === WebSocket.OPEN) {
      return
    }

    this.isConnecting = true

    try {
      const url = this.getWebSocketUrl()
      console.log('正在连接 WebSocket:', url)

      this.ws = this.protocols ? new WebSocket(url, this.protocols) : new WebSocket(url)

      this.ws.onopen = () => {
        this.isConnecting = false
        this.reconnectAttempts = 0
        this.reconnectDelay = 1000
        this.onConnected()
      }

      this.ws.onmessage = (event) => {
        try {
          const message: T = JSON.parse(event.data)
          this.handleMessage(message)
        } catch (error) {
          console.error('解析 WebSocket 消息失败:', error)
        }
      }

      this.ws.onclose = (event) => {
        this.isConnecting = false
        this.ws = null
        this.onDisconnected(event)

        if (this.shouldReconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
          this.scheduleReconnect()
        }
      }

      this.ws.onerror = (error) => {
        this.isConnecting = false
        this.onError(error)
      }
    } catch (error) {
      console.error('创建 WebSocket 连接失败:', error)
      this.isConnecting = false
      if (this.shouldReconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.scheduleReconnect()
      }
    }
  }

  /**
   * 安排重连
   */
  private scheduleReconnect(): void {
    this.reconnectAttempts++
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1) // 指数退避

    console.log(`${delay}ms 后尝试第 ${this.reconnectAttempts} 次重连...`)

    setTimeout(() => {
      if (this.shouldReconnect) {
        this.connect()
      }
    }, delay)
  }

  /**
   * 发送消息
   */
  public send(data: string | object): boolean {
    if (this.ws?.readyState === WebSocket.OPEN) {
      const message = typeof data === 'string' ? data : JSON.stringify(data)
      this.ws.send(message)
      return true
    }
    console.warn('WebSocket 未连接，无法发送消息')
    return false
  }

  /**
   * 检查是否已连接
   */
  public isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  /**
   * 获取连接状态
   */
  public getConnectionState(): string {
    if (!this.ws) return 'disconnected'

    switch (this.ws.readyState) {
      case WebSocket.CONNECTING:
        return 'connecting'
      case WebSocket.OPEN:
        return 'connected'
      case WebSocket.CLOSING:
        return 'closing'
      case WebSocket.CLOSED:
        return 'disconnected'
      default:
        return 'unknown'
    }
  }

  /**
   * 断开连接
   */
  public disconnect(): void {
    this.shouldReconnect = false

    if (this.ws) {
      this.ws.close()
      this.ws = null
    }

    console.log('WebSocket 连接已手动断开')
  }

  /**
   * 重新连接
   */
  public reconnect(): void {
    this.disconnect()
    this.shouldReconnect = true
    this.reconnectAttempts = 0
    this.connect()
  }

  /**
   * 启动连接（供子类调用）
   */
  protected start(): void {
    this.connect()
  }
}

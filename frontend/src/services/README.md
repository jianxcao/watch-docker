# WebSocket 服务架构

这个目录包含了项目中的 WebSocket 相关服务，采用了基础类 + 具体实现的架构模式。

## 文件结构

```
services/
├── baseWebSocket.ts              # 通用 WebSocket 基础类
├── websocket.ts                  # 容器统计 WebSocket 服务
├── notificationWebSocket.example.ts  # 通知 WebSocket 服务示例
└── README.md                     # 本文档
```

## 基础类 (BaseWebSocketService)

`BaseWebSocketService` 是一个抽象的泛型类，提供了 WebSocket 连接的通用功能：

### 核心功能
- ✅ **连接管理**: 自动连接、断开、重连
- ✅ **重连机制**: 指数退避算法，可配置重试次数和延迟
- ✅ **状态管理**: 连接状态跟踪和查询
- ✅ **错误处理**: 统一的错误处理和日志记录
- ✅ **消息发送**: 支持字符串和对象消息发送
- ✅ **协议支持**: 支持自定义 WebSocket 子协议

### 配置选项
```typescript
interface WebSocketConfig {
  maxReconnectAttempts?: number  // 最大重连次数，默认 5
  reconnectDelay?: number        // 重连延迟（毫秒），默认 1000
  protocols?: string | string[]  // WebSocket 子协议
}
```

## 如何创建新的 WebSocket 服务

### 1. 定义消息类型
```typescript
import type { BaseMessage } from './baseWebSocket'

// 扩展基础消息接口
interface YourMessage extends BaseMessage {
  type: 'your-message-type'
  data: {
    // 你的数据结构
  }
}
```

### 2. 继承基础类
```typescript
import { BaseWebSocketService } from './baseWebSocket'

export class YourWebSocketService extends BaseWebSocketService<YourMessage> {
  constructor(token: string) {
    super(token, {
      maxReconnectAttempts: 5,
      reconnectDelay: 1000,
    })
    this.start() // 启动连接
  }

  // 必须实现：获取 WebSocket URL
  protected getWebSocketUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    return `${protocol}//${host}/api/v1/your-endpoint/ws?token=${this.token}`
  }

  // 必须实现：处理接收到的消息
  protected handleMessage(message: YourMessage): void {
    // 处理消息逻辑
  }
}
```

### 3. 可选的生命周期方法
```typescript
export class YourWebSocketService extends BaseWebSocketService<YourMessage> {
  // 连接成功时调用
  protected onConnected(): void {
    super.onConnected()
    console.log('你的服务已连接')
    // 可以发送初始化消息
    this.send({ type: 'init' })
  }

  // 连接关闭时调用
  protected onDisconnected(event: CloseEvent): void {
    super.onDisconnected(event)
    console.log('你的服务已断开')
  }

  // 连接错误时调用
  protected onError(error: Event): void {
    super.onError(error)
    console.error('你的服务连接错误')
  }
}
```

## 现有服务

### StatsWebSocketService (容器统计)
- **文件**: `websocket.ts`
- **功能**: 获取 Docker 容器的实时统计信息
- **端点**: `/api/v1/containers/stats/ws`
- **消息类型**: `StatsMessage`

### NotificationWebSocketService (示例)
- **文件**: `notificationWebSocket.example.ts`
- **功能**: 演示如何实现通知推送服务
- **特性**: 支持频道订阅、多种通知类型

## 最佳实践

### 1. 单例模式
对于全局使用的服务，推荐使用单例模式：

```typescript
let yourService: YourWebSocketService | null = null

export function getYourService(token: string): YourWebSocketService {
  if (!yourService) {
    yourService = new YourWebSocketService(token)
  }
  return yourService
}

export function destroyYourService(): void {
  if (yourService) {
    yourService.disconnect()
    yourService = null
  }
}
```

### 2. 在 Vue 组件中使用
```typescript
// 在组件中
import { getYourService } from '@/services/yourWebSocket'

const yourService = getYourService(userToken)

// 添加消息监听
yourService.addCallback((data) => {
  // 处理数据
})

// 组件卸载时清理
onUnmounted(() => {
  yourService.removeCallback(callback)
})
```

### 3. 错误处理
```typescript
// 监听连接状态
const connectionState = computed(() => yourService.getConnectionState())

// 手动重连
if (!yourService.isConnected()) {
  yourService.reconnect()
}
```

### 4. 消息发送
```typescript
// 发送字符串消息
yourService.send('hello')

// 发送对象消息
yourService.send({
  type: 'command',
  action: 'start',
  target: 'container-id'
})
```

## 扩展指南

如果需要为新功能添加 WebSocket 支持：

1. 参考 `notificationWebSocket.example.ts`
2. 定义你的消息接口
3. 继承 `BaseWebSocketService`
4. 实现必要的抽象方法
5. 添加业务特定的方法
6. 考虑是否需要单例模式

这种架构确保了代码的可重用性和一致性，同时保持了各个服务的独立性。

# Term 组件

## 设计理念

TermView 组件遵循**单一职责原则**，只负责终端的渲染和交互，不包含任何业务逻辑（如 WebSocket 连接）。

### 职责划分

```
┌─────────────────────────────────────┐
│     ComposeLogsModal (业务层)       │
│  - WebSocket 连接管理               │
│  - 连接状态管理                     │
│  - 业务逻辑处理                     │
│  - 错误处理和消息提示               │
└──────────────┬──────────────────────┘
               │ 使用
               ▼
┌─────────────────────────────────────┐
│      TermView (展示层)              │
│  - 终端初始化                       │
│  - 文本渲染                         │
│  - 大小适应                         │
│  - 用户输入处理                     │
└─────────────────────────────────────┘
```

## 组件结构

```
Term/
├── TermView.vue    # 终端组件
├── config.ts       # 默认配置（主题等）
└── README.md       # 文档
```

## TermView 组件 API

### Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `config` | `TermConfig` | - | 终端配置对象 |
| `autoFit` | `boolean` | `true` | 是否自动适应容器大小 |

### TermConfig 配置项

```typescript
interface TermConfig {
  theme?: ITheme           // 终端主题
  fontSize?: number        // 字体大小，默认 13
  fontFamily?: string      // 字体
  rows?: number            // 行数，默认 30
  cols?: number            // 列数，默认自动
  scrollback?: number      // 滚动缓冲区，默认 10000
  cursorBlink?: boolean    // 光标闪烁，默认 false
  convertEol?: boolean     // 转换行尾，默认 true
  disableStdin?: boolean   // 禁用输入（日志模式），默认 false
}
```

### Events

| 事件 | 参数 | 说明 |
|------|------|------|
| `ready` | `terminal: Terminal` | 终端初始化完成 |
| `data` | `data: string` | 用户输入数据（仅在启用输入时） |
| `resize` | `size: { cols, rows }` | 终端大小改变 |

### Methods（通过 ref 调用）

| 方法 | 参数 | 说明 |
|------|------|------|
| `write(data: string)` | 文本 | 写入文本 |
| `writeln(data: string)` | 文本 | 写入一行 |
| `clear()` | - | 清空终端 |
| `reset()` | - | 重置终端 |
| `fit()` | - | 调整大小 |
| `scrollToBottom()` | - | 滚动到底部 |
| `getTerminal()` | - | 获取 xterm 实例 |

## 使用示例

### 1. 日志查看模式（只读）

```vue
<template>
  <TermView 
    ref="termRef" 
    :config="logConfig" 
    @ready="handleReady" 
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import TermView, { type TermConfig } from '@/components/Term/TermView.vue'

const termRef = ref<InstanceType<typeof TermView>>()

// 日志模式配置：禁用输入
const logConfig: TermConfig = {
  disableStdin: true,
  fontSize: 13,
  scrollback: 10000,
}

const handleReady = () => {
  // 终端就绪后，可以写入内容
  termRef.value?.writeln('日志查看器已启动')
  
  // 然后在业务代码中管理 WebSocket
  connectToLogStream()
}

const connectToLogStream = () => {
  const ws = new WebSocket('ws://...')
  
  ws.onmessage = (event) => {
    termRef.value?.write(event.data)
  }
}
</script>
```

### 2. 交互式终端模式

```vue
<template>
  <TermView 
    ref="termRef" 
    :config="terminalConfig" 
    @ready="handleReady"
    @data="handleUserInput"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import TermView, { type TermConfig } from '@/components/Term/TermView.vue'

const termRef = ref<InstanceType<typeof TermView>>()

// 交互式配置：启用输入
const terminalConfig: TermConfig = {
  disableStdin: false,
  cursorBlink: true,
  fontSize: 14,
}

const handleReady = () => {
  termRef.value?.writeln('$ 欢迎使用终端')
  termRef.value?.write('$ ')
}

const handleUserInput = (data: string) => {
  // 处理用户输入
  termRef.value?.write(data)
  
  // 发送到后端（通过 WebSocket 或 API）
  sendToBackend(data)
}
</script>
```

### 3. 实际案例：ComposeLogsModal

参考 `ComposeLogsModal.vue` 的完整实现：

```vue
<template>
  <TermView ref="termRef" :config="termConfig" @ready="handleTermReady" />
</template>

<script setup lang="ts">
// 1. 配置终端（只读模式）
const termConfig: TermConfig = {
  disableStdin: true,
  cursorBlink: false,
  fontSize: 13,
  scrollback: 10000,
  convertEol: true,
}

// 2. 终端就绪后连接 WebSocket
const handleTermReady = () => {
  connectWebSocket()
}

// 3. 在业务代码中管理 WebSocket
const connectWebSocket = () => {
  const ws = new WebSocket(getWebSocketUrl())
  
  ws.onopen = () => {
    termRef.value?.writeln('\x1b[32m已连接\x1b[0m')
  }
  
  ws.onmessage = (event) => {
    termRef.value?.write(event.data)
  }
  
  ws.onclose = () => {
    termRef.value?.writeln('\x1b[33m已断开\x1b[0m')
  }
  
  ws.onerror = () => {
    termRef.value?.writeln('\x1b[31m连接错误\x1b[0m')
  }
}
</script>
```

## 为什么不在 TermView 中集成 WebSocket？

### 设计原则

1. **单一职责**
   - TermView 只负责终端渲染
   - WebSocket 连接是业务逻辑

2. **灵活性**
   - 不同场景可能需要不同的连接方式
   - 可能需要自定义认证、重连逻辑等

3. **可测试性**
   - TermView 可以独立测试
   - WebSocket 逻辑可以独立测试

4. **可复用性**
   - TermView 可用于非 WebSocket 场景
   - 例如：本地日志文件查看、命令行输出等

### 比较

#### ❌ 不推荐：TermView 集成 WebSocket

```vue
<!-- TermView 内部管理 WebSocket -->
<TermView websocket-url="ws://..." />
```

**问题：**
- 如何处理认证 token？
- 如何处理重连逻辑？
- 如何处理不同的错误场景？
- 如何在不需要 WebSocket 时使用？

#### ✅ 推荐：业务层管理 WebSocket

```vue
<!-- TermView 只负责渲染 -->
<TermView ref="term" />

<script>
// 业务层管理 WebSocket
const ws = new WebSocket(url)
ws.onmessage = (e) => term.value?.write(e.data)
</script>
```

**优势：**
- 完全控制连接逻辑
- 易于添加业务特定的处理
- 可以使用任何数据源
- 组件更纯粹、更易维护

## ANSI 颜色代码

TermView 支持完整的 ANSI 转义序列：

```typescript
// 颜色
termRef.value?.writeln('\x1b[31m红色\x1b[0m')     // 红色
termRef.value?.writeln('\x1b[32m绿色\x1b[0m')     // 绿色
termRef.value?.writeln('\x1b[33m黄色\x1b[0m')     // 黄色
termRef.value?.writeln('\x1b[34m蓝色\x1b[0m')     // 蓝色
termRef.value?.writeln('\x1b[35m品红\x1b[0m')     // 品红
termRef.value?.writeln('\x1b[36m青色\x1b[0m')     // 青色

// 样式
termRef.value?.writeln('\x1b[1m加粗\x1b[0m')      // 加粗
termRef.value?.writeln('\x1b[4m下划线\x1b[0m')    // 下划线
termRef.value?.writeln('\x1b[7m反色\x1b[0m')      // 反色

// 组合
termRef.value?.writeln('\x1b[1;32m加粗绿色\x1b[0m')
```

## 注意事项

1. **容器高度**
   - TermView 需要明确的高度才能正常显示
   - 建议父容器设置 `height` 或 `flex: 1`

2. **性能**
   - `scrollback` 设置过大可能影响性能
   - 建议根据实际需求设置（默认 10000 行）

3. **清理**
   - 组件会自动清理资源
   - 如果有 WebSocket，记得在组件卸载前关闭

4. **大小调整**
   - 默认启用 `autoFit`，自动适应容器大小
   - 如需手动控制，设置 `autoFit={false}` 并调用 `fit()` 方法

## 未来扩展

可以基于 TermView 构建：

- ✅ Compose 日志查看（已实现）
- 🔮 容器日志查看
- 🔮 容器 Shell 交互
- 🔮 构建日志实时显示
- 🔮 SSH 终端
- 🔮 本地文件查看器
- 🔮 命令行输出展示


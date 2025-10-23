# Compose 创建进度组件重构

## 重构目标

将 Compose 创建和启动的日志显示功能重构为一个独立组件，提升代码质量和可维护性。

## 主要改进

### 1. 使用 VueUse 管理 WebSocket

**重构前**：

```typescript
// 手动管理 WebSocket 连接
let ws: WebSocket | null = null;

ws = new WebSocket(wsUrl);

ws.onopen = () => {
  // ...
};

ws.onmessage = (event) => {
  // ...
};

ws.onerror = (error) => {
  // ...
};

ws.onclose = (event) => {
  // ...
};

// 需要手动清理
const cleanupWebSocket = () => {
  if (ws) {
    ws.close();
    ws = null;
  }
};
```

**重构后**：

```typescript
// 使用 VueUse 的 useWebSocket Hook
import { useWebSocket } from "@vueuse/core";

const {
  data: wsData,
  send,
  open,
  close,
} = useWebSocket(wsUrl.value, {
  immediate: false,
  autoReconnect: false,
  heartbeat: {
    message: "ping",
    interval: 30000,
  },
  onConnected: () => {
    // 连接成功回调
  },
  onError: () => {
    // 错误处理
  },
  onDisconnected: () => {
    // 断开连接处理
  },
});

// 监听消息
watch(wsData, (data) => {
  // 处理消息
});
```

**优势**：

- ✅ 自动管理连接生命周期
- ✅ 内置心跳机制
- ✅ 响应式数据流
- ✅ 无需手动清理资源
- ✅ 更少的样板代码

### 2. 使用 Term 组件显示日志

**重构前**：

```vue
<template>
  <div class="log-container" ref="logContainerRef">
    <div
      v-for="(log, index) in logs"
      :key="index"
      :class="['log-line', `log-${log.type}`]"
    >
      {{ log.message }}
    </div>
  </div>
</template>

<style>
.log-container {
  max-height: 400px;
  overflow-y: auto;
  background-color: #1e1e1e;
  color: #d4d4d4;
  font-family: "Menlo", "Monaco", "Courier New", monospace;
  /* ... 大量样式代码 ... */
}
</style>
```

**重构后**：

```vue
<template>
  <TermView ref="termRef" height="400px" :config="termConfig" />
</template>

<script setup>
const termConfig: TermConfig = {
  disableStdin: true, // 禁用输入，仅用于日志显示
  scrollback: 5000,
  cursorBlink: false,
};

// 写入日志
const writeLine = (text: string) => {
  termRef.value?.write(text);
  termRef.value?.scrollToBottom();
};
</script>
```

**优势**：

- ✅ 复用现有的 xterm.js 功能
- ✅ 更好的性能（虚拟滚动）
- ✅ 支持 ANSI 颜色代码
- ✅ 自动主题适配
- ✅ 无需自定义样式
- ✅ 更专业的终端体验

### 3. 组件化封装

**文件结构**：

```
frontend/src/components/
├── ComposeCreateProgress.vue  (新增独立组件)
└── Term/
    ├── TermView.vue           (复用)
    └── config.ts
```

**ComposeCreateProgress 组件 API**：

```typescript
interface Props {
  show: boolean; // 是否显示
  projectName?: string; // 项目名称
  yamlContent?: string; // YAML 配置
}

interface Emits {
  (e: "success", composeFile: string): void; // 创建成功
  (e: "error", message: string): void; // 创建失败
  (e: "complete"): void; // 创建完成
}

// 暴露的方法
defineExpose({
  start, // 开始创建
  reset, // 重置状态
  status, // 当前状态
});
```

**使用示例**：

```vue
<template>
  <ComposeCreateProgress
    ref="createProgressRef"
    :show="showProgress"
    :project-name="formData.name"
    :yaml-content="formData.yaml"
    @success="handleCreateSuccess"
    @error="handleCreateError"
    @complete="handleCreateComplete"
  />
</template>

<script setup>
const createProgressRef = ref();
const showProgress = ref(false);

const handleSubmit = async () => {
  showProgress.value = true;
  createProgressRef.value?.start();
};
</script>
```

## 代码对比

### ComposeCreateView.vue 简化

**重构前（370+ 行）**：

- 手动管理 WebSocket 连接
- 自定义日志容器和样式
- 大量的状态管理代码
- 复杂的消息处理逻辑

**重构后（350 行）**：

- 引入组件，简单调用
- 专注于表单逻辑
- 清晰的事件处理
- 更易维护

### 新增 ComposeCreateProgress 组件（230 行）

**职责单一**：

- WebSocket 连接管理
- 日志显示和格式化
- 状态管理和事件触发

**可复用性**：

- 可在其他需要类似功能的地方复用
- Props 和 Emits 设计清晰
- 独立的生命周期管理

## 技术细节

### ANSI 颜色代码

使用 xterm.js 的 ANSI 颜色支持，使日志更加美观：

```typescript
// 蓝色信息
writeLine("\x1b[36mℹ 正在连接服务器...\x1b[0m\r\n");

// 绿色成功
writeLine("\x1b[32m✓ 项目创建成功\x1b[0m\r\n");

// 红色错误
writeLine("\x1b[31m✗ 创建失败\x1b[0m\r\n");

// 重置颜色
writeLine("\x1b[0m");
```

### 状态管理

使用 `status` ref 跟踪当前状态：

```typescript
const status = ref<"idle" | "connecting" | "creating" | "success" | "error">(
  "idle"
);
```

在 UI 中显示对应的状态标签：

```vue
<n-tag v-if="status === 'connecting'" type="info">连接中...</n-tag>
<n-tag v-else-if="status === 'creating'" type="warning">创建中...</n-tag>
<n-tag v-else-if="status === 'success'" type="success">创建成功</n-tag>
<n-tag v-else-if="status === 'error'" type="error">创建失败</n-tag>
```

### 自动滚动

使用 `nextTick` 确保 DOM 更新后再滚动：

```typescript
const writeLine = (text: string) => {
  nextTick(() => {
    termRef.value?.write(text);
    termRef.value?.scrollToBottom();
  });
};
```

## 性能优化

### 1. Terminal 虚拟滚动

xterm.js 内置虚拟滚动，即使有数千行日志也能保持流畅。

### 2. 按需连接

WebSocket 不会立即连接，只在调用 `start()` 方法时才建立连接：

```typescript
const { open } = useWebSocket(wsUrl.value, {
  immediate: false, // 不立即连接
});

const start = () => {
  open(); // 手动打开连接
};
```

### 3. 自动清理

VueUse 会在组件卸载时自动清理 WebSocket 连接，无需手动处理。

## 可扩展性

### 1. 添加更多消息类型

只需在 `watch(wsData)` 中添加新的 case：

```typescript
switch (type) {
  case "INFO":
    writeLine(`\x1b[36mℹ ${msg}\x1b[0m`);
    break;
  case "WARNING": // 新增
    writeLine(`\x1b[33m⚠ ${msg}\x1b[0m`);
    break;
  // ...
}
```

### 2. 自定义 Terminal 配置

通过 props 传递不同的配置：

```vue
<ComposeCreateProgress
  :term-config="{
    scrollback: 10000,
    fontSize: 14,
  }"
/>
```

### 3. 添加操作按钮

在组件内添加取消、重试等按钮：

```vue
<template #footer>
  <n-space>
    <n-button @click="handleCancel">取消</n-button>
    <n-button v-if="status === 'error'" @click="handleRetry">重试</n-button>
  </n-space>
</template>
```

## 总结

| 方面               | 重构前           | 重构后                  |
| ------------------ | ---------------- | ----------------------- |
| **代码行数**       | 500+ 行          | 350 (页面) + 230 (组件) |
| **WebSocket 管理** | 手动管理         | VueUse Hook             |
| **日志显示**       | 自定义 div + CSS | xterm.js Terminal       |
| **可复用性**       | 低               | 高                      |
| **可维护性**       | 中等             | 高                      |
| **性能**           | 普通             | 优秀                    |
| **用户体验**       | 良好             | 优秀                    |

### 主要收益

1. **更少的代码**：使用成熟的库减少样板代码
2. **更好的性能**：Terminal 虚拟滚动，支持大量日志
3. **更易维护**：职责分离，组件化封装
4. **更好的体验**：专业的终端样式，ANSI 颜色支持
5. **更强的扩展性**：清晰的 API，易于扩展

## 相关文件

- `frontend/src/components/ComposeCreateProgress.vue` - 新增进度组件
- `frontend/src/pages/ComposeCreateView.vue` - 简化后的创建页面
- `frontend/src/components/Term/TermView.vue` - 复用的 Terminal 组件
- `doc/compose-create-with-up.md` - 原功能文档

## 依赖

```json
{
  "@vueuse/core": "^13.9.0",
  "@xterm/xterm": "5.6.0-beta.134"
}
```

## 参考资料

- [VueUse useWebSocket](https://vueuse.org/core/useWebSocket/)
- [xterm.js Documentation](https://xtermjs.org/)
- [ANSI Escape Codes](https://en.wikipedia.org/wiki/ANSI_escape_code)

# Compose 创建并启动功能实现

## 功能概述

实现了创建 Docker Compose 项目后自动启动项目，并实时显示创建和启动日志的功能。整个过程通过 WebSocket 连接实现，提供流畅的实时反馈体验。

## 技术架构

### 后端实现

#### 1. WebSocket 端点

**路由**: `GET /api/compose/create-and-up/ws`

**文件**: `backend/internal/api/compose_socket.go`

**核心流程**:

1. **升级 WebSocket 连接**

   ```go
   conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
   ```

2. **接收创建请求**

   ```go
   var req struct {
       Name        string `json:"name"`
       YamlContent string `json:"yamlContent"`
   }
   conn.ReadJSON(&req)
   ```

3. **创建项目文件**

   ```go
   composeFile, err := s.composeClient.SaveNewProject(ctx, req.Name, req.YamlContent)
   ```

4. **启动项目（docker compose up -d）**

   ```go
   result := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
       ExecPath:      projectPath,
       Args:          []string{"up", "-d"},
       OperationName: "compose up",
   })
   ```

5. **实时发送日志**

   - 读取命令输出流
   - 通过 WebSocket 发送到前端
   - 区分不同类型的消息（INFO、SUCCESS、ERROR、LOG）

6. **获取项目状态**

   ```go
   statusResult := composecli.ExecuteDockerComposeCommandStream(ctx, composecli.ExecDockerComposeStreamOptions{
       ExecPath:      projectPath,
       Args:          []string{"ps"},
       OperationName: "compose ps",
   })
   ```

7. **完成并通知**
   ```go
   sendWSMessage(conn, "COMPLETE", composeFile)
   ```

#### 2. 消息格式

**发送格式**:

```json
{
  "type": "INFO|SUCCESS|ERROR|LOG|COMPLETE",
  "message": "消息内容"
}
```

**消息类型**:

- `INFO`: 普通信息（蓝色）
- `SUCCESS`: 成功信息（绿色）
- `ERROR`: 错误信息（红色）
- `LOG`: Docker Compose 输出日志（默认颜色）
- `COMPLETE`: 完成信号（附带 composeFile 路径）

#### 3. 错误处理

- 创建文件失败：发送 ERROR 消息，保持连接打开以便查看错误
- 启动失败：发送 ERROR 消息，显示详细错误信息
- 连接超时：5 分钟超时保护
- 资源清理：defer 确保 Reader 正确关闭

### 前端实现

#### 1. 组件结构

**文件**: `frontend/src/pages/ComposeCreateView.vue`

**核心元素**:

1. **表单区域**

   - 项目名称输入
   - 自动生成路径显示
   - YAML 编辑器

2. **日志显示区域**

   ```vue
   <n-card v-if="showLogs" title="创建日志">
     <div class="log-container">
       <div v-for="(log, index) in logs" :class="['log-line', `log-${log.type}`]">
         {{ log.message }}
       </div>
     </div>
   </n-card>
   ```

3. **操作按钮**
   - 取消按钮（创建过程中禁用）
   - 创建并启动按钮（显示加载状态）

#### 2. WebSocket 连接管理

**连接建立**:

```typescript
const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
const host = window.location.host;
const token = settingStore.getToken();
let wsUrl = `${protocol}//${host}/api/compose/create-and-up/ws`;

if (token) {
  wsUrl += `?token=${encodeURIComponent(token)}`;
}

ws = new WebSocket(wsUrl);
```

**消息发送**:

```typescript
ws.onopen = () => {
  ws?.send(
    JSON.stringify({
      name: formData.value.name,
      yamlContent: formData.value.yaml,
    })
  );
};
```

**消息接收**:

```typescript
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  const { type, message } = data;

  switch (type) {
    case "INFO":
      addLog("info", message);
      break;
    case "SUCCESS":
      addLog("success", message);
      break;
    case "ERROR":
      addLog("error", message);
      break;
    case "LOG":
      addLog("log", message);
      break;
    case "COMPLETE":
      addLog("success", "\n✓ 项目创建并启动成功！\n");
      setTimeout(() => router.push({ name: "compose" }), 2000);
      break;
  }
};
```

**连接清理**:

```typescript
const cleanupWebSocket = () => {
  if (ws) {
    ws.close();
    ws = null;
  }
};

onBeforeUnmount(() => {
  cleanupWebSocket();
});
```

#### 3. 日志管理

**添加日志并自动滚动**:

```typescript
const addLog = (type: string, message: string) => {
  logs.value.push({ type, message });
  nextTick(() => {
    if (logContainerRef.value) {
      logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight;
    }
  });
};
```

**日志类型样式**:

- `log-info`: 蓝色，普通信息
- `log-success`: 绿色，成功信息
- `log-error`: 红色，错误信息
- `log-log`: 默认颜色，Docker 输出

#### 4. 样式设计

**日志容器**:

```less
.log-container {
  max-height: 400px;
  overflow-y: auto;
  background-color: #1e1e1e;
  color: #d4d4d4;
  font-family: "Menlo", "Monaco", "Courier New", monospace;
  font-size: 13px;
  line-height: 1.6;
  padding: 12px;
  border-radius: 4px;
}
```

**明暗主题适配**:

- 暗色主题：`#1e1e1e` 背景
- 亮色主题：`#f5f5f5` 背景（响应 `prefers-color-scheme`）

## 用户流程

1. **填写项目信息**

   - 输入项目名称
   - 编辑 docker-compose.yml 配置

2. **点击"创建并启动项目"**

   - 按钮显示加载状态
   - 取消按钮被禁用

3. **查看实时日志**

   - 连接服务器
   - 创建项目文件
   - 启动项目
   - 拉取镜像（如果需要）
   - 创建并启动容器
   - 显示项目状态

4. **完成**
   - 显示成功消息
   - 2 秒后自动跳转到项目列表页

## 错误处理

### 前端错误处理

1. **连接失败**

   ```typescript
   ws.onerror = (error) => {
     addLog("error", "WebSocket 连接错误\n");
     message.error("连接失败");
     submitting.value = false;
   };
   ```

2. **异常关闭**
   ```typescript
   ws.onclose = (event) => {
     if (!event.wasClean) {
       addLog("error", "连接异常关闭\n");
     }
     submitting.value = false;
     cleanupWebSocket();
   };
   ```

### 后端错误处理

1. **创建项目失败**

   - 发送 ERROR 消息
   - 保持连接，允许查看完整错误信息

2. **启动失败**

   - 发送 ERROR 消息
   - 显示 Docker Compose 的错误输出

3. **超时保护**
   - 5 分钟超时限制
   - 防止长时间挂起

## 优势

### 1. 实时反馈

- 用户可以实时看到创建和启动进度
- 不需要等待整个过程完成才知道结果

### 2. 错误诊断

- 详细的错误日志
- 可以看到 Docker Compose 的完整输出
- 便于排查配置问题

### 3. 用户体验

- 流畅的实时更新
- 清晰的视觉反馈（颜色编码）
- 自动滚动日志
- 完成后自动跳转

### 4. 可扩展性

- 消息类型易于扩展
- 可以添加更多步骤（如健康检查）
- 支持取消操作（未来可实现）

## 后续优化建议

1. **添加取消功能**

   - 允许用户中途取消创建
   - 清理已创建的资源

2. **进度指示**

   - 显示当前步骤
   - 总进度百分比

3. **日志下载**

   - 允许下载完整日志
   - 便于问题报告

4. **失败重试**

   - 创建失败后提供重试选项
   - 保留已填写的配置

5. **预检查**
   - 在创建前验证 YAML 语法
   - 检查镜像是否可用
   - 检查端口冲突

## 相关文件

### 后端

- `backend/internal/api/compose_socket.go` - WebSocket 处理器
- `backend/internal/api/compose_router.go` - 路由配置
- `backend/internal/composecli/client.go` - Compose 客户端

### 前端

- `frontend/src/pages/ComposeCreateView.vue` - 创建页面
- `frontend/src/store/setting.ts` - 设置存储（Token 管理）
- `frontend/src/components/YamlEditor/` - YAML 编辑器

## 参考资料

- [WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
- [Docker Compose CLI](https://docs.docker.com/compose/reference/)
- [Gin WebSocket Example](https://github.com/gin-gonic/examples/tree/master/gorilla-websocket)

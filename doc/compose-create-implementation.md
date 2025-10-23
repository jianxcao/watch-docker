# Compose 项目创建功能实现文档

## 功能概述

实现了一个完整的 Docker Compose 项目创建功能，用户可以通过 Web 界面创建新的 Compose 项目，包括项目名称输入、自动生成存放路径以及 YAML 配置编辑。

## 设计要点

### 1. 用户界面

- **项目名称**：用户手动输入，支持字母、数字、下划线和连字符，长度 1-50 字符
- **存放路径**：根据 `APP_PATH` 环境变量和项目名称自动生成，不可修改（disabled）
- **YAML 编辑器**：使用 naive-ui 的 `n-code` 组件，提供语法高亮和基本的 YAML 验证

### 2. 编辑器选择

选择了 **CodeMirror 6**，理由如下：

- ✅ 支持 YAML 语法高亮
- ✅ 移动端（wap 端）支持优秀，专为触摸优化
- ✅ 现代化、轻量级（~40KB gzipped）
- ✅ 可编辑，功能完善
- ✅ 性能出色，虚拟 DOM 渲染
- ✅ 支持自动换行和行号显示
- ✅ 原生 TypeScript 支持

备选方案：

- Monaco Editor：功能更强大但体积较大（~250KB+）
- Ace Editor：成熟但维护较慢，体积较大
- 简单 textarea：无语法高亮，用户体验较差

## 实现细节

### 前端部分

#### 1. 新增组件

**文件**: `frontend/src/components/ComposeCreateModal.vue`

主要功能：

- 表单验证（项目名称格式、YAML 格式）
- 自动生成路径（`APP_PATH + 项目名称`）
- YAML 编辑器集成
- 文件导入功能

关键特性：

```vue
<!-- 使用独立的 YamlEditor 组件 -->
<YamlEditor
  v-model="formData.yaml"
  placeholder="请输入 docker-compose.yml 配置内容"
  min-height="300px"
  max-height="500px"
  @change="handleYamlChange"
/>
```

#### YamlEditor 组件

**文件**: `frontend/src/components/YamlEditor.vue`

独立封装的 YAML 编辑器组件，基于 CodeMirror 6：

特性：

- 完整的编辑功能
- YAML 语法高亮
- 行号显示
- 自动换行
- 主题跟随系统（明暗主题）
- 移动端触摸优化
- 响应式设计

Props:

- `modelValue`: 双向绑定的值
- `placeholder`: 占位符文本
- `readonly`: 只读模式
- `minHeight`: 最小高度
- `maxHeight`: 最大高度

Events:

- `update:modelValue`: 值变化事件
- `change`: 内容变化事件

暴露方法:

- `focus()`: 聚焦编辑器
- `getValue()`: 获取当前值
- `setValue(value)`: 设置值

#### 2. 更新的组件

**文件**: `frontend/src/pages/ComposeView.vue`

新增：

- 创建项目按钮点击处理
- 创建成功后刷新列表
- 集成 `ComposeCreateModal` 组件

#### 3. API 接口

**文件**: `frontend/src/common/api.ts`

新增接口：

```typescript
saveNewProject: (name: string, yamlContent: string) =>
  axios.post<{ ok: boolean; composeFile: string }>(`/compose/new`, {
    name,
    yamlContent,
  });
```

#### 4. Store 更新

**文件**: `frontend/src/store/compose.ts`

新增方法：

```typescript
const saveNewProject = async (name: string, yamlContent: string) => {
  // 调用后端 API 保存项目
  const response = await composeApi.saveNewProject(name, yamlContent);
  // 处理响应...
};
```

#### 5. 类型定义更新

**文件**: `frontend/src/store/setting.ts`

添加 `appPath` 字段到 `SystemInfo` 接口：

```typescript
interface SystemInfo {
  // ... 其他字段
  appPath: string;
}
```

### 后端部分

#### 1. API 路由

**文件**: `backend/internal/api/compose_router.go`

新增路由：

```go
protected.POST("/compose/new", s.handleSaveNewProject())
```

新增处理函数：

```go
func (s *Server) handleSaveNewProject() gin.HandlerFunc {
  // 接收项目名称和 YAML 内容
  // 调用 composecli.SaveNewProject
  // 返回创建的 compose 文件路径
}
```

#### 2. Compose 客户端

**文件**: `backend/internal/composecli/client.go`

新增方法：

```go
func (c *Client) SaveNewProject(ctx context.Context, name string, yamlContent string) (string, error) {
  // 1. 检查 APP_PATH 是否设置
  // 2. 创建项目目录：APP_PATH/项目名称
  // 3. 写入 docker-compose.yml 文件
  // 4. 返回文件完整路径
}
```

实现逻辑：

- 使用 `os.MkdirAll` 创建项目目录
- 使用 `os.WriteFile` 保存 YAML 内容
- 失败时自动清理已创建的目录
- 记录详细的日志信息

#### 3. 系统信息 API

**文件**: `backend/internal/api/router.go`

更新 `handleGetInfo` 方法，添加 `appPath` 字段：

```go
info := gin.H{
  // ... 其他字段
  "appPath": envCfg.APP_PATH,
}
```

## 流程图

```
用户操作流程：
1. 点击 "创建项目" 按钮
2. 输入项目名称
   ↓
3. 系统自动生成存放路径（APP_PATH/项目名称）
   ↓
4. 编辑 YAML 配置
   - 可以手动输入
   - 可以插入示例
   - 可以导入文件
   ↓
5. 点击 "创建" 按钮
   ↓
6. 前端验证（名称格式、YAML 格式）
   ↓
7. 调用后端 API (/compose/new)
   ↓
8. 后端创建目录并保存 YAML 文件
   ↓
9. 返回成功，前端刷新项目列表
```

## YAML 验证规则

前端实现了基本的 YAML 格式验证：

1. **必需字段检查**：检查是否包含 `services:` 关键字
2. **引号检查**：检查单引号和双引号是否成对
3. **非空检查**：YAML 内容不能为空

注意：这些是基础验证，更复杂的 YAML 语法验证由后端 Docker Compose 执行。

## 示例 YAML

系统提供的默认示例：

```yaml
version: "3.8"

services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    volumes:
      - ./html:/usr/share/nginx/html
    environment:
      - NGINX_HOST=localhost
      - NGINX_PORT=80
    restart: unless-stopped

  app:
    image: node:18-alpine
    working_dir: /app
    volumes:
      - ./app:/app
    command: npm start
    depends_on:
      - web
    restart: unless-stopped
```

## 移动端适配

编辑器在移动端进行了优化：

```less
@media (max-width: 768px) {
  .yaml-editor-wrapper {
    .yaml-toolbar {
      flex-direction: column;
      align-items: flex-start;
      gap: 8px;
    }

    .yaml-editor {
      min-height: 250px;
      max-height: 300px;
    }
  }
}
```

特性：

- 工具栏垂直布局
- 编辑器高度自适应
- 触摸友好的按钮尺寸

## 环境变量配置

后端需要配置 `APP_PATH` 环境变量：

```bash
# Docker 环境
APP_PATH=/data/compose

# 本地开发环境
APP_PATH=/path/to/compose/projects
```

如果未设置 `APP_PATH`：

- 后端将返回错误："APP_PATH 未设置，无法创建项目"
- 前端显示默认路径：`/data/compose`

## 错误处理

### 前端

1. **表单验证错误**：

   - 项目名称格式不正确
   - 路径为空
   - YAML 格式错误

2. **网络错误**：

   - API 调用失败
   - 超时处理

3. **业务错误**：
   - 项目名称重复
   - 磁盘空间不足

### 后端

1. **环境配置错误**：

   - APP_PATH 未设置
   - APP_PATH 路径不存在

2. **文件系统错误**：

   - 目录创建失败
   - 文件写入失败
   - 权限不足

3. **清理机制**：
   - 写入文件失败时自动清理已创建的目录

## 安全考虑

1. **路径安全**：

   - 项目名称限制字符集（防止路径遍历攻击）
   - 后端使用 `filepath.Join` 安全拼接路径

2. **输入验证**：

   - 项目名称长度限制（1-50 字符）
   - YAML 内容不为空

3. **文件权限**：
   - 目录权限：0755
   - 文件权限：0644

## 测试建议

### 功能测试

1. 正常流程测试

   - 创建简单项目
   - 创建复杂项目（多服务）
   - 导入 YAML 文件

2. 异常流程测试

   - 项目名称重复
   - YAML 格式错误
   - APP_PATH 未设置

3. 边界测试
   - 超长项目名称
   - 超大 YAML 文件
   - 特殊字符处理

### 兼容性测试

1. 浏览器兼容性

   - Chrome
   - Firefox
   - Safari
   - Edge

2. 移动端测试

   - iOS Safari
   - Android Chrome
   - 平板设备

3. 屏幕尺寸
   - 手机（<768px）
   - 平板（768px-1024px）
   - 桌面（>1024px）

## 后续优化建议

1. **编辑器增强**

   - 添加 YAML 自动补全
   - 提供更多模板示例
   - 支持 YAML 格式化功能

2. **验证增强**

   - 集成完整的 YAML parser
   - Docker Compose schema 验证
   - 实时语法检查

3. **用户体验**

   - 添加撤销/重做功能
   - 支持快捷键操作
   - 添加拖拽上传功能

4. **功能扩展**
   - 支持从 GitHub 导入
   - 项目模板库
   - 版本控制集成

## 依赖说明

### 前端依赖

已安装的依赖：

- naive-ui: 提供 UI 组件
- @vueuse/core: 提供响应式工具
- vue: 核心框架
- codemirror: 代码编辑器核心库（~40KB gzipped）
- @codemirror/lang-yaml: YAML 语言支持
- @codemirror/view: 视图层
- @codemirror/state: 状态管理
- @codemirror/commands: 编辑器命令

### 后端依赖

现有依赖，无需额外安装：

- gin: Web 框架
- docker/client: Docker 客户端
- zap: 日志库

## 总结

本次实现完成了一个功能完整、用户友好的 Docker Compose 项目创建功能，主要特点：

✅ 简洁直观的用户界面  
✅ 自动路径生成，减少用户输入  
✅ 移动端友好的 YAML 编辑器  
✅ 完善的错误处理和验证  
✅ 良好的代码组织和可维护性

该功能可以立即投入使用，并为后续的功能扩展奠定了良好的基础。

# Watch Docker 前端架构设计

## 1. 项目概述

Watch Docker 前端是一个 Docker 容器和镜像管理界面，支持容器状态监控、一键更新、镜像管理、Docker Compose 项目管理、系统设置等功能。前端采用现代化的技术栈，支持响应式设计，兼容移动端和 PC 端。

## 2. 技术栈

### 2.1 核心框架

- **Vue 3**: 前端框架，使用 Composition API
- **TypeScript**: 类型安全的 JavaScript
- **Vite**: 构建工具和开发服务器

### 2.2 UI 框架和样式

- **Naive UI**: 主要组件库
- **UnoCSS**: 原子化 CSS 框架
- **TailwindCSS@4**: CSS 工具类
- **Less**: CSS 预处理器

### 2.3 状态管理和路由

- **Pinia**: 状态管理
- **Vue Router**: 路由管理

### 2.4 工具库

- **Axios**: HTTP 请求库
- **dayjs**: 日期处理库
- **qrcode**: 二维码生成
- **@simplewebauthn/browser**: WebAuthn 客户端
- **xterm**: 终端模拟器
- **@monaco-editor/loader**: Monaco 编辑器加载器

## 3. 项目结构

```
frontend/src/
├── components/           # 公共组件
│   ├── LayoutView.vue         # 主布局组件
│   ├── LoadingView.vue        # 加载组件
│   ├── SiderContent.vue       # 侧边栏内容
│   ├── MobileDrawer.vue       # 移动端抽屉菜单
│   ├── StatusBadge.vue        # 状态徽章
│   ├── UpdateStatusBadge.vue  # 更新状态徽章
│   ├── RunningStatusBadge.vue # 运行状态徽章
│   ├── ContainerCard.vue      # 容器卡片
│   ├── ImageCard.vue          # 镜像卡片
│   ├── ComposeCard.vue        # Compose 项目卡片
│   ├── ConfigView.vue         # 配置视图组件
│   ├── ContainerImportModal.vue  # 容器导入模态框
│   ├── ImageImportModal.vue      # 镜像导入模态框
│   ├── ComposeLogsModal.vue      # Compose 日志模态框
│   ├── ComposeCreateProgress.vue # Compose 创建进度
│   ├── TwoFASetup.vue            # 二次验证设置
│   ├── TwoFAVerify.vue           # 二次验证验证
│   ├── Term/                     # 终端组件
│   │   ├── TermView.vue
│   │   └── config.ts
│   └── YamlEditor/               # YAML 编辑器组件
│       ├── YamlEditor.vue
│       ├── types.ts
│       └── utils.ts
├── pages/               # 页面组件
│   ├── HomeView.vue           # 首页/概览页面
│   ├── ContainersView.vue     # 容器列表页面
│   ├── ImagesView.vue         # 镜像列表页面
│   ├── ComposeView.vue        # Compose 项目列表页面
│   ├── ComposeCreateView.vue  # Compose 项目创建页面
│   ├── SettingsView.vue       # 系统设置页面
│   ├── TerminalView.vue       # 终端页面
│   ├── LoginView.vue          # 登录页面
│   └── LogsPageView.vue       # 日志查看页面
├── hooks/               # Vue 3 Composition API Hooks
│   ├── useContainer.ts        # 容器操作相关hooks
│   ├── useImage.ts            # 镜像操作相关hooks
│   ├── useCompose.ts          # Compose 操作相关hooks
│   ├── useResponsive.ts       # 响应式设计hooks
│   ├── useStatsWebSocket.ts   # 容器状态 WebSocket
│   └── useXhrUpload.ts        # XHR 上传相关hooks
├── store/               # Pinia 状态管理
│   ├── app.ts           # 应用全局状态
│   ├── auth.ts          # 认证状态
│   ├── setting.ts       # 设置状态
│   ├── container.ts     # 容器状态管理
│   ├── image.ts         # 镜像状态管理
│   └── compose.ts       # Compose 状态管理
├── common/              # 公共工具
│   ├── api.ts           # API 接口方法
│   ├── types.ts         # TypeScript 类型定义
│   ├── utils.ts         # 工具函数
│   └── axiosConfig.ts   # HTTP 请求配置
├── constants/           # 常量定义
│   ├── api.ts           # API 接口常量
│   ├── code.ts          # 状态码
│   └── msg.ts           # 消息定义
├── evt/                 # 事件相关
│   └── containerStats.ts # 容器状态事件
├── router/              # 路由配置
│   └── index.ts         # 路由定义
├── styles/              # 样式文件
│   └── mix.less         # 混合样式
├── assets/              # 静态资源
│   └── svg/             # SVG 图标
├── main.ts              # 应用入口
└── App.vue              # 根组件
```

## 4. 核心功能设计

### 4.1 布局设计

#### PC 端（≥1024px）

- 左侧固定导航菜单
- 右侧内容区域
- 顶部面包屑导航（可选）

#### 移动端（<1024px）

- 顶部标题栏
- 左侧抽屉菜单
- 内容区域

### 4.2 页面功能

#### 首页（HomeView.vue）

- 系统状态概览
- 容器和镜像统计信息
- 快速操作按钮
- 最近容器列表
- 实时资源监控

#### 容器管理（ContainersView.vue）

- 容器列表展示（卡片式）
- 状态过滤和搜索
- 单个容器操作：启动/停止/更新/删除
- 批量更新所有可更新容器
- 容器信息详情展示
- 实时资源使用监控（CPU/内存）
- 日志查看

#### 镜像管理（ImagesView.vue）

- 镜像列表展示（卡片式）
- 镜像信息展示（大小、创建时间、标签）
- 删除单个镜像
- 批量清理悬空镜像
- 使用状态提示
- 镜像导入功能

#### Compose 管理（ComposeView.vue）

- Compose 项目列表
- 项目状态展示
- 项目操作：启动/停止/重启/删除
- 项目详情查看
- 实时日志查看
- 新建项目入口

#### Compose 创建（ComposeCreateView.vue）

- YAML 编辑器（Monaco）
- 语法高亮和自动补全
- 实时语法检查
- 创建进度展示
- 模板选择（可选）

#### 系统设置（SettingsView.vue）

- 配置项分组展示和编辑
- 实时保存配置更改
- 配置项验证
- 二次验证设置
- Shell 功能控制

#### 终端访问（TerminalView.vue）

- 交互式终端（xterm.js）
- 彩色输出支持
- 中文字符支持
- 终端大小调整
- 粘贴功能

#### 登录页面（LoginView.vue）

- 用户名密码登录
- 二次验证流程
- OTP 验证码输入
- WebAuthn 生物识别
- 记住登录状态

## 5. 类型系统设计

### 5.1 基础类型

```typescript
// 基础响应类型
interface BaseResponse<T = any> {
  code: number;
  msg: string;
  data: T;
}

// 容器状态类型
interface ContainerStatus {
  id: string;
  name: string;
  image: string;
  running: boolean;
  currentDigest: string;
  remoteDigest: string;
  status: "UpToDate" | "UpdateAvailable" | "Skipped" | "Error";
  skipped: boolean;
  skipReason: string;
  labels: Record<string, string>;
  lastCheckedAt: string;
  cpuPercent?: number;
  memoryPercent?: number;
  memoryUsage?: number;
  memoryLimit?: number;
}

// 镜像信息类型
interface ImageInfo {
  id: string;
  repoTags: string[];
  repoDigests: string[];
  size: number;
  created: number;
}

// Compose 项目类型
interface ComposeProject {
  name: string;
  path: string;
  status: "running" | "stopped" | "partial" | "error";
  services: ComposeService[];
  configFiles: string[];
}

interface ComposeService {
  name: string;
  status: string;
  image: string;
  ports: string[];
}

// 系统信息类型
interface SystemInfo {
  version: string;
  dockerVersion: string;
  isShellEnabled: boolean;
  isSecondaryVerificationEnabled: boolean;
}
```

### 5.2 二次验证类型

```typescript
// 二次验证状态
interface TwoFAStatus {
  enabled: boolean;
  isSetup: boolean;
  method: "otp" | "webauthn";
}

// 登录响应（二次验证）
interface LoginResponse {
  token?: string;
  needTwoFA?: boolean;
  isSetup?: boolean;
  method?: string;
  tempToken?: string;
  username?: string;
}

// OTP 设置响应
interface OTPSetupResponse {
  secret: string;
  qrCodeURL: string;
}

// WebAuthn 选项
interface WebAuthnOptions {
  publicKey: PublicKeyCredentialCreationOptions;
}
```

## 6. 状态管理设计

### 6.1 应用状态（useAppStore）

```typescript
interface AppState {
  loading: boolean;
  drawerVisible: boolean;
  systemInfo: SystemInfo | null;
  currentRoute: string;
}
```

### 6.2 认证状态（useAuthStore）

```typescript
interface AuthState {
  token: string;
  username: string;
  isLoggedIn: boolean;
  twoFARequired: boolean;
  twoFASetupRequired: boolean;
  tempToken: string;
  twoFAMethod: string;
}
```

### 6.3 容器状态（useContainerStore）

```typescript
interface ContainerState {
  containers: ContainerStatus[]
  loading: boolean
  updating: Set<string>
  selectedContainer: ContainerStatus | null
}

// 计算属性
computed: {
  updateableContainers: ContainerStatus[]
  runningContainers: ContainerStatus[]
  stoppedContainers: ContainerStatus[]
}
```

### 6.4 镜像状态（useImageStore）

```typescript
interface ImageState {
  images: ImageInfo[]
  loading: boolean
  deleting: Set<string>
}

// 计算属性
computed: {
  totalSize: number
  dangling Images: ImageInfo[]
}
```

### 6.5 Compose 状态（useComposeStore）

```typescript
interface ComposeState {
  projects: ComposeProject[];
  loading: boolean;
  selectedProject: ComposeProject | null;
  logs: string[];
}
```

## 7. API 接口设计

### 7.1 认证 API

```typescript
export const authApi = {
  login: (username: string, password: string) =>
    axios.post<BaseResponse<LoginResponse>>("/api/v1/auth/login", {
      username,
      password,
    }),

  logout: () => axios.post("/api/v1/auth/logout"),
};
```

### 7.2 二次验证 API

```typescript
export const twoFAApi = {
  getStatus: () => axios.get<BaseResponse<TwoFAStatus>>("/api/v1/2fa/status"),

  // OTP
  setupOTPInit: () =>
    axios.post<BaseResponse<OTPSetupResponse>>("/api/v1/2fa/setup/otp/init"),
  setupOTPVerify: (code: string, secret: string) =>
    axios.post("/api/v1/2fa/setup/otp/verify", { code, secret }),
  verifyOTP: (code: string) => axios.post("/api/v1/2fa/verify/otp", { code }),

  // WebAuthn
  setupWebAuthnBegin: () => axios.post("/api/v1/2fa/setup/webauthn/begin"),
  setupWebAuthnFinish: (sessionData: string, response: any) =>
    axios.post("/api/v1/2fa/setup/webauthn/finish", { sessionData, response }),
  verifyWebAuthnBegin: () => axios.post("/api/v1/2fa/verify/webauthn/begin"),
  verifyWebAuthnFinish: (sessionData: string, response: any) =>
    axios.post("/api/v1/2fa/verify/webauthn/finish", { sessionData, response }),

  disable: () => axios.post("/api/v1/2fa/disable"),
};
```

### 7.3 容器 API

```typescript
export const containerApi = {
  getContainers: () =>
    axios.get<BaseResponse<{ containers: ContainerStatus[] }>>(
      "/api/containers"
    ),

  updateContainer: (id: string, image?: string) =>
    axios.post(`/api/containers/${id}/update`, { image }),

  batchUpdate: () =>
    axios.post<
      BaseResponse<{ updated: string[]; failed: Record<string, string> }>
    >("/api/updates/run"),

  startContainer: (id: string) => axios.post(`/api/containers/${id}/start`),

  stopContainer: (id: string) => axios.post(`/api/containers/${id}/stop`),

  deleteContainer: (id: string) => axios.delete(`/api/containers/${id}`),
};
```

### 7.4 镜像 API

```typescript
export const imageApi = {
  getImages: () =>
    axios.get<BaseResponse<{ images: ImageInfo[] }>>("/api/images"),

  deleteImage: (ref: string, force: boolean = false) =>
    axios.delete("/api/images", { data: { ref, force } }),
};
```

### 7.5 Compose API

```typescript
export const composeApi = {
  getProjects: () =>
    axios.get<BaseResponse<{ projects: ComposeProject[] }>>("/api/compose"),

  start: (name: string, path: string) =>
    axios.post("/api/compose/start", { name, path }),

  stop: (name: string, path: string) =>
    axios.post("/api/compose/stop", { name, path }),

  restart: (name: string, path: string) =>
    axios.post("/api/compose/restart", { name, path }),

  delete: (name: string, path: string) =>
    axios.delete("/api/compose/delete", { data: { name, path } }),

  create: (name: string, path: string, content: string) =>
    axios.post("/api/compose/create", { name, path, content }),
};
```

## 8. 组件设计原则

### 8.1 组件复用性

- 单一职责：每个组件只负责一个功能
- Props 传递：通过 props 传递数据
- 事件发射：通过 emit 通知父组件
- 插槽支持：提供插槽扩展性

### 8.2 状态管理

- 全局状态：使用 Pinia store
- 组件状态：使用 ref/reactive
- 计算属性：使用 computed
- 副作用：使用 watch/watchEffect

### 8.3 性能优化

- 虚拟滚动：大列表使用虚拟滚动
- 懒加载：路由懒加载
- 防抖节流：频繁操作使用防抖节流
- 缓存：合理使用缓存

## 9. 响应式设计

### 9.1 断点定义

```typescript
const breakpoints = {
  mobile: 640, // < 640px
  tablet: 768, // 640px - 768px
  laptop: 1024, // 768px - 1024px
  desktop: 1280, // >= 1024px
};
```

### 9.2 布局适配

- **移动端（< 768px）**:

  - 单列布局
  - 抽屉式侧边菜单
  - 卡片堆叠排列
  - 悬浮操作按钮

- **平板端（768px - 1024px）**:

  - 两列布局
  - 固定侧边菜单
  - 卡片网格排列

- **桌面端（> 1024px）**:
  - 三列或更多列布局
  - 完整侧边菜单
  - 密集型卡片排列

## 10. 路由设计

```typescript
const routes = [
  { path: "/", component: HomeView },
  { path: "/login", component: LoginView },
  { path: "/containers", component: ContainersView },
  { path: "/images", component: ImagesView },
  { path: "/compose", component: ComposeView },
  { path: "/compose/create", component: ComposeCreateView },
  { path: "/logs/:id", component: LogsPageView },
  { path: "/terminal", component: TerminalView },
  { path: "/settings", component: SettingsView },
];
```

### 路由守卫

- 认证检查：未登录跳转登录页
- 权限检查：Shell 功能权限检查
- 二次验证：未完成二次验证的处理

## 11. 国际化支持（可选）

预留国际化支持，使用 vue-i18n：

```typescript
const messages = {
  "zh-CN": {
    container: {
      start: "启动",
      stop: "停止",
      update: "更新",
      delete: "删除",
    },
  },
  "en-US": {
    container: {
      start: "Start",
      stop: "Stop",
      update: "Update",
      delete: "Delete",
    },
  },
};
```

## 12. 主题系统

支持亮色和暗色主题：

```typescript
const themes = {
  light: {
    primary: "#18a058",
    background: "#ffffff",
    text: "#333333",
  },
  dark: {
    primary: "#63e2b7",
    background: "#1a1a1a",
    text: "#e0e0e0",
  },
};
```

## 13. 安全考虑

### 13.1 认证

- Token 存储在 localStorage
- 每次请求携带 Token
- Token 过期自动跳转登录
- 支持二次验证增强安全

### 13.2 输入验证

- 表单验证
- XSS 防护
- CSRF 防护

### 13.3 敏感信息

- 密码不明文显示
- API Token 加密存储
- OTP 密钥安全处理

## 14. 开发计划

### 阶段一：基础架构 ✅

1. 完善项目结构
2. 配置类型定义和 API 接口
3. 设置 Pinia 状态管理
4. 实现基础布局组件

### 阶段二：核心功能 ✅

1. 实现容器列表页面
2. 实现镜像列表页面
3. 实现基础的 CRUD 操作
4. 添加状态管理和数据流

### 阶段三：高级功能 ✅

1. 实现设置页面
2. 添加批量操作功能
3. 实现响应式设计
4. 添加错误处理和用户反馈
5. Compose 项目管理
6. 二次验证功能
7. 终端访问功能

### 阶段四：优化和测试

1. 性能优化
2. 用户体验优化
3. 错误边界处理
4. 测试和调试
5. 国际化支持（可选）

## 15. 设计原则

1. **用户体验优先**：简洁直观的界面，流畅的操作体验
2. **响应式设计**：完美支持移动端和桌面端
3. **性能优化**：快速加载，流畅交互
4. **可维护性**：清晰的代码结构，完善的类型定义
5. **可扩展性**：模块化设计，易于添加新功能
6. **安全性**：认证授权，二次验证，数据加密

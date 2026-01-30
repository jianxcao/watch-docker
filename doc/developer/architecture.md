# Watch Docker 架构设计

本文档描述 Watch Docker 的整体架构设计和核心组件。

## 1. 项目概述

### 1.1 背景与目标

**背景**: 在 Docker 主机上自动监控并更新运行中的容器，借鉴 watchtower 思路，但提供更强的可观测性、策略控制与 API。

**目标**:

- 列出当前主机的所有容器与镜像信息
- 周期性检测镜像是否存在更新（基于远端 manifest digest 对比）
- 支持按间隔或 cron 定时执行自动更新
- 支持多种跳过策略：按 label、本地构建镜像、固定版本（digest 或语义化版本）
- 提供 HTTP API 供查询与手动触发更新
- 支持 Docker Compose 项目管理
- 提供 Shell 终端访问功能
- 支持二次验证（OTP/WebAuthn）

**非目标（当前阶段）**:

- 跨主机编排与滚动发布（Kubernetes/Swarm）
- 私有镜像仓库的证书管理与密钥下发（依赖外部 Secret 注入）

### 1.2 术语定义

- **容器引用（image ref）**: registry/namespace/repo:tag 或 repo@sha256:digest
- **本地构建镜像**: `RepoDigests` 为空，未推送至远端 registry 的镜像
- **固定版本**: 通过 digest 固定，或标签为严格语义化版本（如 1.2.3 / v1.2.3）
- **浮动标签**: 如 latest、stable、main 等可能随时间漂移的标签

## 2. 总体架构

### 2.1 系统架构图

```
┌─────────────────────────────────────────────────────────┐
│                    Web 前端 (Vue 3)                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │ 容器管理 │  │ 镜像管理 │  │ Compose │  │ 系统设置 │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP/WebSocket
┌────────────────────┴────────────────────────────────────┐
│              HTTP API Server (Gin)                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │ 认证模块 │  │ 容器API  │  │ Compose │  │ 二次验证 │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────┐
│              核心业务逻辑层                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │ Scanner  │  │ Updater  │  │ Scheduler│  │ Policy  │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────┴────────────────────────────────────┐
│              基础设施层                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │Docker SDK│  │ Registry │  │ Compose │  │  WebSocket│ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
              ┌──────┴──────┐
              │ Docker Engine│
              └─────────────┘
```

### 2.2 核心组件

#### 后端组件

- **DockerClient**: 通过 Docker SDK 获取容器/镜像信息，执行拉取、创建、启动、删除等操作
- **RegistryClient**: 通过 Registry V2 API（Resty）获取远端 manifest 与 digest
- **Scanner**: 周期性扫描容器，评估是否有更新，并汇总状态
- **PolicyEngine**: 应用跳过与强制策略（label、本地构建、固定版本识别）
- **Updater**: 执行拉取新镜像、按原配置重建容器与回滚
- **Scheduler**: 按 interval 或 cron 触发扫描/更新作业
- **StateCache**: 进程内缓存镜像远端 digest、扫描结果、上次更新时间等
- **HTTP API**: Gin 提供查询与操作接口
- **Config**: Viper + envconfig 读取 YAML/ENV 配置
- **Logger**: Zap 结构化日志
- **ComposeCLI**: Docker Compose CLI 命令封装
- **WSStream**: WebSocket 流管理（容器状态、日志、Shell）
- **TwoFA**: 二次验证管理，支持 OTP 和 WebAuthn 验证方式
- **Auth**: JWT Token 管理，支持临时 Token 和完整 Token

#### 前端组件

- **Vue 3**: 前端框架，使用 Composition API
- **TypeScript**: 类型安全的 JavaScript
- **Vite**: 构建工具和开发服务器
- **Naive UI**: 主要组件库
- **Pinia**: 状态管理
- **Vue Router**: 路由管理
- **Axios**: HTTP 请求库
- **WebSocket**: 实时通信

### 2.3 数据流

1. Scheduler 触发扫描
2. Scanner 调用 DockerClient 列出容器，解析镜像引用
3. RegistryClient 获取对应远端 digest（带缓存与去重）
4. PolicyEngine 评估：跳过或标记需要更新
5. 结果写入 StateCache，API 对外提供
6. 若配置自动更新，Scheduler 触发 Updater 执行更新

## 3. 后端架构

### 3.1 项目结构

```
backend/
├── cmd/
│   └── watch-docker/           # main 入口
├── internal/
│   ├── api/                    # Gin 路由与 handler
│   ├── auth/                   # JWT Token 管理
│   ├── twofa/                  # 二次验证模块
│   ├── config/                 # 配置加载与校验
│   ├── conf/                   # 环境变量配置
│   ├── dockercli/              # Docker SDK 封装
│   ├── composecli/             # Docker Compose CLI 封装
│   ├── registry/               # Registry V2 客户端
│   ├── scanner/                # 扫描与状态评估
│   ├── updater/                # 更新器与回滚
│   ├── policy/                 # 策略判定
│   ├── scheduler/              # 调度器
│   ├── wsstream/               # WebSocket 流管理
│   └── logging/                # zap 初始化
└── pkg/                        # 对外可复用库
```

### 3.2 核心模块

#### 容器管理

**发现策略**: 默认仅包含运行中容器，可配置包含已停止容器

**采集字段**:
- 容器: ID、Name、Image（原始引用）、RepoTags、RepoDigests、Labels、State、Created、Compose 相关 labels
- 平台: GOOS/GOARCH/variant（用于选择 manifest list 子项）

**更新检查**:
- 对比原则: 本地镜像 `RepoDigests` 与远端 manifest `Docker-Content-Digest` 比较
- 远端 digest 获取: 解析 `registry/namespace/repo:tag`，先鉴权，再 `HEAD/GET` manifest
- 判定规则: 浮动标签且远端 digest ≠ 本地 → `UpdateAvailable`

#### 策略引擎

**Label 策略**:
- `watchdocker.skip=true` → 跳过
- `watchdocker.force=true` → 即便固定版本也可更新

**镜像来源**:
- 无 `RepoDigests` → 本地构建，跳过

**固定版本**:
- `image@sha256:...` 或严格 semver 标签 → 跳过（可配置）

#### 更新执行

**流程**:
1. `ImagePull` 拉取目标镜像
2. `ContainerInspect` 读取原容器配置
3. 优雅停止原容器，重命名旧容器
4. 以相同参数创建并启动新容器
5. 新容器健康后删除旧容器（可配置保留）

### 3.3 Docker Compose 管理

**功能特性**:
- 项目发现: 自动扫描指定目录，识别 docker-compose.yml 文件
- 项目管理: 启动、停止、重启、删除 Compose 项目
- 状态监控: 实时查看项目及其服务的运行状态
- 日志查看: 通过 WebSocket 实时查看项目日志

**实现方式**:
- 使用 Docker Compose CLI 命令封装
- 通过 `docker compose` 命令执行操作
- 解析命令输出获取状态信息

### 3.4 二次验证

**验证方式**:
- **OTP**: 基于 TOTP（RFC 6238）协议，使用 `github.com/pquerna/otp` 库
- **WebAuthn**: 基于 FIDO2/WebAuthn 标准，使用 `github.com/go-webauthn/webauthn` 库

**认证流程**:
1. 用户输入用户名密码 → 验证通过
2. 系统检查 `IS_SECONDARY_VERIFICATION` 环境变量
3. 查询用户是否已设置二次验证
4. 若未设置 → 生成临时 Token（15 分钟有效期）
5. 用户完成设置并验证成功
6. 系统升级临时 Token 为完整 Token（24 小时有效期）

## 4. 前端架构

### 4.1 项目结构

```
frontend/src/
├── components/           # 公共组件
│   ├── LayoutView.vue         # 主布局组件
│   ├── ContainerCard.vue      # 容器卡片
│   ├── ImageCard.vue          # 镜像卡片
│   ├── ComposeCard.vue        # Compose 项目卡片
│   └── ...
├── pages/               # 页面组件
│   ├── HomeView.vue           # 首页
│   ├── ContainersView.vue     # 容器列表
│   ├── ImagesView.vue         # 镜像列表
│   ├── ComposeView.vue        # Compose 项目列表
│   └── ...
├── hooks/               # Vue 3 Composition API Hooks
│   ├── useContainer.ts        # 容器操作相关hooks
│   ├── useImage.ts            # 镜像操作相关hooks
│   └── ...
├── store/               # Pinia 状态管理
│   ├── app.ts           # 应用全局状态
│   ├── auth.ts          # 认证状态
│   ├── container.ts     # 容器状态管理
│   └── ...
├── common/              # 公共工具
│   ├── api.ts           # API 接口方法
│   ├── types.ts         # TypeScript 类型定义
│   └── utils.ts         # 工具函数
└── router/              # 路由配置
```

### 4.2 核心功能设计

#### 布局设计

**PC 端（≥1024px）**:
- 左侧固定导航菜单
- 右侧内容区域
- 顶部面包屑导航

**移动端（<1024px）**:
- 顶部标题栏
- 左侧抽屉菜单
- 内容区域

#### 状态管理

使用 Pinia 进行集中式状态管理：

- **应用状态**: 加载状态、系统信息、当前路由
- **认证状态**: Token、用户名、登录状态、二次验证状态
- **容器状态**: 容器列表、加载状态、更新状态
- **镜像状态**: 镜像列表、加载状态
- **Compose 状态**: 项目列表、加载状态、日志

#### API 通信

- 使用 Axios 进行 HTTP 请求
- 请求拦截器自动添加 Token
- 响应拦截器统一处理错误
- WebSocket 用于实时数据推送

## 5. 技术栈

### 后端技术栈

- **语言**: Go 1.21+
- **Web 框架**: `github.com/gin-gonic/gin`
- **日志**: `go.uber.org/zap`
- **配置**: `github.com/spf13/viper` + `github.com/kelseyhightower/envconfig`
- **HTTP 客户端**: `github.com/go-resty/resty/v2`
- **Docker SDK**: `github.com/docker/docker`
- **定时调度**: `github.com/robfig/cron/v3`
- **二次验证 OTP**: `github.com/pquerna/otp`
- **二次验证 WebAuthn**: `github.com/go-webauthn/webauthn`

### 前端技术栈

- **框架**: Vue 3 (Composition API)
- **语言**: TypeScript
- **构建工具**: Vite
- **UI 框架**: Naive UI
- **状态管理**: Pinia
- **路由**: Vue Router
- **HTTP 客户端**: Axios
- **样式**: UnoCSS + Less
- **WebAuthn**: @simplewebauthn/browser
- **终端**: xterm.js

## 6. 安全设计

### 认证与授权

- **基础认证**: 用户名密码验证
- **Token 机制**: JWT Token，支持临时 Token 和完整 Token
- **二次验证**: OTP/WebAuthn 双重验证
- **中间件**: 认证中间件保护所有 API 端点

### 安全考虑

- 访问 Docker socket（建议最小权限挂载）
- Registry 凭据通过环境变量/Secret 注入，避免明文落盘
- 进程以非 root 运行（如可能）
- 二次验证增强账户安全
- Shell 功能需显式开启并强制身份验证

## 7. 部署架构

### 部署方式

1. **原生安装包**: 单文件二进制，嵌入前端资源
2. **Docker 镜像**: 轻量级二进制 + 外部静态目录
3. **开发环境**: 前后端分离，热重载支持

### 构建策略

- **原生构建**: 使用 Go embed 嵌入前端资源
- **Docker 构建**: 使用构建标签，不嵌入资源，使用外部目录
- **条件编译**: 通过 Go build tags 实现不同部署场景

## 8. 性能优化

### 后端优化

- 结果缓存（TTL），同一镜像去重请求
- 网络错误退避重试
- 并发控制：受限 goroutine 池
- Singleflight 去重请求

### 前端优化

- 虚拟滚动：大列表使用虚拟滚动
- 懒加载：路由懒加载
- 防抖节流：频繁操作使用防抖节流
- 缓存：合理使用缓存

## 9. 可观测性

### 日志

- Zap 结构化日志字段：`component`, `container`, `image`, `digest`, `action`, `durationMs`
- 日志级别：debug, info, warn, error

### 监控

- 健康检查端点：`/healthz`, `/readyz`
- WebSocket 实时推送容器状态
- 扫描和更新操作记录

## 10. 扩展性设计

### 插件化

- 策略引擎可扩展
- Registry 客户端可扩展
- 通知系统可扩展

### API 设计

- RESTful API 设计
- 统一响应格式
- 版本化 API（`/api/v1/`）

## 11. 未来规划

- Kubernetes 支持
- 多主机管理
- 更丰富的通知渠道
- 性能指标收集
- 审计日志

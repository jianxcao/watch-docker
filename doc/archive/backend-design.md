# Watch Docker 后端架构设计

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

### 2.1 核心组件

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

### 2.2 数据流

1. Scheduler 触发扫描
2. Scanner 调用 DockerClient 列出容器，解析镜像引用
3. RegistryClient 获取对应远端 digest（带缓存与去重）
4. PolicyEngine 评估：跳过或标记需要更新
5. 结果写入 StateCache，API 对外提供
6. 若配置自动更新，Scheduler 触发 Updater 执行更新

## 3. 容器管理

### 3.1 容器发现与信息模型

**发现策略**: 默认仅包含运行中容器，可配置包含已停止容器

**采集字段**:

- 容器: ID、Name、Image（原始引用）、RepoTags、RepoDigests、Labels、State、Created、Compose 相关 labels
- 平台: GOOS/GOARCH/variant（用于选择 manifest list 子项）

**状态模型（对外返回）**:

```json
{
  "id": "ab12c3",
  "name": "web",
  "image": "nginx:1.25",
  "running": true,
  "currentDigest": "sha256:aaa...",
  "remoteDigest": "sha256:bbb...",
  "status": "UpToDate | UpdateAvailable | Skipped | Error",
  "skipped": true,
  "skipReason": "pinned digest",
  "labels": { "watchdocker.skip": "true" },
  "lastCheckedAt": "2025-09-19T09:30:00Z"
}
```

### 3.2 更新检查

**对比原则**: 本地镜像 `RepoDigests` 与远端 manifest `Docker-Content-Digest` 比较

**远端 digest 获取**:

- 解析 `registry/namespace/repo:tag`
- 先鉴权（Docker Hub/ghcr 等支持 Bearer Token），再 `HEAD/GET` manifest（Accept: OCI/Docker manifest / manifest list）
- 如为 manifest list，按宿主平台选择具体 manifest，取其 digest

**判定规则**:

- 浮动标签且远端 digest ≠ 本地 → `UpdateAvailable`
- 镜像以 `@sha256:` 引用 → 视为固定版本，`Skipped`（除非强制）
- 语义化版本标签（1.2.3 / v1.2.3）且启用 `skipSemverPinned` → `Skipped`
- 本地构建（无 RepoDigests） → `Skipped`

**优化**:

- 结果缓存（TTL），同一镜像去重请求
- 网络错误退避重试

### 3.3 跳过与强制策略（PolicyEngine）

**Label 策略**:

- `watchdocker.skip=true` → 跳过
- `watchdocker.force=true` → 即便固定版本也可更新
- 支持 `onlyLabels` 与 `excludeLabels` 组合过滤

**镜像来源**:

- 无 `RepoDigests` → 本地构建，跳过

**固定版本**:

- `image@sha256:...` 或严格 semver 标签 → 跳过（可配置）

### 3.4 更新执行（Updater）

**流程**:

1. `ImagePull` 拉取目标镜像（如已存在不同 digest）
2. `ContainerInspect` 读取原容器配置（Config/HostConfig/NetworkingConfig）
3. 优雅停止原容器（带超时），重命名旧容器
4. 以相同参数创建并启动新容器（复用名称）
5. 新容器健康后删除旧容器（可配置保留）

**注意点**:

- 端口、卷、网络、环境变量、重启策略与日志驱动需完整保留
- 对含 `com.docker.compose.*` labels 的容器，默认仅提示不自动更新，避免与 Compose 冲突（可配置允许）
- 并发与镜像分组串行：同一镜像的多个容器更新需串行，避免重复拉取

**失败与回滚**:

- 新容器启动失败立即回滚激活旧容器并记录错误

## 4. Docker Compose 管理

### 4.1 功能特性

- **项目发现**: 自动扫描指定目录，识别 docker-compose.yml 或 compose.yml 文件
- **项目管理**: 启动、停止、重启、删除 Compose 项目
- **项目创建**: 通过 Web 界面创建新项目（YAML 编辑器）
- **状态监控**: 实时查看项目及其服务的运行状态
- **日志查看**: 通过 WebSocket 实时查看项目日志
- **服务详情**: 查看项目中的服务、网络、卷等详细信息

### 4.2 项目状态

- **running**: 所有服务都在运行
- **stopped**: 所有服务都已停止
- **partial**: 部分服务在运行
- **error**: 项目存在错误

### 4.3 实现方式

- 使用 Docker Compose CLI 命令封装
- 通过 `docker compose` 命令执行操作
- 解析命令输出获取状态信息
- WebSocket 流式传输日志和进度

## 5. Shell 终端访问

### 5.1 功能特性

- **交互式终端**: 完整的 PTY (伪终端) 支持
- **彩色输出**: 支持 ANSI 颜色和格式化输出
- **实时通信**: 基于 WebSocket 的低延迟通信
- **中文支持**: 支持中文字符显示（UTF-8）
- **会话管理**: 自动处理终端大小调整和会话超时

### 5.2 安全控制

- **显式开启**: 必须设置环境变量 `IS_OPEN_DOCKER_SHELL=true`
- **身份验证**: 必须配置用户名密码
- **权限控制**: 仅认证用户可访问
- **日志记录**: 记录所有 Shell 访问和命令执行

### 5.3 技术实现

- **默认 Shell**: 使用系统环境变量 `$SHELL`，如未设置则使用 `/bin/sh`
- **终端类型**: `xterm-256color`
- **字符编码**: `UTF-8 (zh_CN.UTF-8)`
- **心跳检测**: 30 秒
- **会话超时**: 90 秒无活动后断开

## 6. 二次验证（Two-Factor Authentication）

### 6.1 概述

**目标**: 为 Watch Docker Web 界面提供额外的安全保护层，支持 OTP 和 WebAuthn 两种主流验证方式

**场景**: 在用户名密码验证通过后，要求用户完成第二次验证才能获得完整访问权限

### 6.2 验证方式

**OTP（一次性密码）**

- 基于 TOTP（RFC 6238）协议
- 使用 `github.com/pquerna/otp` 库实现
- 生成 Base32 编码的密钥和二维码 URL
- 30 秒刷新的 6 位数字验证码
- 支持 Google Authenticator、Authy 等标准应用

**WebAuthn（生物验证）**

- 基于 FIDO2/WebAuthn 标准
- 使用 `github.com/go-webauthn/webauthn` 库实现
- 支持指纹、Face ID、Windows Hello、硬件密钥等
- 公钥加密，私钥存储在用户设备中
- 多域名凭据支持（每个域名独立注册）
- 防钓鱼攻击，安全性更高

### 6.3 认证流程

**首次设置流程（启用二次验证时）**:

1. 用户输入用户名密码 → 验证通过
2. 系统检查 `IS_SECONDARY_VERIFICATION` 环境变量
3. 查询用户是否已设置二次验证
4. 若未设置 → 生成临时 Token（15 分钟有效期）
5. 前端引导用户选择验证方式（OTP 或 WebAuthn）
6. 用户完成设置并验证成功
7. 系统升级临时 Token 为完整 Token（24 小时有效期）
8. 保存用户配置到 `config.yaml`

**日常登录流程（已设置二次验证）**:

1. 用户输入用户名密码 → 验证通过
2. 系统检查用户已设置二次验证
3. 生成临时 Token，返回验证方式信息
4. 前端展示对应验证界面（OTP 输入框或 WebAuthn 按钮）
5. 用户完成二次验证
6. 系统升级临时 Token 为完整 Token
7. 用户获得完整访问权限

### 6.4 Token 机制

**临时 Token（Temp Token）**:

- 仅在通过用户名密码验证后颁发
- 有效期：15 分钟
- 权限：仅可访问二次验证相关 API（`/api/v1/2fa/*`）
- JWT Claims 包含：`isTempToken: true`, `twoFAVerified: false`
- 用于完成二次验证设置或验证过程

**完整 Token（Full Token）**:

- 在完成二次验证后颁发
- 有效期：24 小时
- 权限：可访问所有 API 端点
- JWT Claims 包含：`isTempToken: false`, `twoFAVerified: true`
- 正常的访问凭证

**中间件层次**:

- `AuthMiddleware`: 验证 Token 有效性，所有需要认证的端点使用
- `TempTokenMiddleware`: 允许临时 Token，仅二次验证 API 使用
- 其他 API 仅接受完整 Token

### 6.5 安全考虑

**密钥管理**:

- OTP 密钥使用 Base32 编码存储
- WebAuthn 私钥存储在用户设备，服务器仅存储公钥
- 配置文件应使用文件系统权限保护（0600）
- 建议定期备份配置文件

**域名验证**（WebAuthn）:

- 支持域名白名单（`TWOFA_ALLOWED_DOMAINS` 环境变量）
- 从请求头提取 RPID（支持反向代理场景）
- 验证凭据的 RPID 与当前域名匹配
- 防止跨域攻击

**会话管理**:

- 临时 Token 短期有效（15 分钟），限制时间窗口
- WebAuthn Session 数据临时存储，验证后立即清理
- 支持强制注销（禁用二次验证时清除所有凭据）

**错误处理**:

- 验证失败不泄露具体原因（防止信息探测）
- 记录失败尝试日志，便于审计
- 支持时间偏移容错（OTP）

## 7. 调度（Scheduler）

### 7.1 调度模式

- **interval**: 周期扫描/更新
- **cron**: 指定时刻自动更新（支持时区与秒级精度）

### 7.2 任务类型

- **扫描任务**: 只更新状态
- **更新任务**: 对 `UpdateAvailable` 且通过策略的容器执行更新

### 7.3 并发控制

- 全局并发度限制 + 每镜像桶内串行

## 8. API 设计（Gin）

### 8.1 健康检查

- `GET /healthz`: 健康检查
- `GET /readyz`: 就绪检查

### 8.2 容器管理

- `GET /api/containers`: 列出容器及状态（含 running）
- `POST /api/containers/:id/update`: 对单容器触发更新
- `POST /api/updates/run`: 批量更新
- `POST /api/containers/:id/stop`: 停止容器
- `POST /api/containers/:id/start`: 启动容器
- `DELETE /api/containers/:id`: 删除容器

### 8.3 镜像管理

- `GET /api/images`: 获取镜像列表
- `DELETE /api/images`: 删除镜像

### 8.4 Compose 管理

- `GET /api/compose`: 获取 Compose 项目列表
- `POST /api/compose/start`: 启动 Compose 项目
- `POST /api/compose/stop`: 停止 Compose 项目
- `POST /api/compose/restart`: 重启 Compose 项目
- `DELETE /api/compose/delete`: 删除 Compose 项目
- `POST /api/compose/create`: 创建 Compose 项目
- `GET /api/compose/logs/ws`: Compose 日志 WebSocket

### 8.5 终端访问

- `GET /api/shell/ws`: Shell 终端 WebSocket

### 8.6 二次验证

**二次验证相关端点**（使用 `TempTokenMiddleware`）:

- `GET /api/v1/2fa/status`: 获取用户二次验证状态
- `POST /api/v1/2fa/setup/otp/init`: 初始化 OTP（生成密钥和二维码）
- `POST /api/v1/2fa/setup/otp/verify`: 验证并启用 OTP
- `POST /api/v1/2fa/setup/webauthn/begin`: 开始 WebAuthn 注册
- `POST /api/v1/2fa/setup/webauthn/finish`: 完成 WebAuthn 注册
- `POST /api/v1/2fa/verify/otp`: 验证 OTP 登录
- `POST /api/v1/2fa/verify/webauthn/begin`: 开始 WebAuthn 验证
- `POST /api/v1/2fa/verify/webauthn/finish`: 完成 WebAuthn 验证
- `POST /api/v1/2fa/disable`: 禁用二次验证（需完整 Token）

**登录端点调整**:

- `POST /api/v1/auth/login`: 返回格式扩展，支持二次验证流程

### 8.7 统一响应格式

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "containers": [...],
    "total": 10
  }
}
```

**错误码**:

- `CodeImageRequired=40001`: 镜像参数必需
- `CodeScanFailed=50001`: 扫描失败
- `CodeUpdateFailed=50002`: 更新失败
- `CodeDockerError=50003`: Docker 错误
- `CodeRegistryError=50004`: Registry 错误
- `CodeUnauthorized=401`: 未授权
- `CodeInternalError=500`: 内部错误

## 9. 配置（YAML + ENV）

### 9.1 配置文件示例

```yaml
server:
  addr: ":8080"

docker:
  host: "unix:///var/run/docker.sock"
  includeStopped: false

scan:
  interval: "10m" # 扫描间隔
  initialScanOnStart: true # 启动时立即扫描
  concurrency: 3 # 并发数
  cacheTTL: "5m" # 缓存时间

update:
  enabled: true # 启用自动更新
  autoUpdateCron: "0 3 * * *" # 每天凌晨3点自动更新
  allowComposeUpdate: false # 是否允许更新 Compose 容器
  removeOldContainer: true # 更新后删除旧容器

policy:
  skipLabels: ["watchdocker.skip=true"] # 跳过标签
  skipLocalBuild: true # 跳过本地构建
  skipPinnedDigest: true # 跳过固定 digest
  skipSemverPinned: true # 跳过语义化版本

registry:
  auth:
    - host: "registry-1.docker.io"
      username: ""
      password: ""
    - host: "ghcr.io"
      username: ""
      password: ""

logging:
  level: "info"

twofa:
  users:
    admin:
      method: "otp" # 或 "webauthn"
      otpSecret: "BASE32_ENCODED_SECRET"
      webauthnCredentials:
        - credential: { ... }
          rpid: "example.com"
      isSetup: true
```

### 9.2 环境变量

ENV 覆盖（前缀 `WATCH_`），常见示例:

- `WATCH_SERVER_ADDR=:8080`
- `WATCH_SCAN_INTERVAL=10m`
- `WATCH_UPDATE_ENABLED=true`
- `WATCH_REGISTRY_AUTH_0_HOST=ghcr.io`
- `WATCH_REGISTRY_AUTH_0_USERNAME=xxx`
- `WATCH_REGISTRY_AUTH_0_PASSWORD=yyy`

**二次验证相关**:

- `IS_SECONDARY_VERIFICATION=true`: 启用二次验证功能
- `TWOFA_ALLOWED_DOMAINS=example.com,app.example.com`: WebAuthn 域名白名单

**其他配置**:

- `USER_NAME=admin`: 登录用户名
- `USER_PASSWORD=admin`: 登录密码
- `IS_OPEN_DOCKER_SHELL=false`: 是否开启 Shell 功能
- `APP_PATH=/volume1/docker`: Docker Compose 项目目录

## 10. 可观测性与日志

### 10.1 日志

- Zap 结构化日志字段：`component`, `container`, `image`, `digest`, `action`, `durationMs`
- 日志级别：debug, info, warn, error

### 10.2 指标（可选）

- 扫描耗时
- 成功/失败更新计数
- Registry 请求时延

## 11. 安全与部署

### 11.1 安全考虑

- 访问 Docker socket（建议最小权限挂载，更新需写权限）
- Registry 凭据通过环境变量/Secret 注入，避免明文落盘
- 进程以非 root 运行（如可能），限制网络超时、设置重试上限
- 二次验证增强账户安全
- Shell 功能需显式开启并强制身份验证

### 11.2 部署建议

- 使用 Docker Compose 部署
- 持久化配置目录
- 设置强密码
- 配置域名白名单（WebAuthn）
- 限制网络访问
- 定期备份配置

## 12. 里程碑

- M1: 项目骨架、配置/日志、健康检查
- M2: 容器发现与本地信息返回
- M3: Registry digest 获取与比对
- M4: 跳过策略完善
- M5: 更新器（重建与回滚）
- M6: 调度器与自动更新
- M7: API 完整化
- M8: 稳定性与文档完善
- M9: 二次验证功能（OTP + WebAuthn）
- M10: Docker Compose 支持
- M11: Shell 终端访问

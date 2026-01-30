# Watch Docker 后端技术实现

## 1. 技术栈

- **语言**: Go 1.21+
- **Web 框架**: `github.com/gin-gonic/gin`
- **日志**: `go.uber.org/zap`
- **配置**: `github.com/spf13/viper` + `github.com/kelseyhightower/envconfig`
- **HTTP 客户端**: `github.com/go-resty/resty/v2`
- **Docker SDK**: `github.com/docker/docker`
- **定时调度**: `github.com/robfig/cron/v3`
- **语义化版本**: `github.com/blang/semver/v4`
- **二次验证 OTP**: `github.com/pquerna/otp`
- **二次验证 WebAuthn**: `github.com/go-webauthn/webauthn`

## 2. 项目结构

```
.
├── cmd/
│   └── watch-docker/           # main 入口
├── internal/
│   ├── api/                    # Gin 路由与 handler
│   ├── auth/                   # JWT Token 管理（临时/完整 Token）
│   ├── twofa/                  # 二次验证模块（OTP/WebAuthn）
│   ├── config/                 # 配置加载与校验
│   ├── conf/                   # 环境变量配置
│   ├── dockercli/              # Docker SDK 封装
│   ├── composecli/             # Docker Compose CLI 封装
│   ├── registry/               # Registry V2 客户端
│   ├── scanner/                # 扫描与状态评估
│   ├── updater/                # 更新器与回滚
│   ├── policy/                 # 策略判定
│   ├── scheduler/              # 调度器（interval/cron）
│   ├── wsstream/               # WebSocket 流管理
│   └── logging/                # zap 初始化
├── pkg/                        # 对外可复用库（如有）
├── doc/                        # 设计与实现文档
└── go.mod / go.sum
```

## 3. 核心模块实现

### 3.1 配置模块（internal/config）

#### 配置结构

```go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Docker   DockerConfig   `mapstructure:"docker"`
    Scan     ScanConfig     `mapstructure:"scan"`
    Update   UpdateConfig   `mapstructure:"update"`
    Policy   PolicyConfig   `mapstructure:"policy"`
    Registry RegistryConfig `mapstructure:"registry"`
    Logging  LoggingConfig  `mapstructure:"logging"`
    TwoFA    TwoFAConfig    `mapstructure:"twofa"`
}

type ServerConfig struct {
    Addr string `mapstructure:"addr" envconfig:"SERVER_ADDR" default:":8080"`
}

type DockerConfig struct {
    Host           string `mapstructure:"host" envconfig:"DOCKER_HOST"`
    IncludeStopped bool   `mapstructure:"includeStopped" envconfig:"DOCKER_INCLUDE_STOPPED"`
}

type TwoFAConfig struct {
    Users map[string]TwoFAUserConfig `mapstructure:"users"`
}

type TwoFAUserConfig struct {
    Method              string                    `mapstructure:"method"`
    OTPSecret           string                    `mapstructure:"otpSecret"`
    WebAuthnCredentials []WebAuthnCredentialData  `mapstructure:"webauthnCredentials"`
    IsSetup             bool                      `mapstructure:"isSetup"`
}
```

#### 配置加载策略

- 使用 Viper 读取 `config.yaml`（支持多个默认路径），ENV 前缀 `WATCH_`
- 使用 envconfig 将 ENV 映射至结构体（覆盖 Viper 值）
- 提供 `Validate()` 校验（端口格式、并发度、策略冲突等）

### 3.2 Docker 客户端封装（internal/dockercli）

#### 关键操作

- **列出容器**: `ContainerList(ctx, opts)`
- **检查容器**: `ContainerInspect(ctx, id)`
- **拉取镜像**: `ImagePull(ctx, ref, types.ImagePullOptions)`
- **容器操作**: `ContainerCreate/Start/Stop/Remove`

#### 容器重建流程

1. 提取原容器 `Config/HostConfig/NetworkingConfig`
2. 停止并重命名旧容器
3. 创建新容器复用名称
4. 启动新容器
5. 可选删除旧容器

### 3.3 Registry 客户端（internal/registry）

#### 实现方式

使用 Resty HTTP 客户端：

- Docker Hub/ghcr 等通用 Registry V2：`/v2/<repo>/manifests/<tag>` `HEAD/GET`
- Accept 头部包含：
  - Docker manifest list: `application/vnd.docker.distribution.manifest.list.v2+json`
  - Docker manifest: `application/vnd.docker.distribution.manifest.v2+json`
  - OCI index/manifest: `application/vnd.oci.image.index.v1+json`, `application/vnd.oci.image.manifest.v1+json`
- 从响应头 `Docker-Content-Digest` 读取 digest

#### 鉴权机制

- 处理 401 响应中的 `WWW-Authenticate`
- 请求 token endpoint 获取 Bearer Token（scope=repository:repo:pull）
- 支持 per-registry 基本凭据（ENV 注入）

#### Manifest list 处理

根据本机平台选择合适子 manifest（os/architecture/variant）

#### 优化策略

- **缓存**: `map[imageRef]digest` + TTL；错误也做短 TTL 缓存避免雪崩
- **去重**: `singleflight` 合并同一镜像并发请求

### 3.4 策略引擎（internal/policy）

#### 输入输出

- **输入**: 容器信息、镜像解析结果、远端 digest、本地 RepoDigests、配置
- **输出**: `Decision{Skipped bool, Reason string, Force bool}`

#### 规则实现

- **Label 匹配**: `watchdocker.skip=true` → skip；`watchdocker.force=true` → force
- **本地构建**: `len(RepoDigests)==0` → skip（可关）
- **固定版本**: `@sha256:` 或严格 semver → skip（可关）
- **浮动标签**: 名单外的 tag 可选择不检查（降低噪音）

### 3.5 扫描器（internal/scanner）

#### 职责

- 合并容器发现、digest 获取与策略评估
- 产出容器状态列表
- 缓存到 `state`

#### 并发控制

- 使用 goroutine + 限制并发度（`semaphore` 或 `worker pool`）
- 同一镜像引用的远端请求合并（singleflight）

#### 状态管理

- `ContainerStatus` 增加 `running` 字段（`State==running`）
- 缓存扫描结果供 API 使用

### 3.6 更新器（internal/updater）

#### 触发条件

- 容器状态为 `UpdateAvailable` 且未被策略跳过
- 或手动触发

#### 更新步骤

1. `ImagePull` 新镜像
2. 读取原容器配置并停止原容器
3. 创建新容器（同名），启动并健康检查（如配置）
4. 成功后按配置删除旧容器；失败则回滚

#### 并发控制

- 全局最大并发
- 同一镜像组内串行

### 3.7 调度器（internal/scheduler）

#### 调度策略

- 若配置了 `update.autoUpdateCron`：按 cron 扫描并自动更新
- 未配置 cron：仅按 `scan.interval` 扫描，不自动更新

#### 实现方式

- 使用 `github.com/robfig/cron/v3`
- 支持标准 cron 表达式
- 支持时区配置

### 3.8 Compose 客户端（internal/composecli）

#### 实现方式

- 封装 `docker compose` CLI 命令
- 执行命令并解析输出
- 支持项目发现、启动、停止、删除等操作

#### 主要功能

```go
type Client interface {
    List(ctx context.Context) ([]Project, error)
    Up(ctx context.Context, projectPath string) error
    Down(ctx context.Context, projectPath string) error
    Start(ctx context.Context, projectPath string) error
    Stop(ctx context.Context, projectPath string) error
    Restart(ctx context.Context, projectPath string) error
    Logs(ctx context.Context, projectPath string, follow bool) (io.ReadCloser, error)
}
```

### 3.9 WebSocket 流管理（internal/wsstream）

#### 流类型

- **容器状态流**: 实时推送容器状态变化
- **Compose 日志流**: 流式传输 Compose 日志
- **Shell 终端流**: 双向交互式终端通信

#### 实现方式

- 使用 Gorilla WebSocket 库
- 实现流管理器（StreamManager）
- 支持多客户端订阅
- 自动清理断开的连接

#### 核心结构

```go
type StreamManager struct {
    streams map[string]*Stream
    mu      sync.RWMutex
}

type Stream struct {
    ID      string
    Source  StreamSource
    Clients map[*Client]bool
}

type StreamSource interface {
    Start(ctx context.Context) error
    Stop() error
    Read() ([]byte, error)
}
```

## 4. 二次验证实现（internal/twofa）

### 4.1 模块结构

#### types.go

定义数据结构和接口：

```go
type UserTwoFAConfig struct {
    Method              string
    OTPSecret           string
    WebAuthnCredentials []WebAuthnCredentialWithRPID
    IsSetup             bool
}

type WebAuthnCredentialWithRPID struct {
    Credential webauthn.Credential
    RPID       string
}

type WebAuthnUser struct {
    id          []byte
    name        string
    displayName string
    credentials []webauthn.Credential
}

// 实现 webauthn.User 接口
func (u *WebAuthnUser) WebAuthnID() []byte
func (u *WebAuthnUser) WebAuthnName() string
func (u *WebAuthnUser) WebAuthnDisplayName() string
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential
```

#### otp.go

OTP 功能实现：

```go
// 生成 Base32 编码的 OTP 密钥
func GenerateOTPSecret() (string, error)

// 生成用于身份验证器的 otpauth URL
func GenerateQRCodeURL(secret, account, issuer string) (string, error)

// 验证 6 位 OTP 验证码（支持时间偏移容错）
func ValidateOTPCode(secret, code string) bool
```

#### webauthn.go

WebAuthn 功能实现：

```go
// 创建 WebAuthn 服务实例（配置 RPID 和 Origin）
func NewWebAuthnService(displayName, rpID, origin string) (*webauthn.WebAuthn, error)

// 开始 WebAuthn 注册流程
func BeginRegistration(username string, existingCredentials []webauthn.Credential)
    (*protocol.CredentialCreation, *webauthn.SessionData, error)

// 完成 WebAuthn 注册并返回凭据
func FinishRegistration(username string, credentials []webauthn.Credential,
    session webauthn.SessionData, response *protocol.ParsedCredentialCreationData)
    (*webauthn.Credential, error)

// 开始 WebAuthn 验证流程
func BeginLogin(username string, credentials []webauthn.Credential)
    (*protocol.CredentialAssertion, *webauthn.SessionData, error)

// 完成 WebAuthn 验证
func FinishLogin(username string, credentials []webauthn.Credential,
    session webauthn.SessionData, response *protocol.ParsedCredentialAssertionData)
    (*webauthn.Credential, error)
```

#### storage.go

配置持久化：

```go
// 从配置文件读取用户二次验证配置
func GetUserConfig(username string) (*UserTwoFAConfig, error)

// 保存用户二次验证配置到文件
func SaveUserConfig(username string, config *UserTwoFAConfig) error

// 获取用户在指定域名下的 WebAuthn 凭据
func GetUserCredentialsForRPID(username, rpid string) ([]webauthn.Credential, error)

// 检查用户是否已为指定方法和域名设置二次验证
func IsUserSetupForMethod(username, method, rpid string) (bool, error)
```

### 4.2 认证模块（internal/auth）

#### JWT Token 结构

```go
type Claims struct {
    Username       string `json:"username"`
    TwoFAVerified  bool   `json:"twoFAVerified"`  // 是否完成二次验证
    IsTempToken    bool   `json:"isTempToken"`    // 是否为临时 Token
    jwt.StandardClaims
}
```

#### 核心函数

```go
// 生成完整 Token（24 小时有效期）
func GenerateToken(username string) (string, error)

// 生成临时 Token（15 分钟有效期）
func GenerateTempToken(username string) (string, error)

// 升级临时 Token 为完整 Token
func UpgradeTempToken(tempToken string) (string, error)

// 解析并验证 Token
func ParseToken(tokenString string) (*Claims, error)
```

#### 中间件

```go
// 标准认证中间件，拒绝临时 Token
func AuthMiddleware() gin.HandlerFunc

// 允许临时 Token 的中间件，用于二次验证 API
func TempTokenMiddleware() gin.HandlerFunc
```

### 4.3 API 处理器（internal/api/twofa_handler.go）

#### 辅助函数

```go
// 从请求头提取 RPID 和 Origin（支持反向代理）
func extractRPIDAndOrigin(c *gin.Context) (rpid string, origin string)

// 检查域名是否在白名单中
func isAllowedDomain(domain string) bool
```

#### 处理函数

- `handleTwoFAStatus()`: 获取用户二次验证状态
- `handleOTPSetupInit()`: 初始化 OTP 设置（生成密钥和二维码）
- `handleOTPSetupVerify()`: 验证 OTP 并完成设置
- `handleVerifyOTP()`: 验证 OTP 登录
- `handleWebAuthnRegisterBegin()`: 开始 WebAuthn 注册
- `handleWebAuthnRegisterFinish()`: 完成 WebAuthn 注册
- `handleWebAuthnLoginBegin()`: 开始 WebAuthn 验证
- `handleWebAuthnLoginFinish()`: 完成 WebAuthn 验证
- `handleDisableTwoFA()`: 禁用二次验证

## 5. API 路由实现（internal/api/router.go）

### 5.1 路由注册

```go
// 健康检查
router.GET("/healthz", s.handleHealth())
router.GET("/readyz", s.handleReady())

// 认证相关
v1 := router.Group("/api/v1")
{
    v1.POST("/auth/login", s.handleLogin())

    // 二次验证 API（允许临时 Token）
    twofa := v1.Group("/2fa").Use(auth.TempTokenMiddleware())
    {
        twofa.GET("/status", s.handleTwoFAStatus())

        // OTP 设置
        twofa.POST("/setup/otp/init", s.handleOTPSetupInit())
        twofa.POST("/setup/otp/verify", s.handleOTPSetupVerify())

        // WebAuthn 设置
        twofa.POST("/setup/webauthn/begin", s.handleWebAuthnRegisterBegin())
        twofa.POST("/setup/webauthn/finish", s.handleWebAuthnRegisterFinish())

        // OTP 验证
        twofa.POST("/verify/otp", s.handleVerifyOTP())

        // WebAuthn 验证
        twofa.POST("/verify/webauthn/begin", s.handleWebAuthnLoginBegin())
        twofa.POST("/verify/webauthn/finish", s.handleWebAuthnLoginFinish())

        // 禁用二次验证（需要完整 Token，在内部额外验证）
        twofa.POST("/disable", s.handleDisableTwoFA())
    }
}

// 需要完整认证的 API
api := router.Group("/api").Use(auth.AuthMiddleware())
{
    // 容器管理
    api.GET("/containers", s.handleGetContainers())
    api.POST("/containers/:id/update", s.handleUpdateContainer())
    api.POST("/updates/run", s.handleBatchUpdate())
    api.POST("/containers/:id/start", s.handleStartContainer())
    api.POST("/containers/:id/stop", s.handleStopContainer())
    api.DELETE("/containers/:id", s.handleDeleteContainer())

    // 镜像管理
    api.GET("/images", s.handleGetImages())
    api.DELETE("/images", s.handleDeleteImage())

    // Compose 管理
    api.GET("/compose", s.handleGetCompose())
    api.POST("/compose/start", s.handleStartCompose())
    api.POST("/compose/stop", s.handleStopCompose())
    api.POST("/compose/restart", s.handleRestartCompose())
    api.DELETE("/compose/delete", s.handleDeleteCompose())
    api.POST("/compose/create", s.handleCreateCompose())

    // WebSocket
    api.GET("/compose/logs/ws", s.handleComposeLogsWS())
    api.GET("/shell/ws", s.handleShellWS())
}
```

### 5.2 登录处理调整

```go
func (s *Server) handleLogin() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 验证用户名密码
        // 2. 检查 IS_SECONDARY_VERIFICATION 环境变量
        // 3. 如果启用：
        //    - 查询用户是否已设置二次验证
        //    - 生成临时 Token
        //    - 返回 needTwoFA, isSetup, method, tempToken
        // 4. 如果未启用：
        //    - 直接生成完整 Token
        //    - 返回 token
    }
}
```

## 6. 日志与中间件

### 6.1 日志实现

- 初始化 Zap（生产模式，带采样）
- Gin 使用自定义 logger 与 recovery 中间件（捕获 panic）
- 统一日志字段：`component`, `action`, `container`, `image`, `digest`, `durationMs`, `err`

### 6.2 中间件

```go
// CORS 中间件
func CORSMiddleware() gin.HandlerFunc

// 日志中间件
func LoggerMiddleware(logger *zap.Logger) gin.HandlerFunc

// 恢复中间件
func RecoveryMiddleware(logger *zap.Logger) gin.HandlerFunc

// 认证中间件
func AuthMiddleware() gin.HandlerFunc

// 临时 Token 中间件
func TempTokenMiddleware() gin.HandlerFunc
```

## 7. 并发与缓存

### 7.1 并发控制

- **扫描**: 受限 goroutine 池；同镜像引用请求合并
- **更新**: 全局并发上限 + 镜像桶内串行
- **WebSocket**: 每个连接独立 goroutine

### 7.2 缓存策略

- **远端 digest**: TTL 缓存
- **容器状态**: 缓存供 API 使用（当前为即时扫描返回）
- **配置**: 内存缓存，文件变更时重载

## 8. 错误处理与安全

### 8.1 错误处理

- **网络错误**: 指数退避重试（Resty 内置 + 自定义策略）
- **更新失败**: 恢复旧容器运行状态，保留失败上下文日志
- **统一错误码**: 返回明确错误码与文案，日志包含 container/image/digest

### 8.2 安全实现

#### OTP 安全

- 使用 TOTP（RFC 6238）协议
- 密钥长度：32 字节（256 位）
- 时间步长：30 秒
- 验证码长度：6 位
- 时间偏移容错：±1 个时间窗口（前后各 30 秒）

#### WebAuthn 安全

- 挑战长度：32 字节随机数
- 用户验证要求：`preferred`（优先但不强制）
- 认证器选择：`platform`（平台认证器）或 `cross-platform`（跨平台）
- 超时时间：60 秒
- 凭据类型：`public-key`
- Session 数据临时存储在内存中

#### 域名安全

- 从请求头提取真实域名（支持 `X-Forwarded-Host`, `X-Original-Host`）
- WebAuthn RPID 验证（凭据绑定到特定域名）
- 可选域名白名单限制（生产环境推荐）

#### Token 安全

- 临时 Token 短期有效（15 分钟），限制攻击窗口
- 完整 Token 仅在完成二次验证后颁发
- JWT 签名验证，防止篡改
- Token 包含验证状态标记（`twoFAVerified`, `isTempToken`）

## 9. 测试策略

### 9.1 单元测试

- 策略引擎规则测试
- Semver 版本判定测试
- Manifest 选择逻辑测试
- 鉴权分支测试
- 回滚路径测试
- OTP 密钥生成和验证
- JWT Token 生成、解析和升级
- 配置读写和数据序列化
- WebAuthn 用户接口实现

### 9.2 集成测试

- 本地容器操作测试
- 私有仓库鉴权测试
- 完整的 OTP 设置和验证流程
- WebAuthn 注册和验证流程（需要浏览器环境）
- Token 升级和权限验证
- 域名白名单验证

### 9.3 功能测试

- Compose 项目管理
- WebSocket 通信
- Shell 终端交互
- 首次设置流程（OTP 和 WebAuthn）
- 日常登录流程
- 禁用二次验证
- 错误场景（错误验证码、过期 Token 等）
- 多域名场景（WebAuthn）

## 10. 性能与资源

### 10.1 性能优化

- 控制并发与带宽
- 合理配置 daemon 的下载并发
- 缓存 TTL 默认 5m，可按需调整
- Goroutine 池限制并发数
- Singleflight 去重请求

### 10.2 资源管理

- 自动清理断开的 WebSocket 连接
- 定期清理过期缓存
- 限制内存使用
- Goroutine 泄漏检测

## 11. 构建与运行

### 11.1 开发环境

```bash
# 初始化项目
go mod init github.com/your/repo/watch-docker
go mod tidy

# 开发运行
go run cmd/watch-docker/main.go

# 构建
go build -o bin/watch-docker cmd/watch-docker/main.go

# 运行
./bin/watch-docker -config ./config.yaml
```

### 11.2 生产构建

```bash
# 使用 Dockerfile 构建
docker build -t watch-docker .

# 或使用 Go 交叉编译
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o watch-docker cmd/watch-docker/main.go
```

### 11.3 环境变量

常用环境变量：

- `WATCH_SERVER_ADDR`、`WATCH_SCAN_INTERVAL`、`WATCH_UPDATE_ENABLED`
- `WATCH_REGISTRY_AUTH_0_HOST`、`WATCH_REGISTRY_AUTH_0_USERNAME`、`WATCH_REGISTRY_AUTH_0_PASSWORD`
- `IS_SECONDARY_VERIFICATION`、`TWOFA_ALLOWED_DOMAINS`
- `IS_OPEN_DOCKER_SHELL`、`USER_NAME`、`USER_PASSWORD`
- `APP_PATH`、`CONFIG_PATH`、`CONFIG_FILE`

## 12. 里程碑与交付

按设计文档阶段推进，每阶段交付：

- 代码实现
- 可运行二进制
- 基础用例
- 文档更新

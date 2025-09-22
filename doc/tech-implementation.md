## 技术实现方案（watch-docker）

### 1. 技术栈
- 语言: Go 1.21+
- Web: `github.com/gin-gonic/gin`
- 日志: `go.uber.org/zap`
- 配置: `github.com/spf13/viper` + `github.com/kelseyhightower/envconfig`
- HTTP 客户端: `github.com/go-resty/resty/v2`
- Docker SDK: `github.com/docker/docker`
- 定时: `github.com/robfig/cron/v3`
- 语义化版本: `github.com/blang/semver/v4`

### 2. 目录结构（拟）
```
.
├── cmd/
│   └── watch-docker/           # main 入口
├── internal/
│   ├── api/                    # Gin 路由与 handler
│   ├── config/                 # 配置加载与校验
│   ├── dockercli/              # Docker SDK 封装
│   ├── registry/               # Registry V2 客户端
│   ├── scanner/                # 扫描与状态评估
│   ├── updater/                # 更新器与回滚
│   ├── policy/                 # 策略判定
│   ├── scheduler/              # 调度器（interval/cron）
│   ├── state/                  # 进程内缓存与模型
│   └── logging/                # zap 初始化
├── pkg/                        # 对外可复用库（如有）
├── doc/                        # 设计与实现文档
└── go.mod / go.sum
```

### 3. 配置模型
```go
type ServerConfig struct {
    Addr string `mapstructure:"addr" envconfig:"SERVER_ADDR" default:":8080"`
}

type DockerConfig struct {
    Host           string `mapstructure:"host" envconfig:"DOCKER_HOST"`
    IncludeStopped bool   `mapstructure:"includeStopped" envconfig:"DOCKER_INCLUDE_STOPPED"`
}

type ScanConfig struct {
    Interval          time.Duration `mapstructure:"interval" envconfig:"SCAN_INTERVAL"`
    Cron              string        `mapstructure:"cron" envconfig:"SCAN_CRON"`
    InitialScanOnStart bool         `mapstructure:"initialScanOnStart" envconfig:"SCAN_INITIAL_ON_START"`
    Concurrency       int           `mapstructure:"concurrency" envconfig:"SCAN_CONCURRENCY" default:"3"`
    CacheTTL          time.Duration `mapstructure:"cacheTTL" envconfig:"SCAN_CACHE_TTL" default:"5m"`
}

type UpdateConfig struct {
    Enabled            bool   `mapstructure:"enabled" envconfig:"UPDATE_ENABLED"`
    AutoUpdateCron     string `mapstructure:"autoUpdateCron" envconfig:"UPDATE_CRON"`
    AllowComposeUpdate bool   `mapstructure:"allowComposeUpdate" envconfig:"UPDATE_ALLOW_COMPOSE"`
    RecreateStrategy   string `mapstructure:"recreateStrategy" envconfig:"UPDATE_STRATEGY"`
    RemoveOldContainer bool   `mapstructure:"removeOldContainer" envconfig:"UPDATE_REMOVE_OLD"`
}

type PolicyConfig struct {
    SkipLabels       []string `mapstructure:"skipLabels"`
    OnlyLabels       []string `mapstructure:"onlyLabels"`
    ExcludeLabels    []string `mapstructure:"excludeLabels"`
    SkipLocalBuild   bool     `mapstructure:"skipLocalBuild"`
    SkipPinnedDigest bool     `mapstructure:"skipPinnedDigest"`
    SkipSemverPinned bool     `mapstructure:"skipSemverPinned"`
    FloatingTags     []string `mapstructure:"floatingTags"`
}

type RegistryAuth struct {
    Host     string `mapstructure:"host" envconfig:"HOST"`
    Username string `mapstructure:"username" envconfig:"USERNAME"`
    Password string `mapstructure:"password" envconfig:"PASSWORD"`
}

type RegistryConfig struct {
    Auth []RegistryAuth `mapstructure:"auth"`
}

type LoggingConfig struct {
    Level string `mapstructure:"level" envconfig:"LOG_LEVEL" default:"info"`
}

type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Docker   DockerConfig   `mapstructure:"docker"`
    Scan     ScanConfig     `mapstructure:"scan"`
    Update   UpdateConfig   `mapstructure:"update"`
    Policy   PolicyConfig   `mapstructure:"policy"`
    Registry RegistryConfig `mapstructure:"registry"`
    Logging  LoggingConfig  `mapstructure:"logging"`
}
```

#### 配置加载策略
- 使用 Viper 读取 `config.yaml`（支持多个默认路径），ENV 前缀 `WATCH_`。
- 使用 envconfig 将 ENV 映射至结构体（覆盖 Viper 值）。
- 提供 `Validate()` 校验（端口格式、并发度、策略冲突等）。

### 4. 日志与中间件
- 初始化 Zap（生产模式，带采样）；Gin 使用自定义 logger 与 recovery 中间件（捕获 panic）。
- 统一日志字段：`component`, `action`, `container`, `image`, `digest`, `durationMs`, `err`。

### 5. Docker 客户端封装（internal/dockercli）
- 关键操作：
  - 列出容器：`ContainerList(ctx, opts)`。
  - 检查容器：`ContainerInspect(ctx, id)`。
  - 拉取镜像：`ImagePull(ctx, ref, types.ImagePullOptions)`。
  - 创建/启动/停止/删除容器：`ContainerCreate/Start/Stop/Remove`。
- 容器重建：
  - 提取原容器 `Config/HostConfig/NetworkingConfig`。
  - 停止并重命名旧容器 → 创建新容器复用名称 → 启动 → 可选删除旧容器。

### 6. Registry 客户端（internal/registry）
- 使用 Resty：
  - Docker Hub/ghcr 等通用 Registry V2：`/v2/<repo>/manifests/<tag>` `HEAD/GET`。
  - Accept 头部包含：
    - Docker manifest list: `application/vnd.docker.distribution.manifest.list.v2+json`
    - Docker manifest: `application/vnd.docker.distribution.manifest.v2+json`
    - OCI index/manifest: `application/vnd.oci.image.index.v1+json`, `application/vnd.oci.image.manifest.v1+json`
  - 从响应头 `Docker-Content-Digest` 读取 digest。
- 鉴权：
  - 处理 401 响应中的 `WWW-Authenticate`，请求 token endpoint 获取 Bearer Token（scope=repository:repo:pull）。
  - 支持 per-registry 基本凭据（ENV 注入）。
- Manifest list 处理：根据本机平台选择合适子 manifest（os/architecture/variant）。
- 缓存：`map[imageRef]digest` + TTL；错误也做短 TTL 缓存避免雪崩。
- 去重：`singleflight` 合并同一镜像并发请求。

### 7. 策略引擎（internal/policy）
- 输入：容器信息、镜像解析结果、远端 digest、本地 RepoDigests、配置。
- 输出：`Decision{Skipped bool, Reason string, Force bool}`。
- 规则：
  - Label 匹配：`watchdocker.skip=true` → skip；`watchdocker.force=true` → force。
  - 本地构建：`len(RepoDigests)==0` → skip（可关）。
  - 固定版本：`@sha256:` 或严格 semver → skip（可关）。
  - 浮动标签名单外的 tag 可选择不检查（降低噪音）。

### 8. 扫描器（internal/scanner）
- 职责：合并容器发现、digest 获取与策略评估，产出容器状态列表，并缓存到 `state`。
- 并发：使用 goroutine + 限制并发度（`semaphore` 或 `worker pool`）。
- 状态：`ContainerStatus` 增加 `running` 字段（`State==running`）。
- 去重：相同镜像引用的远端请求合并（singleflight）。

### 9. 更新器（internal/updater）
- 触发条件：容器状态为 `UpdateAvailable` 且未被策略跳过，或手动触发。
- 步骤：
  1) `ImagePull` 新镜像。
  2) 读取原容器配置并停止原容器。
  3) 创建新容器（同名），启动并健康检查（如配置）。
  4) 成功后按配置删除旧容器；失败则回滚。
- 并发控制：全局最大并发；同一镜像组内串行。

### 10. 调度器（internal/scheduler）
- 若配置了 `update.autoUpdateCron`：按 cron 扫描并自动更新。
- 未配置 cron：仅按 `scan.interval` 扫描，不自动更新。

### 11. API（internal/api）
- 路由：
  - `GET /healthz`、`GET /readyz`
  - `GET /api/containers`（含 running）
  - `POST /api/containers/:id/update`
  - `POST /api/updates/run`
  - `POST /api/containers/:id/stop`
  - `POST /api/containers/:id/start`
  - `DELETE /api/containers/:id`
- 统一响应：`BaseRes { code, msg, data }`，成功为 `code=0`
- 错误码建议：`CodeImageRequired=40001`、`CodeScanFailed=50001`、`CodeUpdateFailed=50002`、`CodeDockerError=50003`、`CodeRegistryError=50004`

### 12. 并发与缓存
- 扫描：受限 goroutine 池；同镜像引用请求合并。
- 更新：全局并发上限 + 镜像桶内串行。
- 缓存：远端 digest TTL；容器状态缓存供 API 使用（当前为即时扫描返回）。

### 13. 错误处理与回滚
- 网络错误：指数退避重试（Resty 内置 + 自定义策略）。
- 更新失败：恢复旧容器运行状态，保留失败上下文日志。
- 返回明确错误码与文案，日志包含 container/image/digest。

### 14. 安全
- Docker socket 最小权限；更新阶段需要写权限。
- Registry 凭据由 ENV/Secret 注入，避免落盘。
- 可选：只读模式/简单 token 保护 API。

### 15. 测试策略
- 单元测试：策略、semver、manifest 选择、鉴权分支、回滚路径。
- 集成测试：本地容器与私有仓库鉴权。

### 16. 性能与资源
- 控制并发与带宽；合理配置 daemon 的下载并发。
- 缓存 TTL 默认 5m，可按需调整。

### 17. 构建与运行（参考）
```bash
go mod init github.com/your/repo/watch-docker
go mod tidy
go build -o bin/watch-docker ./cmd/watch-docker

./bin/watch-docker -config ./config.yaml
```

常用环境变量：
- `WATCH_SERVER_ADDR`、`WATCH_SCAN_INTERVAL`、`WATCH_UPDATE_ENABLED`
- `WATCH_REGISTRY_AUTH_0_HOST`、`WATCH_REGISTRY_AUTH_0_USERNAME`、`WATCH_REGISTRY_AUTH_0_PASSWORD`

### 18. 里程碑与验收
- 按设计文档阶段推进；每阶段交付：代码、可运行二进制、基础用例与文档更新。



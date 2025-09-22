## 项目设计文档（watch-docker）

### 1. 背景与目标
- **背景**: 在 Docker 主机上自动监控并更新运行中的容器，借鉴 watchtower 思路，但提供更强的可观测性、策略控制与 API。
- **目标**:
  - 列出当前主机的所有容器与镜像信息。
  - 周期性检测镜像是否存在更新（基于远端 manifest digest 对比）。
  - 支持按间隔或 cron 定时执行自动更新。
  - 支持多种跳过策略：按 label、本地构建镜像、固定版本（digest 或语义化版本）。
  - 提供 HTTP API 供查询与手动触发更新。
- **非目标（当前阶段）**:
  - 跨主机编排与滚动发布（Kubernetes/Swarm）。
  - 私有镜像仓库的证书管理与密钥下发（依赖外部 Secret 注入）。

### 2. 术语
- **容器引用（image ref）**: registry/namespace/repo:tag 或 repo@sha256:digest。
- **本地构建镜像**: `RepoDigests` 为空，未推送至远端 registry 的镜像。
- **固定版本**: 通过 digest 固定，或标签为严格语义化版本（如 1.2.3 / v1.2.3）。
- **浮动标签**: 如 latest、stable、main 等可能随时间漂移的标签。

### 3. 总体架构
- **核心组件**:
  - `DockerClient`: 通过 Docker SDK 获取容器/镜像信息，执行拉取、创建、启动、删除等操作。
  - `RegistryClient`: 通过 Registry V2 API（Resty）获取远端 manifest 与 digest。
  - `Scanner`: 周期性扫描容器，评估是否有更新，并汇总状态。
  - `PolicyEngine`: 应用跳过与强制策略（label、本地构建、固定版本识别）。
  - `Updater`: 执行拉取新镜像、按原配置重建容器与回滚。
  - `Scheduler`: 按 interval 或 cron 触发扫描/更新作业。
  - `StateCache`: 进程内缓存镜像远端 digest、扫描结果、上次更新时间等。
  - `HTTP API`: Gin 提供查询与操作接口。
  - `Config`: Viper + envconfig 读取 YAML/ENV 配置。
  - `Logger`: Zap 结构化日志。

- **数据流**:
  1) Scheduler 触发扫描。
  2) Scanner 调用 DockerClient 列出容器，解析镜像引用。
  3) RegistryClient 获取对应远端 digest（带缓存与去重）。
  4) PolicyEngine 评估：跳过或标记需要更新。
  5) 结果写入 StateCache，API 对外提供。
  6) 若配置自动更新，Scheduler 触发 Updater 执行更新。

### 4. 容器发现与信息模型
- **发现策略**: 默认仅包含运行中容器，可配置包含已停止容器。
- **采集字段**:
  - 容器: ID、Name、Image（原始引用）、RepoTags、RepoDigests、Labels、State、Created、Compose 相关 labels。
  - 平台: GOOS/GOARCH/variant（用于选择 manifest list 子项）。

- **状态模型（对外返回）**:
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
  "labels": {"watchdocker.skip": "true"},
  "lastCheckedAt": "2025-09-19T09:30:00Z"
}
```

### 5. 更新检查
- **对比原则**: 本地镜像 `RepoDigests` 与远端 manifest `Docker-Content-Digest` 比较。
- **远端 digest 获取**:
  - 解析 `registry/namespace/repo:tag`。
  - 先鉴权（Docker Hub/ghcr 等支持 Bearer Token），再 `HEAD/GET` manifest（Accept: OCI/Docker manifest / manifest list）。
  - 如为 manifest list，按宿主平台选择具体 manifest，取其 digest。
- **判定规则**:
  - 浮动标签且远端 digest ≠ 本地 → `UpdateAvailable`。
  - 镜像以 `@sha256:` 引用 → 视为固定版本，`Skipped`（除非强制）。
  - 语义化版本标签（1.2.3 / v1.2.3）且启用 `skipSemverPinned` → `Skipped`。
  - 本地构建（无 RepoDigests） → `Skipped`。
- **优化**:
  - 结果缓存（TTL），同一镜像去重请求。
  - 网络错误退避重试。

### 6. 跳过与强制策略（PolicyEngine）
- **Label 策略**:
  - `watchdocker.skip=true` → 跳过。
  - `watchdocker.force=true` → 即便固定版本也可更新。
  - 支持 `onlyLabels` 与 `excludeLabels` 组合过滤。
- **镜像来源**:
  - 无 `RepoDigests` → 本地构建，跳过。
- **固定版本**:
  - `image@sha256:...` 或严格 semver 标签 → 跳过（可配置）。

### 7. 更新执行（Updater）
- **流程**:
  1) `ImagePull` 拉取目标镜像（如已存在不同 digest）。
  2) `ContainerInspect` 读取原容器配置（Config/HostConfig/NetworkingConfig）。
  3) 优雅停止原容器（带超时），重命名旧容器。
  4) 以相同参数创建并启动新容器（复用名称）。
  5) 新容器健康后删除旧容器（可配置保留）。
- **注意点**:
  - 端口、卷、网络、环境变量、重启策略与日志驱动需完整保留。
  - 对含 `com.docker.compose.*` labels 的容器，默认仅提示不自动更新，避免与 Compose 冲突（可配置允许）。
  - 并发与镜像分组串行：同一镜像的多个容器更新需串行，避免重复拉取。
- **失败与回滚**:
  - 新容器启动失败立即回滚激活旧容器并记录错误。

### 8. 调度（Scheduler）
- **模式**:
  - `interval` 周期扫描/更新。
  - `cron` 指定时刻自动更新（支持时区与秒级精度）。
- **任务**:
  - 扫描任务：只更新状态。
  - 更新任务：对 `UpdateAvailable` 且通过策略的容器执行更新。
- **并发控制**:
  - 全局并发度限制 + 每镜像桶内串行。

### 9. API 设计（Gin）
- `GET /healthz`：健康检查。
- `GET /readyz`：就绪检查。
- `GET /api/containers`：列出容器及状态（含 running）。
- `POST /api/containers/:id/update`：对单容器触发更新。
- `POST /api/updates/run`：批量更新。
- `POST /api/containers/:id/stop`：停止容器。
- `POST /api/containers/:id/start`：启动容器。
- `DELETE /api/containers/:id`：删除容器。
- `GET /api/jobs`：查看调度任务、上次运行时间与结果。

返回示例见第 4 节。

### 10. 统一响应与错误码
- 统一响应 `BaseRes`：`{ code, msg, data }`，`code=0` 为成功
- 预置错误码：`CodeImageRequired=40001`、`CodeScanFailed=50001`、`CodeUpdateFailed=50002`、`CodeDockerError=50003`、`CodeRegistryError=50004`

### 11. 配置（YAML + ENV）
```yaml
server:
  addr: ":8080"

docker:
  host: "unix:///var/run/docker.sock"
  includeStopped: false

scan:
  interval: "10m"
  cron: ""
  initialScanOnStart: true
  concurrency: 3
  cacheTTL: "5m"

update:
  enabled: true
  autoUpdateCron: "0 3 * * *"
  allowComposeUpdate: false
  recreateStrategy: "recreate"
  removeOldContainer: true

policy:
  skipLabels: ["watchdocker.skip=true"]
  onlyLabels: []
  excludeLabels: []
  skipLocalBuild: true
  skipPinnedDigest: true
  skipSemverPinned: true
  floatingTags: ["latest", "main", "stable"]

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
```

ENV 覆盖（前缀 `WATCH_`），常见示例：
- `WATCH_SERVER_ADDR=:8080`
- `WATCH_SCAN_INTERVAL=10m`
- `WATCH_UPDATE_ENABLED=true`
- `WATCH_REGISTRY_AUTH_0_HOST=ghcr.io`
- `WATCH_REGISTRY_AUTH_0_USERNAME=xxx`
- `WATCH_REGISTRY_AUTH_0_PASSWORD=yyy`

### 12. 可观测性与日志
- Zap 结构化日志字段：`component`, `container`, `image`, `digest`, `action`, `durationMs`。
- 指标（可选）：扫描耗时、成功/失败更新计数、registry 请求时延。

### 13. 安全与部署
- 访问 Docker socket（建议最小权限挂载，更新需写权限）。
- Registry 凭据通过环境变量/Secret 注入，避免明文落盘。
- 进程以非 root 运行（如可能），限制网络超时、设置重试上限。

### 14. 里程碑
- M1: 项目骨架、配置/日志、健康检查。
- M2: 容器发现与本地信息返回。
- M3: Registry digest 获取与比对。
- M4: 跳过策略完善。
- M5: 更新器（重建与回滚）。
- M6: 调度器与自动更新。
- M7: API 完整化。
- M8: 稳定性与文档完善。



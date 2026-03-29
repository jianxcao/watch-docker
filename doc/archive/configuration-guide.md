# Watch Docker - 配置指南

## 配置文件位置

- **Linux**: `~/.watch-docker/config.yaml`
- **macOS**: `~/.watch-docker/config.yaml`
- **Windows**: `%USERPROFILE%\.watch-docker\config.yaml`
- **Docker**: `/config/config.yaml`

## 完整配置示例

```yaml
# 服务器配置
server:
  addr: ":8080"              # 监听地址和端口
  readTimeout: "30s"         # 读取超时
  writeTimeout: "30s"        # 写入超时
  readOnly: false            # 全局只读模式

# Docker 配置
docker:
  host: "unix:///var/run/docker.sock"  # Docker socket 地址
  # Windows: "npipe:////./pipe/docker_engine"
  # TCP: "tcp://localhost:2375"
  includeStopped: false      # 是否包含已停止的容器
  apiVersion: ""             # Docker API 版本（留空自动检测）

# 扫描配置
scan:
  interval: "10m"            # 扫描间隔（检查镜像更新）
  initialScanOnStart: true   # 启动时立即扫描
  concurrency: 3             # 并发扫描数量
  cacheTTL: "5m"             # 缓存时间
  enabled: true              # 是否启用扫描

# 更新配置
update:
  enabled: true              # 启用自动更新
  autoUpdateCron: "0 3 * * *"  # Cron 表达式（每天凌晨3点）
  allowComposeUpdate: false  # 是否允许更新 Compose 容器
  removeOldContainer: true   # 更新后删除旧容器
  removeOldImage: false      # 更新后删除旧镜像

# 策略配置
policy:
  skipLabels:                # 跳过包含这些标签的容器
    - "watchdocker.skip=true"
  skipLocalBuild: true       # 跳过本地构建的镜像
  skipPinnedDigest: true     # 跳过固定 digest 的镜像
  skipSemverPinned: true     # 跳过语义化版本固定的镜像

# 镜像仓库认证
registry:
  auth:
    - host: "registry-1.docker.io"  # Docker Hub
      username: ""
      password: ""
    - host: "ghcr.io"               # GitHub Container Registry
      username: ""
      password: ""
    - host: "registry.example.com"  # 私有仓库
      username: ""
      password: ""

# 日志配置
logging:
  level: "info"              # 日志级别: debug, info, warn, error
  format: "json"             # 日志格式: json, text
  output: "stdout"           # 输出: stdout, file
  file:
    path: "./logs/watch-docker.log"
    maxSize: 100             # MB
    maxBackups: 3
    maxAge: 28               # 天
    compress: true

# 认证配置
auth:
  username: "admin"          # 用户名
  password: "admin"          # 密码（建议修改）
  tokenExpiry: "24h"         # Token 有效期

# 二次验证配置
twofa:
  enabled: false             # 是否启用二次验证
  allowedDomains: []         # WebAuthn 允许的域名（留空允许所有）
  users:                     # 用户配置（自动生成，不要手动编辑）
    admin:
      method: "otp"          # otp 或 webauthn
      otpSecret: ""          # OTP 密钥（自动生成）
      webauthnCredentials: [] # WebAuthn 凭据（自动生成）
      isSetup: false

# Shell 终端配置
shell:
  enabled: false             # 是否启用 Shell 功能（⚠️ 高风险）
  timeout: "30m"             # Shell 会话超时
  maxSessions: 5             # 最大并发会话数

# Compose 项目配置
compose:
  projectPath: ""            # Compose 项目目录（留空不启用）
  autoDiscover: true         # 自动发现项目

# 通知配置（可选）
notification:
  enabled: false
  providers:
    - type: "webhook"
      url: "https://example.com/webhook"
      events: ["update_success", "update_failed"]
```

## 环境变量

环境变量优先级高于配置文件。

### 基础配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `CONFIG_PATH` | `~/.watch-docker` | 配置目录 |
| `CONFIG_FILE` | `config.yaml` | 配置文件名 |
| `PORT` | `8080` | 服务端口 |
| `USER_NAME` | `admin` | 用户名 |
| `USER_PASSWORD` | `admin` | 密码 |
| `TZ` | `Asia/Shanghai` | 时区 |

### Docker 配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket |
| `DOCKER_API_VERSION` | ` ` | Docker API 版本 |

### 功能开关

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `IS_OPEN_DOCKER_SHELL` | `false` | 是否启用 Shell |
| `IS_SECONDARY_VERIFICATION` | `false` | 是否启用二次验证 |
| `APP_PATH` | ` ` | Compose 项目路径 |

### 安全配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `TWOFA_ALLOWED_DOMAINS` | ` ` | WebAuthn 域名白名单（逗号分隔） |

### 进程配置（Docker）

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PUID` | `0` | 运行进程的用户 ID |
| `PGID` | `0` | 运行进程的组 ID |
| `UMASK` | `0000` | 文件权限掩码 |

## 配置优先级

1. 命令行参数（最高）
2. 环境变量
3. 配置文件
4. 默认值（最低）

## 常见配置场景

### 场景一：家庭 NAS

```yaml
server:
  addr: ":8080"

docker:
  host: "unix:///var/run/docker.sock"

update:
  enabled: true
  autoUpdateCron: "0 3 * * *"  # 每天凌晨3点更新
  allowComposeUpdate: false

policy:
  skipLabels:
    - "watchdocker.skip=true"
```

### 场景二：开发环境

```yaml
server:
  addr: ":8080"

scan:
  interval: "5m"  # 更频繁的扫描

update:
  enabled: false  # 禁用自动更新

logging:
  level: "debug"  # 详细日志
```

### 场景三：生产环境

```yaml
server:
  addr: ":8080"
  readOnly: true  # 只读模式

docker:
  host: "unix:///var/run/docker.sock"

update:
  enabled: false  # 生产环境禁用自动更新

auth:
  username: "admin"
  password: "your_strong_password_here"  # 强密码

twofa:
  enabled: true  # 启用二次验证

shell:
  enabled: false  # 生产环境禁用 Shell

logging:
  level: "warn"
  format: "json"
```

## 安全建议

1. **修改默认密码**
   ```yaml
   auth:
     username: "admin"
     password: "your_strong_password_here"
   ```

2. **启用二次验证**
   ```yaml
   twofa:
     enabled: true
   ```

3. **限制网络访问**
   ```yaml
   server:
     addr: "127.0.0.1:8080"  # 仅本地访问
   ```

4. **禁用危险功能**
   ```yaml
   shell:
     enabled: false  # 禁用 Shell
   ```

5. **设置只读模式**
   ```yaml
   server:
     readOnly: true  # 全局只读
   ```

## 故障排除

### 配置文件不生效

1. 检查配置文件路径
2. 检查 YAML 格式是否正确
3. 检查环境变量是否覆盖了配置

### 无法连接 Docker

检查 Docker socket 路径：
```bash
# Linux/macOS
ls -la /var/run/docker.sock

# Windows
docker version  # 确认 Docker Desktop 运行
```

### 端口被占用

修改监听端口：
```yaml
server:
  addr: ":9090"
```

## 更多帮助

- 配置文档: https://github.com/jianxcao/watch-docker/blob/main/doc/user-guide/configuration.md
- 问题反馈: https://github.com/jianxcao/watch-docker/issues

# Watch Docker 配置指南

Watch Docker 支持多种配置方式，本文档详细说明如何配置应用。

## 配置优先级

Watch Docker 支持多种配置方式，按以下优先级加载（从高到低）：

1. **环境变量** - 最高优先级
2. **配置文件** (`~/.watch-docker/config.yaml` 和 `~/.watch-docker/app.yaml`)
3. **默认值** - 最低优先级

## 配置文件位置

### 默认位置

```
~/.watch-docker/
├── app.yaml        # 应用环境配置（用户名、密码、功能开关）
└── config.yaml    # Docker 业务配置（扫描、通知、服务器等）
```

### 自定义位置

通过环境变量指定：

```bash
export CONFIG_PATH="/path/to/config/dir"
export CONFIG_FILE="my-config.yaml"
export ENV_FILE="my-app.yaml"
```

## 配置文件格式

### app.yaml - 应用环境配置

创建或编辑 `~/.watch-docker/app.yaml`：

```yaml
# 用户认证配置
username: "admin"          # 登录用户名（可通过 USER_NAME 环境变量覆盖）
password: "admin"          # 登录密码（可通过 USER_PASSWORD 环境变量覆盖）

# 功能开关
enable_2fa: false          # 是否启用双因素认证（可通过 IS_SECONDARY_VERIFICATION 覆盖）
twofa_allowed_domains: ""  # 2FA 允许的域名白名单，逗号分隔（可通过 TWOFA_ALLOWED_DOMAINS 覆盖）
enable_docker_shell: false # 是否开启容器 shell 功能（可通过 IS_OPEN_DOCKER_SHELL 覆盖）

# 应用配置
static_dir: ""             # 静态资源目录，空表示使用嵌入资源（可通过 STATIC_DIR 覆盖）
app_path: ""               # Docker Compose 项目目录（可通过 APP_PATH 覆盖）
version: "v0.1.6"          # 应用版本（可通过 VERSION_WATCH_DOCKER 覆盖）
```

### config.yaml - Docker 业务配置

创建或编辑 `~/.watch-docker/config.yaml`：

```yaml
# 服务器配置
server:
  addr: ":8080"              # HTTP 服务监听地址
  read_timeout: 30s          # 读取超时
  write_timeout: 30s         # 写入超时

# Docker 配置
docker:
  host: "unix:///var/run/docker.sock"  # Docker socket 路径
  includeStopped: false                 # 是否包含已停止的容器

# 扫描配置
scan:
  interval: "1h"             # 扫描间隔
  concurrency: 5             # 并发扫描数
  cacheTTL: "5m"            # 缓存时间

# 更新配置
update:
  enabled: true              # 是否启用自动更新
  autoUpdateCron: "0 3 * * *"  # 自动更新 cron 表达式
  allowComposeUpdate: false  # 是否允许更新 Compose 容器
  removeOldContainer: true   # 更新后删除旧容器

# 策略配置
policy:
  skipLabels: ["watchdocker.skip=true"]  # 跳过标签
  skipLocalBuild: true                   # 跳过本地构建
  skipPinnedDigest: true                 # 跳过固定 digest
  skipSemverPinned: true                 # 跳过语义化版本

# Registry 认证配置
registry:
  auth:
    - host: "registry-1.docker.io"
      username: ""
      password: ""
    - host: "ghcr.io"
      username: ""
      password: ""

# 通知配置
notify:
  url: ""                    # 通知 Webhook URL
  method: "POST"             # HTTP 方法 (GET/POST)

# 日志配置
logging:
  level: "info"              # 日志级别：debug, info, warn, error
```

## 环境变量配置

所有配置都可以通过环境变量覆盖配置文件：

### 基本配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `CONFIG_PATH` | `~/.watch-docker` | 配置文件目录 |
| `CONFIG_FILE` | `config.yaml` | 配置文件名 |
| `ENV_FILE` | `app.yaml` | 应用配置文件 |
| `VERSION_WATCH_DOCKER` | `v0.1.6` | 应用版本 |

### 认证配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `USER_NAME` | `admin` | 登录用户名 |
| `USER_PASSWORD` | `admin` | 登录密码 |
| `IS_SECONDARY_VERIFICATION` | `false` | 是否启用双因素认证 |
| `TWOFA_ALLOWED_DOMAINS` | `""` | 2FA 域名白名单（逗号分隔） |

### 功能配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `STATIC_DIR` | `""` | 静态资源目录（空=使用嵌入资源） |
| `IS_OPEN_DOCKER_SHELL` | `false` | 是否开启容器 shell |
| `APP_PATH` | `""` | Docker Compose 项目目录 |

### Docker 业务配置

| 环境变量 | 说明 |
|----------|------|
| `WATCH_SERVER_ADDR` | HTTP 服务监听地址 |
| `WATCH_SCAN_INTERVAL` | 扫描间隔 |
| `WATCH_UPDATE_ENABLED` | 是否启用自动更新 |
| `WATCH_REGISTRY_AUTH_0_HOST` | Registry 主机地址 |
| `WATCH_REGISTRY_AUTH_0_USERNAME` | Registry 用户名 |
| `WATCH_REGISTRY_AUTH_0_PASSWORD` | Registry 密码 |

## 配置示例

### 示例 1：修改端口和密码

**方式 A：使用配置文件**

编辑 `~/.watch-docker/app.yaml`：
```yaml
username: "myuser"
password: "mypassword"
```

编辑 `~/.watch-docker/config.yaml`：
```yaml
server:
  addr: ":9090"
```

**方式 B：使用环境变量**

```bash
export USER_NAME="myuser"
export USER_PASSWORD="mypassword"
export WATCH_SERVER_ADDR=":9090"
```

### 示例 2：启用双因素认证

**配置文件方式：**

编辑 `~/.watch-docker/app.yaml`：
```yaml
enable_2fa: true
twofa_allowed_domains: "example.com,localhost:8080"
```

**环境变量方式：**

```bash
export IS_SECONDARY_VERIFICATION=true
export TWOFA_ALLOWED_DOMAINS="example.com,localhost:8080"
```

### 示例 3：使用外部静态资源

```bash
export STATIC_DIR="/path/to/static/files"
```

或在配置文件中：
```yaml
# app.yaml
static_dir: "/path/to/static/files"
```

### 示例 4：开启容器 Shell 功能

```bash
export IS_OPEN_DOCKER_SHELL=true
```

或在配置文件中：
```yaml
# app.yaml
enable_docker_shell: true
```

## 配置管理最佳实践

### 1. 首次安装后修改配置

```bash
# 1. 安装后启动一次，生成默认配置文件
watch-docker

# 2. 停止服务
# Ctrl+C 或 systemctl stop watch-docker

# 3. 编辑配置文件
nano ~/.watch-docker/app.yaml
nano ~/.watch-docker/config.yaml

# 4. 重启服务
watch-docker
# 或 systemctl start watch-docker
```

### 2. Linux 系统服务配置

**方式 A：修改 systemd 服务文件**

编辑 `/lib/systemd/system/watch-docker.service`：
```ini
[Service]
Environment="USER_NAME=myuser"
Environment="USER_PASSWORD=mypassword"
```

重新加载并重启：
```bash
sudo systemctl daemon-reload
sudo systemctl restart watch-docker
```

**方式 B：使用配置文件（推荐）**

编辑 `~/.watch-docker/app.yaml`，无需修改 systemd 配置。

### 3. Docker 部署配置

**docker-compose.yaml：**

```yaml
services:
  watch-docker:
    environment:
      - USER_NAME=myuser
      - USER_PASSWORD=mypassword
      - IS_SECONDARY_VERIFICATION=true
    volumes:
      - ./app.yaml:/config/app.yaml
      - ./config.yaml:/config/config.yaml
      - CONFIG_PATH=/config
```

### 4. Windows 配置

**方式 A：使用环境变量**

```powershell
# 临时设置
$env:USER_NAME="myuser"
$env:USER_PASSWORD="mypassword"
.\watch-docker.exe

# 永久设置（系统环境变量）
[System.Environment]::SetEnvironmentVariable('USER_NAME', 'myuser', 'User')
[System.Environment]::SetEnvironmentVariable('USER_PASSWORD', 'mypassword', 'User')
```

**方式 B：使用配置文件（推荐）**

创建 `%USERPROFILE%\.watch-docker\app.yaml`：
```yaml
username: "myuser"
password: "mypassword"
```

## 配置验证

### 检查当前配置

启动应用后，日志会显示加载的配置：
```
INFO: 配置文件路径: /Users/username/.watch-docker/config.yaml
INFO: 应用配置路径: /Users/username/.watch-docker/app.yaml
INFO: 服务器监听: :8080
```

### 配置文件示例

完整的配置文件模板位于：
- 源码：`backend/internal/config/config.go`
- 示例：创建 `~/.watch-docker/app.yaml` 和 `config.yaml` 并参考上述格式

## 常见问题

### Q1: 如何查看当前使用的配置？

A: 查看日志输出，或访问 API（如果有配置查看接口）。

### Q2: 配置文件不存在会怎样？

A: 应用会使用默认值和环境变量，首次运行时会自动创建配置目录。

### Q3: 环境变量和配置文件同时设置怎么办？

A: 环境变量优先级更高，会覆盖配置文件中的值。

### Q4: 修改配置后需要重启吗？

A: 是的，配置在应用启动时加载，修改后需要重启服务。

### Q5: 如何备份配置？

A: 备份 `~/.watch-docker/` 目录即可，包含所有配置和数据。

## 安全建议

1. **保护配置文件权限**
   ```bash
   chmod 600 ~/.watch-docker/app.yaml
   chmod 644 ~/.watch-docker/config.yaml
   ```

2. **不要在配置文件中存储明文密码**
   - 使用环境变量
   - 或首次登录后立即修改密码

3. **限制配置目录访问**
   ```bash
   chmod 700 ~/.watch-docker
   ```

4. **定期更新密码**
   ```bash
   # 编辑配置文件
   nano ~/.watch-docker/app.yaml
   # 重启服务
   systemctl restart watch-docker
   ```

## 配置迁移

### 从 Docker 迁移到原生安装

1. 导出 Docker 配置（如有）
2. 创建 `~/.watch-docker/app.yaml` 和 `config.yaml`
3. 将配置值复制到新文件
4. 启动原生应用

### 从旧版本升级

配置文件格式兼容，直接覆盖安装即可，配置文件会保留。

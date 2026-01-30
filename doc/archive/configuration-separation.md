# 配置文件分离优化总结

## 问题

用户指出原有设计将应用环境配置（用户名、密码等）和 Docker 业务配置（扫描、通知等）混在同一个 `config.yaml` 文件中，职责不清晰。

## 解决方案

### 配置文件分离

将配置拆分为两个独立文件，职责明确：

#### 1. `app.yaml` - 应用环境配置
```yaml
# 应用运行时环境配置
username: "admin"
password: "admin"
enable_2fa: false
twofa_allowed_domains: ""
static_dir: ""
enable_docker_shell: false
app_path: ""
version: "v0.1.6"
```

**用途**：
- 用户认证信息
- 功能开关
- 应用环境设置
- 与环境变量对应（USER_NAME, USER_PASSWORD 等）

#### 2. `config.yaml` - Docker 业务配置
```yaml
# Docker 相关业务逻辑配置
server:
  addr: ":8080"

docker:
  host: ""
  includeStopped: false

scan:
  cron: "0 */1 * * *"
  concurrency: 5
  cacheTTL: 60

policy:
  skipLabels: []
  floatingTags: ["latest"]

registry:
  auth: []

notify:
  url: ""
  method: "POST"
```

**用途**：
- HTTP 服务器配置
- Docker 连接配置
- 容器扫描策略
- Registry 认证
- 通知配置

### 代码实现

#### backend/internal/conf/envConfig.go

```go
type EnvConfig struct {
    CONFIG_PATH  string `default:"~/.watch-docker"`
    CONFIG_FILE  string `default:"config.yaml"`    // Docker 业务配置
    ENV_FILE     string `default:"app.yaml"`       // 应用环境配置（新）
    // ... 其他字段
}

func NewEnvConfig() *EnvConfig {
    // 1. 从环境变量加载
    // 2. 从 app.yaml 加载应用配置
    // 3. 合并配置（环境变量优先）
    // 4. 自动创建示例文件
}
```

**特性**：
- ✅ 只从 `app.yaml` 读取应用环境配置
- ✅ `config.yaml` 保持原有职责，由 viper 管理
- ✅ 环境变量优先级最高
- ✅ 自动生成 `app.yaml.example`
- ✅ 清晰的日志提示

### 配置优先级

```
环境变量 > app.yaml > 默认值  （应用环境配置）
环境变量 > config.yaml > 默认值（Docker 业务配置）
```

### 文件结构

```
~/.watch-docker/
├── app.yaml               # 应用环境配置
├── app.yaml.example       # 应用配置示例
├── config.yaml            # Docker 业务配置
└── config.yaml.example    # 业务配置示例
```

### 用户体验

#### 安装后提示

```
配置文件：
  应用配置: ~/.watch-docker/app.yaml        (用户名、密码、功能开关)
  业务配置: ~/.watch-docker/config.yaml    (Docker 扫描、通知等)

配置示例：
  ~/.watch-docker/app.yaml.example
  ~/.watch-docker/config.yaml.example

⚠️  安全提示：
  1. 请修改应用配置中的默认密码
  2. 编辑 ~/.watch-docker/app.yaml
  3. 修改后重启服务

📝 配置说明：
  - app.yaml    应用配置（用户名、密码、2FA 等）
  - config.yaml 业务配置（扫描、通知、服务器等）
```

#### 配置方式

**方式 1：编辑 app.yaml（推荐）**
```bash
nano ~/.watch-docker/app.yaml
# 修改 username 和 password
sudo systemctl restart watch-docker
```

**方式 2：使用环境变量**
```bash
export USER_NAME="myuser"
export USER_PASSWORD="mypass"
watch-docker
```

**方式 3：混合使用**
```yaml
# app.yaml - 基础配置
username: "admin"
enable_2fa: true
```

```bash
# 环境变量覆盖密码
export USER_PASSWORD="secret"
watch-docker
```

### 打包集成

#### .goreleaser.yml

```yaml
archives:
  files:
    - app.yaml.example      # 应用配置示例
    - config.yaml.example   # 业务配置示例

nfpms:
  contents:
    - src: ./app.yaml.example
      dst: /usr/local/share/watch-docker/app.yaml.example
    - src: ./config.yaml.example
      dst: /usr/local/share/watch-docker/config.yaml.example
```

#### scripts/postinstall.sh

```bash
# 复制并创建两个配置文件
cp app.yaml.example ~/.watch-docker/
cp config.yaml.example ~/.watch-docker/

# 创建默认配置
cp app.yaml.example ~/.watch-docker/app.yaml
cp config.yaml.example ~/.watch-docker/config.yaml
```

### 优势

#### 1. 职责清晰
- ✅ 应用配置（app.yaml）：用户名、密码、功能开关
- ✅ 业务配置（config.yaml）：Docker 扫描、通知、策略

#### 2. 易于理解
- ✅ 用户一眼就知道哪个文件管什么
- ✅ 修改密码只需编辑 app.yaml
- ✅ 调整扫描策略只需编辑 config.yaml

#### 3. 安全性
- ✅ 敏感配置（密码）单独存放
- ✅ 可以对两个文件设置不同权限
- ✅ app.yaml 可以 600，config.yaml 可以 644

#### 4. 灵活性
- ✅ 可以只修改应用配置，不影响业务配置
- ✅ 可以独立备份和恢复
- ✅ 可以用不同方式管理（app.yaml 用环境变量，config.yaml 用文件）

#### 5. 兼容性
- ✅ Docker 用户继续使用环境变量
- ✅ 原生安装用户可选择文件或环境变量
- ✅ 不破坏现有的 config.yaml 结构

### 文件映射

| 配置项 | app.yaml | 环境变量 | 说明 |
|--------|----------|----------|------|
| 用户名 | `username` | `USER_NAME` | 登录用户名 |
| 密码 | `password` | `USER_PASSWORD` | 登录密码 |
| 2FA | `enable_2fa` | `IS_SECONDARY_VERIFICATION` | 双因素认证 |
| 域名白名单 | `twofa_allowed_domains` | `TWOFA_ALLOWED_DOMAINS` | 2FA 域名 |
| 静态资源 | `static_dir` | `STATIC_DIR` | 前端资源路径 |
| Shell | `enable_docker_shell` | `IS_OPEN_DOCKER_SHELL` | 容器终端 |
| 应用路径 | `app_path` | `APP_PATH` | 应用路径 |
| 版本 | `version` | `VERSION_WATCH_DOCKER` | 应用版本 |

### 示例场景

#### 场景 1：修改密码
```bash
# 只需编辑 app.yaml
nano ~/.watch-docker/app.yaml
# 修改 password 字段
systemctl restart watch-docker
```

#### 场景 2：调整扫描间隔
```bash
# 只需编辑 config.yaml
nano ~/.watch-docker/config.yaml
# 修改 scan.cron 字段
systemctl restart watch-docker
```

#### 场景 3：Docker 部署
```yaml
# docker-compose.yaml
services:
  watch-docker:
    environment:
      - USER_NAME=admin       # 应用配置
      - USER_PASSWORD=secret  # 应用配置
    volumes:
      - ./config.yaml:/root/.watch-docker/config.yaml  # 业务配置
```

#### 场景 4：分权管理
```bash
# 安全管理员管理 app.yaml（密码、2FA）
chmod 600 ~/.watch-docker/app.yaml
chown admin:admin ~/.watch-docker/app.yaml

# 运维工程师管理 config.yaml（扫描、通知）
chmod 644 ~/.watch-docker/config.yaml
chown ops:ops ~/.watch-docker/config.yaml
```

### 测试验证

```bash
✅ GoReleaser 构建成功
✅ 压缩包包含两个配置文件：
   - app.yaml.example
   - config.yaml.example
✅ DEB/RPM 将两个文件安装到正确位置
✅ 安装后自动创建两个配置文件
✅ 代码从 app.yaml 读取应用配置
✅ config.yaml 保持原有功能不变
```

### 总结

通过配置文件分离，实现了：

1. **职责分离**：应用配置和业务配置明确分开
2. **易于维护**：修改密码不影响业务配置，反之亦然
3. **灵活性**：可以用不同方式管理不同配置
4. **安全性**：敏感配置可以独立管理权限
5. **兼容性**：不破坏现有功能，向后兼容

现在用户可以：
- ✅ 编辑 `app.yaml` 修改用户名密码
- ✅ 编辑 `config.yaml` 调整 Docker 业务逻辑
- ✅ 使用环境变量覆盖任何配置
- ✅ 清楚地知道每个文件的职责

# 配置系统优化总结

## 问题

用户提出：打包后的应用如何让用户方便地修改配置？Docker 可以用环境变量，但原生安装包需要更友好的配置方式。

## 解决方案

### 1. 多层级配置系统

实现了配置优先级（从高到低）：
```
环境变量 > 配置文件 > 默认值
```

### 2. 配置文件支持

**位置**: `~/.watch-docker/config.yaml`

**格式**: YAML 格式，易于编辑
```yaml
server:
  addr: ":8080"

auth:
  username: "admin"
  password: "admin"
  enable_2fa: false

static:
  dir: ""

docker:
  enable_shell: false
```

### 3. 自动配置文件管理

#### 首次运行
- 自动创建配置目录 `~/.watch-docker/`
- 自动生成示例配置文件 `config.yaml.example`
- 如果配置文件不存在，创建默认配置

#### 安装包安装
- DEB/RPM 安装后自动复制示例配置
- 提示用户修改默认密码
- 提供配置文档位置

### 4. 代码实现

#### backend/internal/conf/envConfig.go
```go
func NewEnvConfig() *EnvConfig {
    // 1. 从环境变量加载（最高优先级）
    // 2. 从配置文件加载（中优先级）
    // 3. 使用默认值（最低优先级）
    // 4. 自动创建配置目录和示例文件
}
```

**特性**:
- ✅ 环境变量优先级最高
- ✅ 配置文件次之
- ✅ 自动扩展 `~` 为用户主目录
- ✅ 自动创建配置目录
- ✅ 自动生成示例配置文件
- ✅ 友好的日志提示

### 5. 打包集成

#### .goreleaser.yml
```yaml
archives:
  files:
    - config.yaml.example        # 配置示例
    - doc/configuration-guide.md # 配置文档

nfpms:
  contents:
    - src: ./config.yaml.example
      dst: /usr/local/share/watch-docker/config.yaml.example
    - src: ./doc/configuration-guide.md
      dst: /usr/local/share/doc/watch-docker/configuration-guide.md
```

#### scripts/postinstall.sh
- 复制示例配置到用户目录
- 创建默认配置文件（如果不存在）
- 提示用户修改默认密码
- 显示配置文件位置

### 6. 文档

#### doc/configuration-guide.md
- 完整的配置说明
- 配置优先级说明
- 所有配置项的详细说明
- 平台特定的配置方法
- 安全建议

#### config.yaml.example
- 带注释的完整配置示例
- 每个选项的说明
- 安全提示

## 用户体验

### Docker 用户
```bash
# 继续使用环境变量（最高优先级）
docker run -e USER_NAME=myuser -e USER_PASSWORD=mypass ...
```

### 原生安装包用户

#### 方式 1：使用配置文件（推荐）
```bash
# 1. 安装后自动生成配置
sudo dpkg -i watch-docker.deb

# 2. 编辑配置文件
nano ~/.watch-docker/config.yaml

# 3. 重启服务
sudo systemctl restart watch-docker
```

#### 方式 2：使用环境变量
```bash
# systemd 服务
sudo systemctl edit watch-docker
# 添加：
[Service]
Environment="USER_NAME=myuser"
Environment="USER_PASSWORD=mypass"

# 或者直接运行
USER_NAME=myuser USER_PASSWORD=mypass watch-docker
```

#### 方式 3：混合使用
```yaml
# config.yaml - 基础配置
server:
  addr: ":8080"
auth:
  username: "admin"
```

```bash
# 环境变量 - 覆盖敏感配置
export USER_PASSWORD="secret_password"
watch-docker
```

## 技术细节

### 配置加载流程

```
启动
 ↓
加载环境变量 (CONFIG_PATH, CONFIG_FILE)
 ↓
扩展 ~ 为用户主目录
 ↓
检查配置文件是否存在
 ↓
[存在] → 读取 YAML 文件 → 合并配置（环境变量优先）
 ↓
[不存在] → 使用默认值 + 环境变量
 ↓
创建配置目录（如果不存在）
 ↓
生成示例配置文件（如果需要）
 ↓
应用启动
```

### 配置映射

| 环境变量 | 配置文件路径 | 默认值 |
|----------|-------------|--------|
| `USER_NAME` | `auth.username` | `admin` |
| `USER_PASSWORD` | `auth.password` | `admin` |
| `IS_SECONDARY_VERIFICATION` | `auth.enable_2fa` | `false` |
| `TWOFA_ALLOWED_DOMAINS` | `auth.allowed_domains` | `""` |
| `STATIC_DIR` | `static.dir` | `""` |
| `IS_OPEN_DOCKER_SHELL` | `docker.enable_shell` | `false` |
| `APP_PATH` | `app.path` | `""` |
| `VERSION_WATCH_DOCKER` | `version` | `v0.1.6` |

### 安全特性

1. **配置文件权限**: 自动设置为 `600` (只有所有者可读写)
2. **配置目录权限**: 设置为 `700` (只有所有者可访问)
3. **密码提示**: 安装后提示用户修改默认密码
4. **示例分离**: 示例配置和实际配置分开，避免覆盖

## 测试验证

### 构建验证
```bash
✅ goreleaser 构建成功
✅ 配置文件已包含在压缩包中
✅ DEB/RPM 包含配置文件和文档
```

### 包内容验证
```
压缩包:
  - config.yaml.example         # 配置示例
  - CONFIGURATION.md            # 配置文档
  - watch-docker                # 主程序

DEB/RPM:
  /usr/local/bin/watch-docker
  /usr/local/share/watch-docker/config.yaml.example
  /usr/local/share/doc/watch-docker/configuration-guide.md
  ~/.watch-docker/config.yaml   # 安装后自动创建
```

## 优势

1. **灵活性**: 支持环境变量、配置文件、默认值三种方式
2. **易用性**: 自动生成配置文件，开箱即用
3. **兼容性**: Docker 用户可继续使用环境变量
4. **安全性**: 配置文件权限控制，密码修改提示
5. **可维护性**: 配置文件可版本控制，便于备份和迁移
6. **文档完善**: 详细的配置说明和示例

## 后续增强建议

1. **配置热加载**: 支持运行时重新加载配置（可选）
2. **配置校验**: 启动时校验配置合法性
3. **配置迁移**: 提供配置迁移工具
4. **Web 界面配置**: 通过 Web 界面修改配置（高级功能）
5. **配置加密**: 支持敏感配置加密存储（可选）

## 总结

通过多层级配置系统，Watch Docker 现在支持：
- ✅ Docker 用户：继续使用环境变量
- ✅ 原生安装包用户：使用配置文件或环境变量
- ✅ 自动化部署：环境变量覆盖配置文件
- ✅ 开发调试：配置文件 + 环境变量灵活组合

配置系统既保持了 Docker 的灵活性，又为原生安装包用户提供了友好的配置文件支持。

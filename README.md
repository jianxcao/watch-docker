<div align="center">

# 🐳 Watch Docker

### 让 Docker 容器管理变得简单而优雅

**一站式 Docker 容器监控 · 智能更新 · 可视化管理**

[![GitHub](https://img.shields.io/github/license/jianxcao/watch-docker)](LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/jianxcao/watch-docker)](https://hub.docker.com/r/jianxcao/watch-docker)
[![GitHub Stars](https://img.shields.io/github/stars/jianxcao/watch-docker)](https://github.com/jianxcao/watch-docker/stargazers)

[快速开始](#-快速开始) · [功能特性](#-主要功能) · [文档](./doc) · [问题反馈](https://github.com/jianxcao/watch-docker/issues)

</div>

---

## 💡 为什么选择 Watch Docker？

你是否遇到过这些痛点？

- ⏰ **手动更新容器太繁琐** - 每次都要 pull、stop、rm、run，步骤多且容易出错
- 👀 **容器状态难以监控** - 不知道哪个容器有更新，哪个正在运行
- 📦 **Compose 项目管理混乱** - 多个项目散落各处，启停重启都要敲命令
- 🔒 **缺少安全的远程管理** - 需要登录服务器才能操作，不方便且不安全
- 📊 **资源使用不透明** - 想知道哪个容器占用资源多需要各种命令

**Watch Docker 让这一切变得简单！**

通过现代化的 Web 界面，你可以：

- ✅ **一键更新** - 自动检测镜像更新，点击即可完成容器更新，失败自动回滚
- ✅ **实时监控** - 容器状态、资源使用、运行日志，一目了然
- ✅ **智能策略** - 灵活的更新策略，保护重要服务不被误更新
- ✅ **统一管理** - Compose 项目、独立容器、镜像，集中管理
- ✅ **安全便捷** - 支持二次验证、Token 认证，随时随地安全访问

## 📖 项目概述

Watch Docker 是一个**现代化的 Docker 容器管理平台**，专为简化 Docker 日常运维而设计。它不仅提供容器监控和镜像更新功能，还集成了 Docker Compose 项目管理、Shell 终端访问、二次验证等企业级特性。

**🎯 核心特性**：

- 🔄 **智能更新** - 自动检测远端镜像更新，支持定时任务和手动触发
- 📊 **实时监控** - WebSocket 实时推送容器状态和资源使用情况
- 🐳 **Compose 管理** - 可视化管理所有 Docker Compose 项目
- 💻 **终端访问** - 内置 Web Shell，无需 SSH 直接管理主机
- 🔐 **企业安全** - 支持 OTP/WebAuthn 二次验证，保护重要操作

**💼 适用场景**：

- 🏠 **家庭服务器** - 管理 NAS 上的各种 Docker 应用（Jellyfin、qBittorrent 等）
- 🔧 **开发测试** - 快速部署和更新测试环境容器
- 📡 **小型服务** - 监控和维护个人/小团队的在线服务
- 🎓 **学习实践** - 了解 Docker 容器生命周期管理最佳实践

## ✨ 主要功能

<table>
<tr>
<td width="50%">

### 🔍 容器监控

📈 **全方位状态追踪**

- 实时监控所有容器运行状态
- 自动检测远端镜像更新
- CPU/内存使用率实时图表
- 容器启停历史记录

💡 _不再需要反复执行 `docker ps` 命令_

</td>
<td width="50%">

### 🔄 智能更新

🎯 **安全可靠的更新机制**

- 一键更新或批量更新
- 支持 Cron 定时自动更新
- 更新失败自动回滚
- 保留原容器配置

💡 _告别手动 pull/stop/rm/run 的繁琐流程_

</td>
</tr>
<tr>
<td width="50%">

### 🎯 策略控制

🛡️ **灵活的更新保护**

- Label 标签控制更新行为
- 自动识别固定版本镜像
- 保护本地构建容器
- Compose 项目智能识别

💡 _确保关键服务不会被误更新_

</td>
<td width="50%">

### 🐳 Compose 管理

📦 **项目级别统一管理**

- 自动发现所有 Compose 项目
- 可视化查看服务状态
- 一键启停/重启项目
- 实时日志流查看

💡 _无需记住各种 docker-compose 命令_

</td>
</tr>
<tr>
<td width="50%">

### 💻 Web 终端

🖥️ **浏览器中的 Shell**

- 无需 SSH 直接访问主机
- 完整的 TTY 终端体验
- 支持彩色输出和中文
- 终端大小自适应调整

💡 _随时随地管理服务器，无需额外工具_

</td>
<td width="50%">

### 🔐 安全认证

🔒 **企业级安全保护**

- OTP 一次性密码验证
- WebAuthn 生物识别登录
- 指纹/Face ID/安全密钥
- 多域名凭据管理

💡 _双重验证保护，账户安全无忧_

</td>
</tr>
</table>

### 🌐 现代化界面

- 📱 **完美响应式** - 桌面、平板、手机全适配
- ⚡ **实时推送** - WebSocket 连接，状态秒级更新
- 🎨 **主题切换** - 亮色/暗色主题随心切换
- 🚀 **流畅体验** - Vue 3 + TypeScript，性能卓越

## ⚠️ 风险提示

在使用本工具前，请仔细阅读以下风险提示：

### 🔐 安全风险

- **高权限访问** - 本工具需要访问 Docker socket (`/var/run/docker.sock`)，这意味着它拥有对宿主机 Docker 守护进程的完全控制权限
- **容器逃逸风险** - 任何能够访问 Docker socket 的容器理论上都可以访问宿主机系统，请确保：
  - 仅在受信任的环境中运行
  - 使用强密码保护 Web 界面
  - 限制网络访问（如使用防火墙规则）
- **Shell 访问风险** - 如果开启 Shell 终端功能，将提供对宿主机的直接命令行访问，这是极其危险的：
  - ⚠️ **切勿在生产环境或公网暴露的环境中开启**
  - ⚠️ 必须使用强密码并启用身份验证
  - ⚠️ 建议通过 VPN 或内网访问
  - ⚠️ 定期审查是否仍需要此功能

### 🔄 更新风险

- **服务中断** - 自动更新容器会导致服务短暂中断，可能影响业务连续性
- **镜像兼容性** - 新版本镜像可能包含破坏性变更，导致应用无法正常运行
- **配置丢失** - 如果容器配置不当（如未正确挂载卷），更新可能导致数据丢失
- **网络变更** - 重建容器可能改变容器的网络配置（如 IP 地址）

### ⚡ 特别注意

- 请勿在生产环境开启过于激进的自动更新策略
- 对于数据库、消息队列等有状态服务，建议设置 `watchdocker.skip=true`
- 更新前请确认新版本的 Release Notes 和变更日志

> **免责声明：本工具仅供学习和测试使用。使用本工具导致的任何直接或间接损失，开发者不承担任何责任。生产环境使用请自行评估风险。**

---

## 🚀 快速开始

### 方式一：Docker Compose（推荐）

只需 3 步，即可启动：

**1. 创建 `docker-compose.yaml` 文件：**

```yaml
services:
  watch-docker:
    image: jianxcao/watch-docker:latest
    container_name: watch-docker
    hostname: watch-docker
    # 自己无法更新自己，会死求的
    labels:
      - "watchdocker.skip=true"
    ports:
      - "8080:8080"
    volumes:
      - /volume1/watch-docker:/config
      # 放置 docker yaml文件的目录，必须左右 2 侧一样
      - /volume1/docker:/volume1/docker
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - TZ=Asia/Shanghai
      - USER_NAME=admin
      - USER_PASSWORD=admin
      - PUID=0
      - PGID=0
      - UMASK=0000
      # 是否开启 shell 功能，危险操作
      - IS_OPEN_DOCKER_SHELL=false
        # 放置 docker yaml文件的目录，注意是所有docker 的目录，不是 watch-docker 的目录
      - APP_PATH=/volume1/docker
      # 是否开启二次验证
      - IS_SECONDARY_VERIFICATION=false
    restart: unless-stopped
```

**2. 启动服务：**

```bash
docker-compose up -d
```

**3. 访问界面：**

打开浏览器访问 `http://localhost:8080`，使用默认账户 `admin/admin` 登录。

🎉 **就这么简单！** 现在你可以开始管理你的 Docker 容器了。

### 方式二：Docker 命令

```bash
docker run -d \
  --name watch-docker \
  --hostname watch-docker \
  --label "watchdocker.skip=true" \
  -p 8080:8080 \
  -v /volume1/watch-docker:/config \
  -v /volume1/docker:/volume1/docker \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e TZ=Asia/Shanghai \
  -e USER_NAME=admin \
  -e USER_PASSWORD=admin \
  -e PUID=0 \
  -e PGID=0 \
  -e UMASK=0000 \
  -e IS_OPEN_DOCKER_SHELL=false \
  -e APP_PATH=/volume1/docker \
  -e IS_SECONDARY_VERIFICATION=false \
  --restart unless-stopped \
  jianxcao/watch-docker:latest
```

> 💡 **提示**：首次访问请使用默认账户 `admin/admin` 登录，建议登录后立即修改密码。

## ⚙️ 配置

### 环境变量

| 变量名                      | 默认值          | 描述                                                  |
| --------------------------- | --------------- | ----------------------------------------------------- |
| `CONFIG_PATH`               | `/config`       | 配置文件目录                                          |
| `CONFIG_FILE`               | `config.yaml`   | 配置文件名                                            |
| `USER_NAME`                 | `admin`         | 登录用户名                                            |
| `USER_PASSWORD`             | `admin`         | 登录密码                                              |
| `TZ`                        | `Asia/Shanghai` | 时区设置                                              |
| `PORT`                      | `8088`          | 服务端口                                              |
| `IS_OPEN_DOCKER_SHELL`      | `false`         | 是否开启 Shell 终端功能（⚠️ 高风险）                  |
| `IS_SECONDARY_VERIFICATION` | `false`         | 是否开启二次验证（OTP/WebAuthn）                      |
| `TWOFA_ALLOWED_DOMAINS`     | ` `             | WebAuthn 允许的域名白名单（逗号分隔，为空则允许所有） |
| `APP_PATH`                  | ` `             | Docker Compose 项目所在目录                           |
| `PUID`                      | `0`             | 运行进程的用户 ID                                     |
| `PGID`                      | `0`             | 运行进程的组 ID                                       |
| `UMASK`                     | `0000`          | 文件权限掩码                                          |

### 配置文件示例

在 `./config/config.yaml` 中配置：

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
```

## 🏷️ 容器标签

通过以下标签控制容器更新行为：

```yaml
# 跳过更新
labels:
  - "watchdocker.skip=true"

# 强制更新（即使是固定版本）
labels:
  - "watchdocker.force=true"

# 在更新开关打开的情况下，只跳过更新，不跳过检测
labels:
  - "watchdocker.skipUpdate=true"
```

## 📚 API 文档

### 主要端点

**容器管理**

- `GET /api/containers` - 获取所有容器状态
- `POST /api/containers/:id/update` - 更新指定容器
- `POST /api/containers/:id/start` - 启动容器
- `POST /api/containers/:id/stop` - 停止容器
- `DELETE /api/containers/:id` - 删除容器
- `POST /api/updates/run` - 批量更新
- `GET /api/images` - 获取镜像列表

**Compose 管理**

- `GET /api/compose` - 获取 Compose 项目列表
- `POST /api/compose/start` - 启动 Compose 项目
- `POST /api/compose/stop` - 停止 Compose 项目
- `POST /api/compose/restart` - 重启 Compose 项目
- `DELETE /api/compose/delete` - 删除 Compose 项目
- `POST /api/compose/create` - 创建 Compose 项目
- `GET /api/compose/logs/ws` - Compose 日志 WebSocket

**终端访问**

- `GET /api/shell/ws` - Shell 终端 WebSocket

**其他**

- `GET /healthz` - 健康检查

### 响应格式

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

## 🐳 Docker Compose 管理

Watch Docker 提供了完整的 Docker Compose 项目管理功能，让你可以通过 Web 界面统一管理所有 Compose 项目。

### 功能特性

- **自动发现** - 自动扫描指定目录，发现所有 `docker-compose.yml` 或 `compose.yml` 文件
- **项目管理** - 启动、停止、重启、删除 Compose 项目
- **状态监控** - 实时查看项目及其服务的运行状态
- **日志查看** - 通过 WebSocket 实时查看项目日志
- **服务详情** - 查看项目中的服务、网络、卷等详细信息

### 配置 Compose 项目路径

Docker 挂载 Compose 项目目录：

```yaml
services:
  watch-docker:
    image: jianxcao/watch-docker:latest
    volumes:
      - ./config:/config
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # 挂载 Compose 项目目录,注意左右 2 侧需要相同
      - /opt/compose-projects:/compose-projects:ro
    environment:
      - /opt/compose-projects
```

### 项目状态

Compose 项目具有以下几种状态：

- **running** - 所有服务都在运行
- **stopped** - 所有服务都已停止
- **partial** - 部分服务在运行
- **error** - 项目存在错误

### 使用建议

1. **目录结构** - 建议将每个 Compose 项目放在独立的目录中
2. **命名规范** - 使用有意义的项目名称和目录名
3. **权限控制** - 确保 Watch Docker 有权限访问 Compose 项目目录
4. **备份配置** - 建议定期备份 Compose 配置文件

## 💻 Shell 终端访问

Watch Docker 提供了通过 Web 界面访问容器主机 Shell 的功能，方便进行调试和管理操作。

### ⚠️ 安全警告

**Shell 功能具有极高的安全风险，请务必仔细阅读以下警告：**

- ⚠️ Shell 访问提供了对宿主机的直接命令行访问权限
- ⚠️ 可以执行任何系统命令，包括危险操作（如删除文件、修改配置等）
- ⚠️ 可以访问所有挂载的 Docker Socket，具有完全的 Docker 控制权限
- ⚠️ 如果被恶意利用，可能导致严重的安全事故
- ⚠️ 仅在完全信任的环境中使用此功能
- ⚠️ 必须使用强密码并启用身份验证

> **严重警告：不要在生产环境或公网暴露的环境中开启此功能！**

### 启用 Shell 功能

Shell 功能默认关闭，需要满足以下条件才能开启：

1. **必须启用身份验证** - 必须设置 `USER_NAME` 和 `USER_PASSWORD`
2. **显式开启功能** - 必须设置环境变量 `IS_OPEN_DOCKER_SHELL=true`

通过环境变量启用：

```bash
docker run -d \
  --name watch-docker \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e USER_NAME=admin \
  -e USER_PASSWORD=your_strong_password \
  -e IS_OPEN_DOCKER_SHELL=true \
  jianxcao/watch-docker:latest
```

或通过 Docker Compose：

```yaml
services:
  watch-docker:
    image: jianxcao/watch-docker:latest
    environment:
      - USER_NAME=admin
      - USER_PASSWORD=your_strong_password
      - IS_OPEN_DOCKER_SHELL=true # 开启 Shell 功能
```

### 功能特性

- **交互式终端** - 完整的 PTY (伪终端) 支持
- **彩色输出** - 支持 ANSI 颜色和格式化输出
- **实时通信** - 基于 WebSocket 的低延迟通信
- **中文支持** - 支持中文字符显示（UTF-8）
- **会话管理** - 自动处理终端大小调整和会话超时

### 访问方式

启用后，在 Web 界面中可以找到 "终端" 或 "Shell" 菜单，点击即可打开交互式终端。

终端使用以下配置：

- **默认 Shell**: 使用系统环境变量 `$SHELL`，如未设置则使用 `/bin/sh`
- **终端类型**: `xterm-256color`
- **字符编码**: `UTF-8 (zh_CN.UTF-8)`
- **心跳检测**: 30 秒
- **会话超时**: 90 秒无活动后断开

### 安全建议

1. **强密码** - 使用复杂的密码，至少 16 位，包含大小写字母、数字和特殊字符
2. **网络隔离** - 使用防火墙规则限制访问 IP 范围
3. **审计日志** - 定期检查系统日志，监控异常活动
4. **最小权限** - 如果可能，使用受限的用户账户而非 root
5. **VPN 访问** - 建议通过 VPN 访问而非直接暴露在公网
6. **定期审查** - 定期审查是否仍需要此功能，不用时应关闭

### 禁用 Shell 功能

如果不再需要 Shell 功能，强烈建议将其关闭：

```bash
# 移除环境变量或设置为 false
-e IS_OPEN_DOCKER_SHELL=false
```

或重启容器时不传递该环境变量。

## 🔐 二次验证（Two-Factor Authentication）

Watch Docker 支持二次验证功能，为您的管理界面提供额外的安全保护。支持两种主流的验证方式。

### 功能特性

Watch Docker 提供了两种二次验证方式：

1. **OTP（一次性密码）**

   - 基于 TOTP（时间型一次性密码）协议
   - 支持 Google Authenticator、Authy、微软身份验证器等应用
   - 30 秒刷新一次的 6 位验证码
   - 扫描二维码即可快速设置

2. **WebAuthn（生物验证）**
   - 基于 FIDO2 标准的硬件认证
   - 支持指纹识别、Face ID、Windows Hello
   - 支持硬件安全密钥（如 YubiKey）
   - 多域名凭据管理，适配不同访问场景
   - 防钓鱼攻击，安全性更高

### 启用二次验证

在 Docker Compose 配置中设置环境变量：

```yaml
services:
  watch-docker:
    image: jianxcao/watch-docker:latest
    environment:
      - USER_NAME=admin
      - USER_PASSWORD=your_strong_password
      - IS_SECONDARY_VERIFICATION=true # 启用二次验证
      - TWOFA_ALLOWED_DOMAINS= # 可选：WebAuthn 域名白名单
```

或使用 Docker 命令：

```bash
docker run -d \
  --name watch-docker \
  -e USER_NAME=admin \
  -e USER_PASSWORD=your_strong_password \
  -e IS_SECONDARY_VERIFICATION=true \
  jianxcao/watch-docker:latest
```

### 首次设置流程

1. **启用功能后首次登录**

   - 输入用户名和密码
   - 系统提示设置二次验证

2. **选择验证方式**

   **OTP 方式：**

   - 选择"OTP (一次性密码)"
   - 点击"生成二维码"
   - 使用身份验证器应用扫描二维码
   - 输入应用显示的 6 位验证码
   - 完成设置

   **WebAuthn 方式：**

   - 选择"WebAuthn (生物验证)"
   - 点击"开始设置"
   - 按照浏览器提示完成生物识别注册
   - 完成设置

3. **后续登录**
   - 输入用户名和密码后
   - 根据设置的方式完成二次验证

### 使用说明

#### OTP 验证

- 每次登录时打开身份验证器应用
- 输入当前显示的 6 位验证码
- 验证码每 30 秒自动更新
- 建议在多个设备上添加相同密钥作为备份

#### WebAuthn 验证

- 每次登录时点击"使用生物验证"按钮
- 按照浏览器提示完成生物识别
- 支持在不同域名下注册不同的凭据
- 浏览器要求：Chrome/Firefox/Safari/Edge 最新版本

### 配置存储

二次验证配置保存在主配置文件中：

```yaml
# config.yaml
twofa:
  users:
    admin:
      method: "otp" # 或 "webauthn"
      otpSecret: "BASE32_SECRET" # OTP 密钥
      webauthnCredentials: [] # WebAuthn 凭据列表
      isSetup: true
```

**注意**：配置文件包含敏感信息，请妥善保管，不要泄露给他人。

### 管理二次验证

在 Web 界面的"系统设置"页面可以：

- 查看当前二次验证状态
- 查看已设置的验证方式
- 禁用二次验证（需要完整登录后操作）

### 安全建议

1. **OTP 密钥备份**

   - 首次设置时保存二维码截图或密钥
   - 可以在多个设备上添加相同的密钥
   - 密钥丢失将无法登录，需要重新配置

2. **WebAuthn 设备管理**

   - 建议注册多个生物识别设备
   - 定期检查已注册的凭据
   - 更换设备时记得重新设置

3. **强密码配合**

   - 二次验证是在密码基础上的额外保护
   - 仍需设置强密码（建议 16 位以上）
   - 两层防护共同保障账户安全

4. **域名白名单**（WebAuthn）
   - 生产环境建议配置 `TWOFA_ALLOWED_DOMAINS`
   - 限制只有可信域名可以使用 WebAuthn
   - 多个域名用逗号分隔，如：`example.com,app.example.com`

### 故障排除

**OTP 验证码错误**

- 确保设备时间准确（OTP 基于时间生成）
- 等待下一个验证码再试
- 检查是否输入了正确的 6 位数字

**WebAuthn 不可用**

- 确认浏览器支持 WebAuthn
- 检查是否已开启生物识别功能
- 某些浏览器在 HTTP 环境下限制 WebAuthn（建议使用 HTTPS）
- 清除浏览器缓存后重试

**无法登录**

- 确认 `IS_SECONDARY_VERIFICATION` 环境变量正确设置
- 检查配置文件是否被意外修改
- 如果完全无法访问，可以临时禁用二次验证环境变量后重新设置

### 禁用二次验证

如果不再需要二次验证功能：

1. **在 Web 界面中禁用**（推荐）

   - 登录后进入"系统设置"
   - 点击"禁用二次验证"按钮

2. **通过环境变量禁用**
   ```bash
   IS_SECONDARY_VERIFICATION=false
   ```
   或移除该环境变量，然后重启容器

**注意**：禁用后需要重新设置才能再次启用。

---

## 🔧 开发

### 🛠️ 技术栈

<table>
<tr>
<td width="50%" valign="top">

**后端架构**

- 🔷 **Go 1.25+** - 高性能并发处理
- 🚀 **Gin** - 轻量级 Web 框架
- 🐳 **Docker SDK** - 原生容器 API
- 📝 **Zap** - 高性能结构化日志
- ⏰ **Cron** - 灵活的任务调度
- 🔐 **Viper + Envconfig** - 配置管理
- 🔑 **OTP** - TOTP 一次性密码
- 🛡️ **WebAuthn** - FIDO2 生物识别

</td>
<td width="50%" valign="top">

**前端架构**

- ⚡ **Vue 3** - 组合式 API
- 📘 **TypeScript** - 类型安全
- 🎨 **Naive UI** - 精美组件库
- 🗂️ **Pinia** - 现代化状态管理
- ⚙️ **Vite** - 极速构建工具
- 💅 **UnoCSS** - 原子化 CSS
- 🔐 **SimpleWebAuthn** - WebAuthn 客户端
- 📱 **PWA** - 渐进式 Web 应用

</td>
</tr>
</table>

### 本地开发

1. **克隆仓库**

```bash
git clone https://github.com/jianxcao/watch-docker.git
cd watch-docker
```

2. **启动后端**

```bash
cd backend
go mod download
go run cmd/watch-docker/main.go
```

3. **启动前端**

```bash
cd frontend
pnpm install
pnpm dev
```

4. **构建**

```bash
# 后端构建
cd backend && go build -o watch-docker cmd/watch-docker/main.go

# 前端构建
cd frontend && pnpm build

# Docker 构建
docker build -t watch-docker .
```

---

## 🤝 贡献

我们欢迎所有形式的贡献！无论是新功能、Bug 修复、文档改进还是建议，都非常感谢。

### 如何贡献

1. 🍴 **Fork** 本仓库
2. 🔀 **创建**特性分支 (`git checkout -b feature/amazing-feature`)
3. 💾 **提交**改动 (`git commit -m 'Add some amazing feature'`)
4. 📤 **推送**到分支 (`git push origin feature/amazing-feature`)
5. 🎉 **创建** Pull Request

### 贡献方向

- 🐛 报告 Bug 和安全问题
- 💡 提出新功能建议
- 📝 改进文档和示例
- 🌍 添加多语言支持
- ✨ 优化 UI/UX 设计

---

## 📄 开源协议

本项目采用 [MIT 许可证](LICENSE) - 自由使用、修改和分发。

---

## 🙏 致谢

- 💡 灵感来源于 [Watchtower](https://github.com/containrrr/watchtower) 项目
- 🎨 UI 设计参考了现代化 Docker 管理工具的最佳实践
- 👥 感谢所有贡献者的支持和反馈

---

## 📞 获取帮助

<table>
<tr>
<td width="33%" align="center">

### 📝 提交问题

[创建 Issue](https://github.com/jianxcao/watch-docker/issues)

报告 Bug 或提出功能建议

</td>
<td width="33%" align="center">

### 📚 查看文档

[阅读 Wiki](https://github.com/jianxcao/watch-docker/wiki)

详细使用指南和最佳实践

</td>
<td width="33%" align="center">

### 💬 参与讨论

[Discussions](https://github.com/jianxcao/watch-docker/discussions)

与社区交流使用心得

</td>
</tr>
</table>

---

<div align="center">

## ⭐ Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=jianxcao/watch-docker&type=Date)](https://star-history.com/#jianxcao/watch-docker&Date)

### 如果这个项目对你有帮助，请点个 ⭐ Star 支持一下！

**你的 Star 是对我们最大的鼓励** 🙏

</div>

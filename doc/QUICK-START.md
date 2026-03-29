# 🚀 Watch Docker 快速开始指南

5 分钟快速上手 Watch Docker！

## 📋 概述

Watch Docker 是一个现代化的 Docker 容器管理平台，提供：

- ✅ **智能更新** - 自动检测并更新容器镜像
- ✅ **实时监控** - 容器状态和资源使用情况
- ✅ **Compose 管理** - 可视化管理 Docker Compose 项目
- ✅ **Web 界面** - 简洁美观的管理界面

## 1️⃣ 安装（2 分钟）

### Linux (推荐)

```bash
# Debian/Ubuntu
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_amd64.deb
sudo dpkg -i watch-docker_*.deb
sudo systemctl enable --now watch-docker@$USER

# RHEL/CentOS/Fedora
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_amd64.rpm
sudo rpm -i watch-docker_*.rpm
sudo systemctl enable --now watch-docker@$USER
```

### macOS

```bash
# Homebrew
brew tap jianxcao/tap
brew install watch-docker
brew services start watch-docker
```

### Windows

1. 下载 [WatchDocker-Setup.exe](https://github.com/jianxcao/watch-docker/releases/latest)
2. 双击安装
3. 访问 http://localhost:8080

### Docker

```bash
# 创建 docker-compose.yaml
cat > docker-compose.yaml <<'EOF'
services:
  watch-docker:
    image: jianxcao/watch-docker:latest
    container_name: watch-docker
    labels:
      - "watchdocker.skip=true"
    ports:
      - "8080:8080"
    volumes:
      - ./config:/config
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - TZ=Asia/Shanghai
      - USER_NAME=admin
      - USER_PASSWORD=admin
    restart: unless-stopped
EOF

# 启动服务
docker-compose up -d
```

> 📖 详细安装指南：[installation.md](./user-guide/installation.md)

## 2️⃣ 访问界面（30 秒）

1. 打开浏览器访问：http://localhost:8080
2. 使用默认账户登录：
   - 用户名：`admin`
   - 密码：`admin`

> ⚠️ **安全提示**：首次登录后请立即修改默认密码！

## 3️⃣ 基础配置（2 分钟）

### 修改默认密码

1. 点击右上角用户图标
2. 选择"修改密码"
3. 输入新密码并保存

### 配置自动更新

1. 进入"系统设置"页面
2. 找到"更新策略"配置
3. 设置自动更新时间（如：每天凌晨 3 点）
4. 保存配置

### 容器排除策略

如果某些容器不希望被自动更新，添加标签：

```yaml
services:
  my-app:
    image: my-app:latest
    labels:
      - "watchdocker.skip=true" # 跳过自动更新
```

> 📖 详细配置指南：[configuration.md](./user-guide/configuration.md)

## 4️⃣ 核心功能（30 秒）

### 容器管理

- **查看容器列表** - 主页显示所有容器状态
- **更新容器** - 点击"更新"按钮即可更新
- **批量更新** - 选择多个容器批量更新
- **启停容器** - 一键启动/停止容器

### 镜像管理

- **查看镜像** - 查看本地所有镜像
- **删除镜像** - 清理不再使用的镜像
- **检测更新** - 手动检测镜像更新

### Compose 管理

- **项目列表** - 查看所有 Compose 项目
- **启停项目** - 一键启停整个项目
- **查看日志** - 实时查看项目日志

## 5️⃣ 常见场景

### 场景 1：自动更新所有容器

```yaml
# config.yaml
update:
  enabled: true
  autoUpdateCron: "0 3 * * *" # 每天凌晨 3 点
  allowComposeUpdate: false # 不自动更新 Compose 容器
```

### 场景 2：保护重要服务

```yaml
services:
  database:
    image: postgres:15
    labels:
      - "watchdocker.skip=true" # 数据库不自动更新

  app:
    image: myapp:latest
    # 没有 skip 标签，会自动更新
```

### 场景 3：启用二次验证

```bash
# 修改 docker-compose.yaml
environment:
  - IS_SECONDARY_VERIFICATION=true

# 重启服务
docker-compose restart
```

> 📖 更多场景：[2fa.md](./user-guide/2fa.md)

## 📚 下一步

现在你已经完成了基础设置，可以：

| 我想...             | 查看文档                                  |
| ------------------- | ----------------------------------------- |
| 🔐 启用双因素认证   | [二次验证指南](./user-guide/2fa.md)       |
| ⚙️ 了解所有配置选项 | [配置指南](./user-guide/configuration.md) |
| 🌐 管理容器网络     | [网络功能](./features/network.md)         |
| 💾 管理存储卷       | [Volume 管理](./features/volume.md)       |
| 🏗️ 了解系统架构     | [架构设计](./developer/architecture.md)   |
| 💻 参与开发         | [开发者文档](./developer/)                |

## ❓ 遇到问题？

### 常见问题

<details>
<summary><b>无法连接到 Docker</b></summary>

检查 Docker 是否正在运行：

```bash
docker ps
```

确保 Docker Socket 权限正确：

```bash
sudo usermod -aG docker $USER
newgrp docker
```

</details>

<details>
<summary><b>端口被占用</b></summary>

修改配置文件中的端口：

```yaml
server:
  addr: ":8088" # 改为其他端口
```

或修改 Docker Compose 端口映射：

```yaml
ports:
  - "8088:8080"
```

</details>

<details>
<summary><b>无法自动更新</b></summary>

检查更新配置：

```yaml
update:
  enabled: true # 确保已启用
```

查看容器标签：

```bash
docker inspect <container> | grep watchdocker
```

</details>

### 获取帮助

- 📖 [完整文档](./README.md)
- 🐛 [报告问题](https://github.com/jianxcao/watch-docker/issues)
- 💬 [讨论区](https://github.com/jianxcao/watch-docker/discussions)

## 🎯 安全提示

Watch Docker 需要访问 Docker Socket，这是一个高权限操作。请注意：

- ⚠️ 使用强密码保护 Web 界面
- ⚠️ 不要在公网直接暴露（使用 VPN 或反向代理）
- ⚠️ 数据库等有状态服务建议设置 `watchdocker.skip=true`
- ⚠️ Shell 功能仅在完全信任的环境中启用

## 📊 功能矩阵

| 功能         | 社区版 | 说明                    |
| ------------ | ------ | ----------------------- |
| 容器监控     | ✅     | 实时状态和资源监控      |
| 镜像更新     | ✅     | 自动检测和更新镜像      |
| Compose 管理 | ✅     | 项目级别管理            |
| Web 终端     | ✅     | 可选开启（高风险）      |
| 二次验证     | ✅     | OTP/WebAuthn 双因素认证 |
| 批量操作     | ✅     | 批量更新/启停容器       |
| 定时任务     | ✅     | Cron 表达式定时更新     |
| API 接口     | ✅     | RESTful API             |
| WebSocket    | ✅     | 实时数据推送            |

## 🎉 完成！

恭喜你完成了 Watch Docker 的快速设置！

现在你可以：

- ✅ 监控所有容器状态
- ✅ 一键更新容器镜像
- ✅ 管理 Docker Compose 项目
- ✅ 通过 Web 界面管理 Docker

享受 Watch Docker 带来的便利吧！🚀

---

<div align="center">

**[返回文档中心](./README.md)** | **[项目主页](../README.md)** | **[GitHub](https://github.com/jianxcao/watch-docker)**

如果觉得有用，请给项目点个 ⭐ Star！

</div>

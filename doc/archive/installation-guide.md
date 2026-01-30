# Watch Docker 安装指南

本指南详细说明如何在不同平台上安装 Watch Docker。

## 目录

- [系统要求](#系统要求)
- [Linux 安装](#linux-安装)
- [macOS 安装](#macos-安装)
- [Windows 安装](#windows-安装)
- [从源代码构建](#从源代码构建)
- [服务管理](#服务管理)
- [卸载](#卸载)
- [故障排除](#故障排除)

## 系统要求

### 必需条件

- **Docker**: 版本 20.10 或更高
- **操作系统**:
  - Linux: Debian 10+, Ubuntu 20.04+, RHEL 8+, CentOS 8+
  - macOS: 10.15 (Catalina) 或更高
  - Windows: Windows 10/11, Windows Server 2019+
- **架构**: AMD64 (x86_64) 或 ARM64

### 推荐配置

- CPU: 2 核心或以上
- 内存: 512MB 或以上
- 磁盘: 100MB 可用空间

## Linux 安装

### Debian/Ubuntu (DEB 包)

#### 1. 下载安装包

```bash
# AMD64
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_x86_64.deb

# ARM64
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_arm64.deb
```

#### 2. 安装

```bash
sudo dpkg -i watch-docker_*.deb
```

如果遇到依赖问题：

```bash
sudo apt-get install -f
```

#### 3. 启动服务

```bash
# 启用服务（开机自启）
sudo systemctl enable watch-docker@$USER

# 立即启动服务
sudo systemctl start watch-docker@$USER

# 查看服务状态
sudo systemctl status watch-docker@$USER
```

### RHEL/CentOS/Fedora (RPM 包)

#### 1. 下载安装包

```bash
# AMD64
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_x86_64.rpm

# ARM64
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_arm64.rpm
```

#### 2. 安装

```bash
sudo rpm -i watch-docker_*.rpm
```

或使用 yum/dnf：

```bash
sudo yum install watch-docker_*.rpm
# 或
sudo dnf install watch-docker_*.rpm
```

#### 3. 启动服务

```bash
sudo systemctl enable watch-docker@$USER
sudo systemctl start watch-docker@$USER
sudo systemctl status watch-docker@$USER
```

### 通用二进制安装（所有 Linux 发行版）

```bash
# 下载对应架构的二进制
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_linux_x86_64.tar.gz

# 解压
tar -xzf watch-docker_*.tar.gz

# 安装到系统路径
sudo install -m 755 watch-docker /usr/local/bin/

# 创建配置目录
mkdir -p ~/.watch-docker

# 直接运行
watch-docker
```

### 一键安装脚本

```bash
curl -fsSL https://raw.githubusercontent.com/jianxcao/watch-docker/main/scripts/install.sh | bash
```

脚本功能：

- 自动检测系统和架构
- 下载对应的二进制文件
- 安装到系统路径
- 可选安装 systemd 服务
- 创建配置目录

## macOS 安装

### Homebrew (推荐)

```bash
# 添加 tap
brew tap jianxcao/tap

# 安装
brew install watch-docker

# 启动服务（可选）
brew services start watch-docker
```

### 手动安装

#### 1. 下载二进制

```bash
# Intel Mac
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_darwin_x86_64.tar.gz

# Apple Silicon (M1/M2/M3)
wget https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_darwin_arm64.tar.gz
```

#### 2. 解压和安装

```bash
tar -xzf watch-docker_*.tar.gz
sudo install -m 755 watch-docker /usr/local/bin/
```

#### 3. 创建配置目录

```bash
mkdir -p ~/.watch-docker
```

#### 4. 运行

```bash
watch-docker
```

### 安装为 launchd 服务

```bash
# 创建 plist 文件
cat > ~/Library/LaunchAgents/com.watchdocker.plist <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.watchdocker</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/watch-docker</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>CONFIG_PATH</key>
        <string>~/.watch-docker</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>~/.watch-docker/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>~/.watch-docker/stderr.log</string>
</dict>
</plist>
EOF

# 加载服务
launchctl load ~/Library/LaunchAgents/com.watchdocker.plist

# 启动服务
launchctl start com.watchdocker
```

## Windows 安装

### 图形界面安装（推荐）

1. 从 [Releases](https://github.com/jianxcao/watch-docker/releases/latest) 下载 `WatchDocker-Setup.exe`
2. 双击运行安装程序
3. 按照向导完成安装
4. 安装完成后，在开始菜单找到 "Watch Docker"
5. 可选择安装为 Windows 服务

### 命令行安装

#### 1. 下载二进制

```powershell
# 在 PowerShell 中下载
Invoke-WebRequest -Uri "https://github.com/jianxcao/watch-docker/releases/latest/download/watch-docker_*_windows_x86_64.zip" -OutFile "watch-docker.zip"

# 解压
Expand-Archive -Path watch-docker.zip -DestinationPath "C:\Program Files\WatchDocker"
```

#### 2. 创建配置目录

```powershell
New-Item -ItemType Directory -Path "$env:USERPROFILE\.watch-docker" -Force
```

#### 3. 运行

```powershell
cd "C:\Program Files\WatchDocker"
.\watch-docker.exe
```

### 安装为 Windows 服务

#### 使用安装脚本（需要管理员权限）

```powershell
# 以管理员身份运行 PowerShell
cd "C:\Program Files\WatchDocker\scripts"
.\install-service.ps1
```

#### 手动使用 NSSM

如果安装了 NSSM (Non-Sucking Service Manager)：

```powershell
# 安装 NSSM (使用 Chocolatey)
choco install nssm

# 创建服务
nssm install WatchDocker "C:\Program Files\WatchDocker\watch-docker.exe"
nssm set WatchDocker AppDirectory "C:\Program Files\WatchDocker"
nssm set WatchDocker DisplayName "Watch Docker Service"
nssm set WatchDocker Start SERVICE_AUTO_START

# 启动服务
nssm start WatchDocker
```

## 从源代码构建

### 前置要求

- Go 1.25 或更高
- Node.js 22 或更高
- pnpm 8 或更高

### 构建步骤

```bash
# 克隆仓库
git clone https://github.com/jianxcao/watch-docker.git
cd watch-docker

# 构建前端
cd frontend
pnpm install
pnpm build

# 复制前端资源到后端
cd ..
rm -rf backend/internal/api/static
mkdir -p backend/internal/api/static
cp -r frontend/dist/* backend/internal/api/static/

# 构建后端
cd backend
go build -o watch-docker cmd/watch-docker/main.go

# 运行
./watch-docker
```

### 使用 GoReleaser 构建所有平台

```bash
# 安装 GoReleaser
go install github.com/goreleaser/goreleaser@latest

# 构建所有平台
goreleaser build --snapshot --clean

# 生成的二进制在 dist/ 目录
```

## 服务管理

### Linux (systemd)

```bash
# 启动
sudo systemctl start watch-docker@$USER

# 停止
sudo systemctl stop watch-docker@$USER

# 重启
sudo systemctl restart watch-docker@$USER

# 查看状态
sudo systemctl status watch-docker@$USER

# 启用开机自启
sudo systemctl enable watch-docker@$USER

# 禁用开机自启
sudo systemctl disable watch-docker@$USER

# 查看日志
sudo journalctl -u watch-docker@$USER -f
```

### macOS (launchd)

```bash
# 启动
launchctl start com.watchdocker

# 停止
launchctl stop com.watchdocker

# 加载（启用）
launchctl load ~/Library/LaunchAgents/com.watchdocker.plist

# 卸载（禁用）
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist

# 查看日志
tail -f ~/.watch-docker/stdout.log
tail -f ~/.watch-docker/stderr.log
```

### Windows

```powershell
# 启动服务
Start-Service WatchDocker

# 停止服务
Stop-Service WatchDocker

# 重启服务
Restart-Service WatchDocker

# 查看状态
Get-Service WatchDocker

# 查看日志（如果使用 NSSM）
Get-Content "$env:USERPROFILE\.watch-docker\service.log" -Tail 100 -Wait
```

## 卸载

### Linux (DEB/RPM)

```bash
# Debian/Ubuntu
sudo apt remove watch-docker

# RHEL/CentOS/Fedora
sudo rpm -e watch-docker

# 删除配置（可选）
rm -rf ~/.watch-docker
```

### macOS (Homebrew)

```bash
# 停止服务
brew services stop watch-docker

# 卸载
brew uninstall watch-docker

# 删除 tap（可选）
brew untap jianxcao/tap

# 删除配置（可选）
rm -rf ~/.watch-docker
```

### Windows

1. 从"控制面板" > "程序和功能"中卸载
2. 或使用卸载脚本：

```powershell
cd "C:\Program Files\WatchDocker\scripts"
.\uninstall-service.ps1
```

3. 手动删除配置（可选）：

```powershell
Remove-Item -Recurse -Force "$env:USERPROFILE\.watch-docker"
```

### 通用卸载脚本（Linux/macOS）

```bash
curl -fsSL https://raw.githubusercontent.com/jianxcao/watch-docker/main/scripts/uninstall.sh | bash
```

## 故障排除

### 无法连接到 Docker

**问题**：Watch Docker 无法连接到 Docker daemon

**解决方案**：

1. 确认 Docker 正在运行：

   ```bash
   docker ps
   ```

2. 检查 Docker socket 权限（Linux）：

   ```bash
   ls -la /var/run/docker.sock
   sudo usermod -aG docker $USER
   newgrp docker
   ```

3. 设置 DOCKER_HOST 环境变量（如果使用非默认 socket）：
   ```bash
   export DOCKER_HOST=unix:///var/run/docker.sock
   ```

### 端口被占用

**问题**：8080 端口已被其他程序占用

**解决方案**：

修改配置文件 `~/.watch-docker/config.yaml`：

```yaml
server:
  addr: ":8088" # 改为其他端口
```

或设置环境变量：

```bash
export WATCH_SERVER_ADDR=:8088
```

### 服务无法启动

**问题**：systemd 服务启动失败

**解决方案**：

1. 查看详细日志：

   ```bash
   sudo journalctl -u watch-docker@$USER -xe
   ```

2. 检查配置文件权限：

   ```bash
   ls -la ~/.watch-docker
   chmod 755 ~/.watch-docker
   ```

3. 手动运行测试：
   ```bash
   /usr/local/bin/watch-docker
   ```

### Windows 服务问题

**问题**：Windows 服务无法启动

**解决方案**：

1. 检查事件查看器中的错误日志
2. 确认可执行文件路径正确
3. 以管理员身份运行服务安装脚本
4. 安装 NSSM 以获得更好的服务管理

### 权限问题

**问题**：Permission denied 错误

**解决方案**：

1. 确保用户在 docker 组中（Linux）：

   ```bash
   sudo usermod -aG docker $USER
   ```

2. 确保配置目录权限正确：

   ```bash
   chmod 755 ~/.watch-docker
   chmod 644 ~/.watch-docker/config.yaml
   ```

3. 重新登录使组成员身份生效

## 配置说明

配置文件位置：`~/.watch-docker/config.yaml`

主要配置项：

```yaml
server:
  addr: ":8080" # 监听地址和端口

docker:
  host: "unix:///var/run/docker.sock" # Docker socket 路径

scan:
  interval: "10m" # 扫描间隔
  concurrency: 3 # 并发扫描数

update:
  enabled: true
  autoUpdateCron: "0 3 * * *" # 自动更新 cron 表达式

logging:
  level: "info" # 日志级别：debug, info, warn, error
```

环境变量（优先级高于配置文件）：

- `CONFIG_PATH`: 配置目录路径
- `USER_NAME`: 登录用户名
- `USER_PASSWORD`: 登录密码
- `STATIC_DIR`: 静态文件目录（留空使用嵌入资源）
- `IS_OPEN_DOCKER_SHELL`: 是否启用 Shell 功能
- `IS_SECONDARY_VERIFICATION`: 是否启用二次验证

## 获取帮助

- GitHub Issues: https://github.com/jianxcao/watch-docker/issues
- Discussions: https://github.com/jianxcao/watch-docker/discussions
- 文档: https://github.com/jianxcao/watch-docker/tree/main/doc

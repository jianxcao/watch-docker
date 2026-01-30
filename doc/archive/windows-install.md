# Watch Docker - Windows Installation Guide

## 系统要求

- Windows 10/11 或 Windows Server 2019+
- Docker Desktop for Windows 已安装并运行
- 管理员权限（可选，用于安装为服务）

## 快速开始

### 1. 解压文件

将 `watch-docker.exe` 解压到您选择的目录，例如：
```
C:\Program Files\WatchDocker\
```

### 2. 创建配置目录

在用户目录下创建配置文件夹：
```powershell
mkdir %USERPROFILE%\.watch-docker
```

### 3. 运行应用

#### 方式 A：直接运行（推荐快速测试）

双击 `watch-docker.exe` 或在 PowerShell/CMD 中运行：

```powershell
cd "C:\Program Files\WatchDocker"
.\watch-docker.exe
```

#### 方式 B：使用 PowerShell 运行（推荐）

```powershell
# 设置环境变量（可选）
$env:CONFIG_PATH="$env:USERPROFILE\.watch-docker"
$env:USER_NAME="admin"
$env:USER_PASSWORD="admin"

# 启动应用
.\watch-docker.exe
```

#### 方式 C：安装为 Windows 服务

使用管理员权限打开 PowerShell，并运行：

```powershell
# 使用 NSSM 安装服务（推荐）
# 下载 NSSM: https://nssm.cc/download

# 安装服务
nssm install WatchDocker "C:\Program Files\WatchDocker\watch-docker.exe"

# 配置服务参数
nssm set WatchDocker AppDirectory "C:\Program Files\WatchDocker"
nssm set WatchDocker AppEnvironmentExtra CONFIG_PATH=%USERPROFILE%\.watch-docker
nssm set WatchDocker DisplayName "Watch Docker"
nssm set WatchDocker Description "Docker Container Management and Monitoring Tool"
nssm set WatchDocker Start SERVICE_AUTO_START

# 启动服务
nssm start WatchDocker

# 查看服务状态
nssm status WatchDocker

# 停止服务
nssm stop WatchDocker

# 卸载服务
nssm remove WatchDocker confirm
```

或使用 PowerShell 原生方式（不推荐，功能受限）：

```powershell
# 创建服务
New-Service -Name "WatchDocker" `
    -BinaryPathName "C:\Program Files\WatchDocker\watch-docker.exe" `
    -DisplayName "Watch Docker" `
    -Description "Docker Container Management and Monitoring Tool" `
    -StartupType Automatic

# 启动服务
Start-Service -Name "WatchDocker"

# 查看服务状态
Get-Service -Name "WatchDocker"

# 停止服务
Stop-Service -Name "WatchDocker"

# 删除服务
Remove-Service -Name "WatchDocker"
```

### 4. 访问应用

打开浏览器访问：
```
http://localhost:8080
```

默认登录凭证：
- 用户名：`admin`
- 密码：`admin`

## 环境变量配置

可以通过环境变量自定义应用行为：

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `CONFIG_PATH` | `~/.watch-docker` | 配置文件目录 |
| `CONFIG_FILE` | `config.yaml` | 配置文件名 |
| `USER_NAME` | `admin` | 默认用户名 |
| `USER_PASSWORD` | `admin` | 默认密码 |
| `PORT` | `8080` | HTTP 服务端口 |
| `LOG_LEVEL` | `info` | 日志级别 |
| `STATIC_DIR` | `` | 静态资源目录（空=使用嵌入资源） |

**设置环境变量**：

临时设置（当前 PowerShell 会话）：
```powershell
$env:CONFIG_PATH="D:\watch-docker-config"
$env:PORT="9090"
```

永久设置（系统环境变量）：
```powershell
[System.Environment]::SetEnvironmentVariable('CONFIG_PATH', 'D:\watch-docker-config', 'User')
[System.Environment]::SetEnvironmentVariable('PORT', '9090', 'User')
```

## 防火墙配置

如果需要从其他机器访问，需要开放防火墙端口：

```powershell
# 允许入站连接
New-NetFirewallRule -DisplayName "Watch Docker" `
    -Direction Inbound `
    -Protocol TCP `
    -LocalPort 8080 `
    -Action Allow
```

## 卸载

### 卸载服务（如果已安装）

使用 NSSM：
```powershell
nssm stop WatchDocker
nssm remove WatchDocker confirm
```

或使用 PowerShell：
```powershell
Stop-Service -Name "WatchDocker"
Remove-Service -Name "WatchDocker"
```

### 删除文件

1. 删除应用目录：
   ```
   C:\Program Files\WatchDocker\
   ```

2. 删除配置目录（可选，保留您的数据）：
   ```
   %USERPROFILE%\.watch-docker\
   ```

3. 删除环境变量（如果设置过）

## 故障排查

### 问题 1：应用无法启动

**检查 Docker 是否运行**：
```powershell
docker ps
```

如果报错，请确保 Docker Desktop 已启动。

### 问题 2：端口被占用

**检查端口占用**：
```powershell
netstat -ano | findstr :8080
```

**更改端口**：
```powershell
$env:PORT="9090"
.\watch-docker.exe
```

### 问题 3：无法访问配置目录

**检查权限**：
```powershell
icacls %USERPROFILE%\.watch-docker
```

**授予权限**：
```powershell
icacls %USERPROFILE%\.watch-docker /grant "%USERNAME%:(OI)(CI)F"
```

### 问题 4：服务无法启动

**查看服务日志**：

使用 NSSM：
```powershell
nssm status WatchDocker
```

使用事件查看器：
```
Windows Logs > Application
```

### 问题 5：Docker 连接错误

确保 Docker Desktop 的 "Expose daemon on tcp://localhost:2375 without TLS" 选项已启用（不推荐生产环境）。

或者使用命名管道（默认）：
```
npipe:////./pipe/docker_engine
```

## 更新应用

1. 停止服务（如果作为服务运行）
2. 下载新版本
3. 替换 `watch-docker.exe`
4. 重新启动服务或应用

## 获取帮助

- GitHub: https://github.com/jianxcao/watch-docker
- Issues: https://github.com/jianxcao/watch-docker/issues
- 文档: https://github.com/jianxcao/watch-docker/tree/main/doc

## 安全建议

1. **更改默认密码**：首次登录后立即更改密码
2. **限制访问**：只允许可信网络访问
3. **使用 HTTPS**：生产环境建议使用反向代理（如 nginx）添加 HTTPS
4. **定期更新**：保持应用为最新版本

## 许可证

MIT License - 详见 LICENSE 文件

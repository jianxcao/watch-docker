# Watch Docker - Windows 安装指南

## 安装步骤

1. **运行安装程序**
   - 双击 `WatchDocker-Setup.exe`
   - 按照向导完成安装

2. **首次运行**
   ```powershell
   # 在安装目录下运行
   .\watch-docker.exe
   ```

3. **配置文件**
   - 配置文件位置：`%USERPROFILE%\.watch-docker\config.yaml`
   - 可以复制 `config.yaml.example` 作为模板

4. **安装为服务（可选）**
   ```powershell
   # 以管理员身份运行 PowerShell
   cd scripts\windows
   .\install-service.ps1
   ```

## 默认配置

- **访问地址**: http://localhost:8080
- **默认用户名**: admin
- **默认密码**: admin

**重要**: 首次登录后请立即修改默认密码！

## 服务管理

如果安装为 Windows 服务：

```powershell
# 启动服务
Start-Service WatchDocker

# 停止服务
Stop-Service WatchDocker

# 查看服务状态
Get-Service WatchDocker
```

## 卸载

1. **卸载服务**（如果已安装）
   ```powershell
   cd scripts\windows
   .\uninstall-service.ps1
   ```

2. **卸载程序**
   - 通过 Windows 设置 → 应用 → 卸载
   - 或运行安装目录下的卸载程序

## 故障排除

### 端口被占用
如果 8080 端口已被占用，可以修改配置文件：

```yaml
# config.yaml
server:
  addr: ":9090"  # 修改为其他端口
```

### 无法连接 Docker
确保 Docker Desktop 正在运行，并且启用了 "Expose daemon on tcp://localhost:2375 without TLS"（仅限开发环境）。

生产环境建议使用命名管道：
```yaml
docker:
  host: "npipe:////./pipe/docker_engine"
```

## 更多帮助

- 文档: https://github.com/jianxcao/watch-docker
- 问题反馈: https://github.com/jianxcao/watch-docker/issues

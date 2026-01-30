# Systemd 服务配置

本文档说明如何在 Linux 系统上配置 Watch Docker 为 systemd 服务。

## 服务文件类型

Watch Docker 提供两种 systemd 服务文件：

### 1. 标准服务（watch-docker.service）

- **用途**：系统级服务，以 root 用户运行
- **配置目录**：`/root/.watch-docker`
- **启动命令**：`systemctl enable watch-docker`
- **适用场景**：单用户系统，或需要 root 权限管理 Docker

**特点**：
- 简单直接
- 开箱即用
- 推荐大多数用户使用

### 2. 模板服务（watch-docker@.service）

- **用途**：多用户支持，每个用户独立实例
- **配置目录**：`/home/%i/.watch-docker`（%i 是用户名）
- **启动命令**：`systemctl enable watch-docker@username`
- **适用场景**：多用户环境，每个用户有独立配置

**特点**：
- 支持多实例
- 用户隔离
- 适合团队使用

## 安装服务文件

### 方式一：通过安装包（推荐）

DEB/RPM 安装包会自动安装两个服务文件：

```bash
# Debian/Ubuntu
sudo dpkg -i watch-docker_*.deb

# RHEL/CentOS/Fedora
sudo rpm -i watch-docker_*.rpm
```

安装后，服务文件位于：
- `/lib/systemd/system/watch-docker.service`
- `/lib/systemd/system/watch-docker@.service`

### 方式二：手动安装

```bash
# 复制服务文件
sudo cp scripts/systemd/watch-docker.service /lib/systemd/system/
sudo cp scripts/systemd/watch-docker@.service /lib/systemd/system/

# 重载 systemd
sudo systemctl daemon-reload
```

## 服务配置

### 标准服务配置（watch-docker.service）

```ini
[Unit]
Description=Watch Docker Service
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/root/.watch-docker
ExecStart=/usr/local/bin/watch-docker
Restart=always
RestartSec=5s

Environment="CONFIG_PATH=/root/.watch-docker"

# 安全设置
ProtectSystem=strict
ReadWritePaths=/root/.watch-docker
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### 模板服务配置（watch-docker@.service）

```ini
[Unit]
Description=Watch Docker Service for %i
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=%i
Group=%i
WorkingDirectory=%h/.watch-docker
ExecStart=/usr/local/bin/watch-docker
Restart=always
RestartSec=5s

Environment="CONFIG_PATH=%h/.watch-docker"

# 安全设置
ProtectSystem=strict
ReadWritePaths=%h/.watch-docker
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### systemd 变量说明

| 变量 | 含义 | 示例（root） | 示例（alice） |
|------|------|-------------|--------------|
| `%i` | 实例名称 | `root` | `alice` |
| `%I` | 转义的实例名称 | `root` | `alice` |
| `%h` | 用户主目录 | `/root` | `/home/alice` |
| `%u` | 用户名 | `root` | `alice` |
| `%U` | 用户 UID | `0` | `1000` |

**重要**：使用 `%h` 变量可以正确处理所有用户的主目录，包括 root 用户。

## 常见问题修复

### 问题 1：CHDIR 错误（status=200/CHDIR）

**错误现象**：
```bash
$ systemctl status watch-docker@root
Active: activating (auto-restart) (Result: exit-code)
Main PID: 2584 (code=exited, status=200/CHDIR)
```

**原因**：使用了硬编码路径 `/home/%i/.watch-docker`，但 root 用户的主目录是 `/root`，不是 `/home/root`。

**解决方案**：使用 `%h` 变量代替硬编码路径：

```ini
# ❌ 错误
WorkingDirectory=/home/%i/.watch-docker

# ✅ 正确
WorkingDirectory=%h/.watch-docker
```

### 问题 2：服务文件不存在

**错误现象**：
```bash
$ systemctl enable watch-docker@root
Failed to enable unit: Unit watch-docker@root.service does not exist
```

**原因**：`.goreleaser.yml` 只安装了标准服务，未安装模板服务。

**解决方案**：确保同时安装两个服务文件：

```yaml
contents:
  - src: ./scripts/systemd/watch-docker.service
    dst: /lib/systemd/system/watch-docker.service
    type: config
  - src: ./scripts/systemd/watch-docker@.service
    dst: /lib/systemd/system/watch-docker@.service
    type: config
```

### 问题 3：标准服务使用模板变量

**错误现象**：标准服务（`watch-docker.service`）使用了 `%i` 和 `%h` 变量，但这些变量只在模板服务中有效。

**解决方案**：标准服务应使用固定值：

```ini
# ❌ 错误（标准服务中使用模板变量）
User=%i
WorkingDirectory=%h/.watch-docker

# ✅ 正确（标准服务使用固定值）
User=root
WorkingDirectory=/root/.watch-docker
```

## 使用服务

### 标准服务

```bash
# 启用服务（开机自启）
sudo systemctl enable watch-docker

# 启动服务
sudo systemctl start watch-docker

# 查看状态
sudo systemctl status watch-docker

# 停止服务
sudo systemctl stop watch-docker

# 重启服务
sudo systemctl restart watch-docker

# 查看日志
sudo journalctl -u watch-docker -f
```

### 模板服务

```bash
# 启用服务（以当前用户运行）
sudo systemctl enable watch-docker@$USER

# 启动服务
sudo systemctl start watch-docker@$USER

# 查看状态
sudo systemctl status watch-docker@$USER

# 停止服务
sudo systemctl stop watch-docker@$USER

# 查看日志
sudo journalctl -u watch-docker@$USER -f
```

## 验证服务

### 检查服务状态

```bash
# 检查服务是否运行
systemctl is-active watch-docker

# 检查服务是否启用
systemctl is-enabled watch-docker

# 查看详细状态
systemctl status watch-docker
```

### 检查进程

```bash
# 查看进程
ps aux | grep watch-docker

# 查看端口
netstat -tlnp | grep 8080
# 或
ss -tlnp | grep 8080
```

### 测试访问

```bash
# 健康检查
curl http://localhost:8080/healthz

# 访问 Web 界面
curl http://localhost:8080
```

## 故障排查

### 查看服务日志

```bash
# 查看最近日志
sudo journalctl -u watch-docker -n 50

# 实时查看日志
sudo journalctl -u watch-docker -f

# 查看错误日志
sudo journalctl -u watch-docker -p err
```

### 检查配置文件

```bash
# 检查配置目录是否存在
ls -la ~/.watch-docker

# 检查配置文件权限
ls -la ~/.watch-docker/config.yaml

# 检查 Docker socket 权限
ls -la /var/run/docker.sock
```

### 手动测试

```bash
# 停止服务
sudo systemctl stop watch-docker

# 手动运行测试
/usr/local/bin/watch-docker

# 查看输出，确认问题
```

### 常见错误代码

| 代码 | 名称 | 含义 |
|------|------|------|
| 200 | EXIT_CHDIR | 无法切换到工作目录 |
| 201 | EXIT_NICE | 无法设置进程优先级 |
| 202 | EXIT_FDS | 无法设置文件描述符 |
| 203 | EXIT_EXEC | 无法执行命令 |
| 204 | EXIT_MEMORY | 内存不足 |
| 217 | EXIT_USER | 无法设置用户 |

## 最佳实践

### 1. 使用 systemd 变量

✅ **推荐**：使用 systemd 内置变量
```ini
WorkingDirectory=%h/.watch-docker
Environment="CONFIG_PATH=%h/.watch-docker"
```

❌ **避免**：硬编码路径
```ini
WorkingDirectory=/home/%i/.watch-docker  # root 用户会失败
```

### 2. 创建目录的责任

**方案 A：服务自动创建**（推荐）

在应用启动时检查并创建目录：

```go
configPath := os.Getenv("CONFIG_PATH")
if configPath == "" {
    configPath = "~/.watch-docker"
}
os.MkdirAll(expandPath(configPath), 0755)
```

**方案 B：postinstall 脚本创建**

```bash
# postinstall.sh
CONFIG_DIR="${HOME}/.watch-docker"
mkdir -p "$CONFIG_DIR"
chmod 755 "$CONFIG_DIR"
```

### 3. 权限设置

```ini
[Service]
# 限制写入权限
ProtectSystem=strict
# 只允许写入配置目录
ReadWritePaths=%h/.watch-docker
# 其他安全设置
NoNewPrivileges=true
PrivateTmp=true
```

### 4. 环境变量配置

如果需要通过环境变量配置，可以创建覆盖文件：

```bash
# 创建覆盖目录
sudo mkdir -p /etc/systemd/system/watch-docker.service.d/

# 创建覆盖文件
sudo tee /etc/systemd/system/watch-docker.service.d/override.conf <<EOF
[Service]
Environment="USER_NAME=myuser"
Environment="USER_PASSWORD=mypassword"
EOF

# 重载并重启
sudo systemctl daemon-reload
sudo systemctl restart watch-docker
```

## 相关资源

- [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/systemd.service.html)
- [systemd.unit(5) - Specifiers](https://www.freedesktop.org/software/systemd/man/systemd.unit.html#Specifiers)
- [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/systemd.exec.html)

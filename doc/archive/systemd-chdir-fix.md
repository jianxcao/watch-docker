# systemd CHDIR 错误修复

## 错误现象

```bash
$ systemctl status watch-docker@root
Active: activating (auto-restart) (Result: exit-code)
Main PID: 2584 (code=exited, status=200/CHDIR)
```

## 错误分析

### status=200/CHDIR 含义

- **200** = EXIT_CHDIR
- **CHDIR** = Change Directory (切换目录失败)

systemd 无法切换到指定的工作目录。

### 根本原因

`watch-docker@.service` 配置错误：

```ini
[Service]
User=%i
WorkingDirectory=/home/%i/.watch-docker  # ❌ 错误！
```

当使用 `watch-docker@root` 时：
- `%i` = `root`
- `WorkingDirectory` = `/home/root/.watch-docker`

**问题**：root 用户的主目录是 `/root`，不是 `/home/root`！

### 目录对比

| 用户 | 实际主目录 | 错误的路径 | 正确的路径 |
|------|-----------|-----------|-----------|
| root | /root | /home/root | /root |
| alice | /home/alice | /home/alice | /home/alice |
| bob | /home/bob | /home/bob | /home/bob |

对于 root 用户，硬编码 `/home/%i` 会导致路径错误。

## 解决方案

### 使用 %h（用户主目录）

修改 `watch-docker@.service`：

```ini
[Service]
Type=simple
User=%i
WorkingDirectory=%h/.watch-docker  # ✅ 使用 %h 自动获取主目录

# 环境变量配置
Environment="CONFIG_PATH=%h/.watch-docker"  # ✅ 也要修改这里
```

### systemd 变量说明

| 变量 | 含义 | 示例（root） | 示例（alice） |
|------|------|-------------|--------------|
| `%i` | 实例名称 | `root` | `alice` |
| `%I` | 转义的实例名 | `root` | `alice` |
| `%h` | 用户主目录 | `/root` | `/home/alice` |
| `%u` | 用户名 | `root` | `alice` |
| `%U` | 用户 UID | `0` | `1000` |

**关键**：`%h` 会正确处理所有用户的主目录，包括 root。

## 修改内容

### 原配置（错误）

```ini
[Service]
Type=simple
User=%i
WorkingDirectory=/home/%i/.watch-docker  # ❌
Environment="CONFIG_PATH=/home/%i/.watch-docker"  # ❌
```

### 新配置（正确）

```ini
[Service]
Type=simple
User=%i
WorkingDirectory=%h/.watch-docker  # ✅
Environment="CONFIG_PATH=%h/.watch-docker"  # ✅
# 添加安全设置
ProtectSystem=strict
ReadWritePaths=%h/.watch-docker
```

## 验证步骤

### 1. 更新服务文件

```bash
# 更新构建
goreleaser release --snapshot --clean --skip=publish

# 或手动复制
sudo cp scripts/systemd/watch-docker@.service /lib/systemd/system/
```

### 2. 重载 systemd

```bash
sudo systemctl daemon-reload
```

### 3. 测试不同用户

#### 测试 root 用户

```bash
# 创建配置目录
sudo mkdir -p /root/.watch-docker

# 启用服务
sudo systemctl enable watch-docker@root
sudo systemctl start watch-docker@root

# 检查状态
sudo systemctl status watch-docker@root
# 应该看到：Active: active (running) ✅

# 验证工作目录
ps aux | grep watch-docker
# 应该显示进程在运行

# 检查配置目录
ls -la /root/.watch-docker/
```

#### 测试普通用户

```bash
# 创建配置目录
sudo mkdir -p /home/alice/.watch-docker
sudo chown alice:alice /home/alice/.watch-docker

# 启用服务
sudo systemctl enable watch-docker@alice
sudo systemctl start watch-docker@alice

# 检查状态
sudo systemctl status watch-docker@alice
# 应该看到：Active: active (running) ✅
```

### 4. 查看日志

```bash
# root 用户的日志
sudo journalctl -u watch-docker@root -f

# alice 用户的日志
sudo journalctl -u watch-docker@alice -f
```

## 其他 systemd 错误代码

| 代码 | 名称 | 含义 |
|------|------|------|
| 200 | EXIT_CHDIR | 无法切换到工作目录 |
| 201 | EXIT_NICE | 无法设置进程优先级 |
| 202 | EXIT_FDS | 无法设置文件描述符 |
| 203 | EXIT_EXEC | 无法执行命令 |
| 204 | EXIT_MEMORY | 内存不足 |
| 205 | EXIT_LIMITS | 无法设置资源限制 |
| 206 | EXIT_OOM_ADJUST | 无法调整 OOM 分数 |
| 207 | EXIT_SIGNAL_MASK | 无法设置信号掩码 |
| 208 | EXIT_STDIN | 无法设置标准输入 |
| 209 | EXIT_STDOUT | 无法设置标准输出 |
| 210 | EXIT_CHROOT | 无法切换根目录 |
| 211 | EXIT_IOPRIO | 无法设置 I/O 优先级 |
| 212 | EXIT_TIMERSLACK | 无法设置定时器松弛 |
| 213 | EXIT_SECUREBITS | 无法设置安全位 |
| 214 | EXIT_SETSCHEDULER | 无法设置调度器 |
| 215 | EXIT_CPUAFFINITY | 无法设置 CPU 亲和性 |
| 216 | EXIT_GROUP | 无法设置组 |
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
Environment="CONFIG_PATH=/home/%i/.watch-docker"
```

### 2. 创建目录的责任

**方案 A：服务自动创建**（推荐）

在应用启动时检查并创建目录：

```go
// Go 代码示例
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

**方案 C：systemd RuntimeDirectory**

```ini
[Service]
RuntimeDirectory=watch-docker
RuntimeDirectoryMode=0755
# 会创建 /run/watch-docker（临时目录）
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

## 故障排查命令

```bash
# 检查服务状态
systemctl status watch-docker@root

# 查看完整日志
journalctl -u watch-docker@root -n 50

# 检查目录是否存在
ls -la /root/.watch-docker

# 检查权限
ls -ld /root/.watch-docker

# 测试手动切换目录
sudo -u root bash -c "cd /root/.watch-docker && pwd"

# 验证 systemd 变量展开
systemctl show watch-docker@root -p WorkingDirectory

# 检查用户主目录
getent passwd root | cut -d: -f6
```

## 相关资源

- [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/systemd.service.html)
- [systemd.unit(5) - Specifiers](https://www.freedesktop.org/software/systemd/man/systemd.unit.html#Specifiers)
- [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/systemd.exec.html)

## 总结

**问题**：硬编码 `/home/%i` 导致 root 用户服务启动失败

**原因**：root 的主目录是 `/root`，不是 `/home/root`

**解决**：使用 `%h` 变量自动获取正确的主目录路径

**结果**：所有用户（包括 root）的服务都能正常启动 ✅

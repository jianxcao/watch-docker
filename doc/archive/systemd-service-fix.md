# systemd 服务文件修复说明

## 问题描述

用户报告 DEB 包安装后无法启用服务：

```bash
$ systemctl enable watch-docker@root
Failed to enable unit: Unit watch-docker@root.service does not exist
```

## 根本原因

1. **`.goreleaser.yml` 配置问题**：
   - 只安装了 `watch-docker.service`（非模板服务）
   - 未安装 `watch-docker@.service`（模板服务）

2. **`postinstall.sh` 提示错误**：
   - 提示用户使用 `watch-docker@$CURRENT_USER`
   - 但该模板服务文件未被安装

3. **`watch-docker.service` 配置错误**：
   - 使用了模板服务专用的 `%i` 和 `%h` 变量
   - 但它本身不是模板服务（文件名没有 `@`）

## 解决方案

### 1. 修改 `.goreleaser.yml`

同时安装两个服务文件：

```yaml
contents:
  # systemd 服务文件 - 标准服务（推荐）
  - src: ./scripts/systemd/watch-docker.service
    dst: /lib/systemd/system/watch-docker.service
    type: config
  # systemd 服务文件 - 模板服务（多用户）
  - src: ./scripts/systemd/watch-docker@.service
    dst: /lib/systemd/system/watch-docker@.service
    type: config
```

### 2. 修改 `watch-docker.service`

移除模板服务专用变量，使用固定的 root 用户：

```ini
[Service]
Type=simple
User=root
WorkingDirectory=/root/.watch-docker

Environment="CONFIG_PATH=/root/.watch-docker"
# ... 其他配置

ExecStart=/usr/local/bin/watch-docker
```

**原来的错误配置**：
```ini
User=%i           # %i 是模板服务专用
Group=%i
WorkingDirectory=%h/.watch-docker  # %h 是模板服务专用
```

### 3. 更新 `postinstall.sh`

提供三种启动方式供用户选择：

```bash
echo "启动方式（选择其一）："
echo ""
echo "方式 1: 使用标准服务（推荐）"
echo "  sudo systemctl enable watch-docker"
echo "  sudo systemctl start watch-docker"
echo ""
echo "方式 2: 使用用户模板服务"
echo "  sudo systemctl enable watch-docker@$CURRENT_USER"
echo "  sudo systemctl start watch-docker@$CURRENT_USER"
echo ""
echo "方式 3: 直接运行"
echo "  watch-docker"
```

## systemd 服务文件对比

### watch-docker.service（标准服务）

- **用途**：系统级服务，以 root 用户运行
- **配置目录**：`/root/.watch-docker`
- **启动命令**：`systemctl enable watch-docker`
- **适用场景**：单用户系统，或需要 root 权限管理 Docker

**特点**：
- 简单直接
- 开箱即用
- 推荐大多数用户使用

### watch-docker@.service（模板服务）

- **用途**：多用户支持，每个用户独立实例
- **配置目录**：`/home/%i/.watch-docker`（%i 是用户名）
- **启动命令**：`systemctl enable watch-docker@username`
- **适用场景**：多用户环境，每个用户有独立配置

**特点**：
- 支持多实例
- 用户隔离
- 适合团队使用

### systemd 模板服务说明

**模板服务变量**：

- `%i`：实例名称（如 `watch-docker@alice.service` 中的 `alice`）
- `%I`：转义的实例名称
- `%n`：完整的单元名称
- `%N`：不带后缀的单元名称
- `%h`：用户主目录（仅在用户服务中）

**错误示例**：

```ini
# ❌ 错误：在非模板服务中使用模板变量
[Unit]
Description=Watch Docker

[Service]
User=%i        # 不起作用！文件名没有 @
WorkingDirectory=%h/.watch-docker

[Install]
WantedBy=multi-user.target
```

**正确示例**：

```ini
# ✅ 正确：标准服务使用固定值
[Unit]
Description=Watch Docker

[Service]
User=root
WorkingDirectory=/root/.watch-docker

[Install]
WantedBy=multi-user.target
```

## 测试验证

### 验证 DEB 包内容

```bash
# 安装包
sudo dpkg -i watch-docker_0.1.5_linux_amd64.deb

# 检查服务文件
ls -la /lib/systemd/system/watch-docker*
# 应该看到：
# - /lib/systemd/system/watch-docker.service
# - /lib/systemd/system/watch-docker@.service

# 重载 systemd
sudo systemctl daemon-reload

# 测试标准服务
sudo systemctl enable watch-docker
sudo systemctl start watch-docker
sudo systemctl status watch-docker

# 测试模板服务（假设当前用户是 myuser）
sudo systemctl enable watch-docker@myuser
sudo systemctl start watch-docker@myuser
sudo systemctl status watch-docker@myuser
```

### 验证服务运行

```bash
# 检查进程
ps aux | grep watch-docker

# 检查端口
netstat -tlnp | grep 8080

# 访问测试
curl http://localhost:8080
```

## 构建验证

```bash
# 重新构建包
goreleaser release --snapshot --clean --skip=publish

# 检查生成的包
ls -lh dist/*.deb dist/*.rpm

# 查看包大小（应该略有增加，因为多了一个服务文件）
```

## 后续改进建议

1. **考虑使用用户服务**（`systemd --user`）
   - 无需 sudo
   - 更符合现代 systemd 实践
   - 配置文件在 `~/.config/systemd/user/`

2. **添加服务健康检查**
   - 使用 `ExecStartPre` 检查 Docker 可用性
   - 使用 `ExecStartPost` 验证服务启动

3. **完善安装脚本**
   - 自动检测当前用户
   - 提供交互式选择启动方式
   - 自动启动服务（可选）

## 相关文档

- [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/systemd.service.html)
- [systemd.unit(5)](https://www.freedesktop.org/software/systemd/man/systemd.unit.html)
- [systemd Template Units](https://www.freedesktop.org/software/systemd/man/systemd.unit.html#id-1.15)

## 总结

通过这次修复：

1. ✅ 同时提供标准服务和模板服务
2. ✅ 修复 `watch-docker.service` 的配置错误
3. ✅ 更新安装提示，提供多种启动方式
4. ✅ 用户可以根据需求选择合适的服务类型

现在用户可以：
- 使用 `systemctl enable watch-docker` 启动标准服务
- 使用 `systemctl enable watch-docker@username` 启动用户服务
- 或直接运行 `watch-docker` 二进制

问题完全解决！✅

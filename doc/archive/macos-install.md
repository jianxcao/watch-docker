# Watch Docker - macOS 安装指南

## 安装方式

### 方式一：使用 Homebrew（推荐）

```bash
# 添加 tap
brew tap jianxcao/tap

# 安装
brew install watch-docker

# 启动服务
brew services start watch-docker
```

### 方式二：手动安装

1. **下载对应架构的压缩包**
   - Apple Silicon (M1/M2/M3): `watch-docker_*_darwin_arm64.tar.gz`
   - Intel: `watch-docker_*_darwin_x86_64.tar.gz`

2. **解压并安装**
   ```bash
   tar -xzf watch-docker_*_darwin_arm64.tar.gz
   sudo install -m 755 watch-docker /usr/local/bin/
   ```

3. **首次运行**
   ```bash
   # 移除隔离属性（因为未签名）
   xattr -d com.apple.quarantine /usr/local/bin/watch-docker
   
   # 运行
   watch-docker
   ```

## 配置文件

配置文件位置：`~/.watch-docker/config.yaml`

可以复制提供的 `config.yaml.example` 作为模板：

```bash
mkdir -p ~/.watch-docker
cp config.yaml.example ~/.watch-docker/config.yaml
```

## 设置开机自启（使用 launchd）

1. **复制 plist 文件**
   ```bash
   cp com.watchdocker.plist ~/Library/LaunchAgents/
   ```

2. **编辑配置文件**
   ```bash
   nano ~/Library/LaunchAgents/com.watchdocker.plist
   ```
   
   确认二进制路径正确：
   - Apple Silicon: `/opt/homebrew/bin/watch-docker`
   - Intel: `/usr/local/bin/watch-docker`

3. **加载并启动服务**
   ```bash
   launchctl load ~/Library/LaunchAgents/com.watchdocker.plist
   launchctl start com.watchdocker
   ```

## 服务管理命令

```bash
# 启动
launchctl start com.watchdocker

# 停止
launchctl stop com.watchdocker

# 重启
launchctl kickstart -k gui/$(id -u)/com.watchdocker

# 查看状态
launchctl list | grep watchdocker

# 卸载自启
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist
```

## 查看日志

```bash
# 标准输出日志
tail -f ~/.watch-docker/stdout.log

# 错误日志
tail -f ~/.watch-docker/stderr.log
```

## 默认配置

- **访问地址**: http://localhost:8080
- **默认用户名**: admin
- **默认密码**: admin

**重要**: 首次登录后请立即修改默认密码！

## 卸载

### 使用 Homebrew 安装的

```bash
# 停止服务
brew services stop watch-docker

# 卸载
brew uninstall watch-docker

# 移除 tap（可选）
brew untap jianxcao/tap
```

### 手动安装的

```bash
# 停止服务（如果设置了自启）
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist
rm ~/Library/LaunchAgents/com.watchdocker.plist

# 删除二进制文件
sudo rm /usr/local/bin/watch-docker

# 删除配置目录（可选，会删除所有配置）
rm -rf ~/.watch-docker
```

## 故障排除

### Gatekeeper 阻止运行

如果遇到"无法打开，因为无法验证开发者"的错误：

```bash
# 方法一：移除隔离属性
xattr -d com.apple.quarantine /usr/local/bin/watch-docker

# 方法二：系统设置中允许运行
# 系统设置 → 隐私与安全性 → 安全性 → 仍要打开
```

### 无法连接 Docker

确保 Docker Desktop 正在运行：

```bash
docker ps  # 测试 Docker 是否正常
```

## 更多帮助

- 文档: https://github.com/jianxcao/watch-docker
- 问题反馈: https://github.com/jianxcao/watch-docker/issues

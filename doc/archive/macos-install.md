# macOS 安装和配置指南

## 安装 Watch Docker

### 通过 Homebrew 安装（推荐）

```bash
# 添加 tap
brew tap jianxcao/tap

# 安装 watch-docker
brew install watch-docker
```

### 验证安装

```bash
# 查看版本
watch-docker --version

# 查看安装路径
which watch-docker
```

## 配置开机自启动

Watch Docker 作为 Homebrew Cask 安装，不支持 `brew services` 命令。需要手动配置 launchd 来实现开机自启动。

### 方法 1：使用提供的 plist 文件（推荐）

#### 1. 复制 plist 文件

安装包中已经包含了 launchd 配置文件，复制到用户的 LaunchAgents 目录：

```bash
# 查找 watch-docker 的 Cask 目录
CASK_DIR=$(brew --prefix)/Caskroom/watch-docker

# 找到最新版本
VERSION=$(ls -t "$CASK_DIR" | head -n 1)

# 复制 plist 文件
cp "$CASK_DIR/$VERSION/com.watchdocker.plist" ~/Library/LaunchAgents/
```

或者一条命令完成：

```bash
cp "$(brew --prefix)/Caskroom/watch-docker/*/com.watchdocker.plist" ~/Library/LaunchAgents/
```

#### 2. 编辑 plist 文件（可选）

打开配置文件确认二进制路径正确：

```bash
# 使用你喜欢的编辑器打开
nano ~/Library/LaunchAgents/com.watchdocker.plist
# 或
open -a TextEdit ~/Library/LaunchAgents/com.watchdocker.plist
```

确认 `ProgramArguments` 中的路径：

- **Apple Silicon (M1/M2/M3)**：`/opt/homebrew/bin/watch-docker`
- **Intel Mac**：`/usr/local/bin/watch-docker`

你也可以修改环境变量来自定义配置：

```xml
<key>EnvironmentVariables</key>
<dict>
    <key>CONFIG_PATH</key>
    <string>~/.watch-docker</string>
    <key>USER_NAME</key>
    <string>admin</string>
    <key>USER_PASSWORD</key>
    <string>your-password</string>
    <!-- 其他配置... -->
</dict>
```

#### 3. 加载服务

```bash
# 加载 launchd 服务（开机自启）
launchctl load ~/Library/LaunchAgents/com.watchdocker.plist

# 立即启动服务
launchctl start com.watchdocker
```

### 方法 2：手动创建 plist 文件

如果你想自定义配置，可以手动创建 plist 文件：

```bash
# 创建文件
nano ~/Library/LaunchAgents/com.watchdocker.plist
```

粘贴以下内容（根据你的系统架构修改路径）：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.watchdocker</string>

    <key>ProgramArguments</key>
    <array>
        <!-- Apple Silicon: /opt/homebrew/bin/watch-docker -->
        <!-- Intel Mac: /usr/local/bin/watch-docker -->
        <string>/opt/homebrew/bin/watch-docker</string>
    </array>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>CONFIG_PATH</key>
        <string>~/.watch-docker</string>
        <key>USER_NAME</key>
        <string>admin</string>
        <key>USER_PASSWORD</key>
        <string>admin</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>~/.watch-docker/stdout.log</string>

    <key>StandardErrorPath</key>
    <string>~/.watch-docker/stderr.log</string>

    <key>WorkingDirectory</key>
    <string>~/.watch-docker</string>
</dict>
</plist>
```

然后加载服务：

```bash
launchctl load ~/Library/LaunchAgents/com.watchdocker.plist
launchctl start com.watchdocker
```

## 服务管理

### 启动服务

```bash
launchctl start com.watchdocker
```

### 停止服务

```bash
launchctl stop com.watchdocker
```

### 重启服务

```bash
# 方法 1：停止后启动
launchctl stop com.watchdocker
launchctl start com.watchdocker

# 方法 2：使用 kickstart 命令（推荐）
launchctl kickstart -k gui/$(id -u)/com.watchdocker
```

### 查看服务状态

```bash
# 查看服务是否运行
launchctl list | grep watchdocker

# 输出示例：
# 12345  0  com.watchdocker
# 第一列是 PID（如果运行中）
# 第二列是退出代码
```

### 查看日志

```bash
# 查看标准输出日志
tail -f ~/.watch-docker/stdout.log

# 查看错误日志
tail -f ~/.watch-docker/stderr.log
```

### 卸载自启动

```bash
# 卸载 launchd 服务
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist

# 删除 plist 文件（可选）
rm ~/Library/LaunchAgents/com.watchdocker.plist
```

## 访问 Watch Docker

服务启动后，在浏览器中访问：

```
http://localhost:8080
```

默认登录凭据：

- 用户名：`admin`
- 密码：`admin`

**⚠️ 首次登录后请立即修改密码！**

## 配置文件

配置文件位于：`~/.watch-docker/config.yaml`

如果配置文件不存在，可以从示例文件创建：

```bash
# 创建配置目录
mkdir -p ~/.watch-docker

# 从安装包复制示例配置
cp "$(brew --prefix)/Caskroom/watch-docker/*/config.yaml.example" ~/.watch-docker/config.yaml

# 编辑配置
nano ~/.watch-docker/config.yaml
```

## 故障排查

### 服务无法启动

1. **检查服务状态**：

```bash
launchctl list | grep watchdocker
```

2. **查看错误日志**：

```bash
cat ~/.watch-docker/stderr.log
```

3. **验证二进制路径**：

```bash
# 确认 watch-docker 安装路径
which watch-docker

# 确认 plist 中的路径与实际路径一致
grep ProgramArguments -A 1 ~/Library/LaunchAgents/com.watchdocker.plist
```

4. **手动测试运行**：

```bash
# 直接运行看是否有错误
watch-docker
```

### 权限问题

```bash
# 确保配置目录存在且有权限
mkdir -p ~/.watch-docker
chmod 755 ~/.watch-docker
```

### 端口占用

如果 8080 端口被占用，修改配置文件中的端口：

```yaml
# ~/.watch-docker/config.yaml
server:
  port: 8081 # 修改为其他端口
```

### 重置服务

```bash
# 卸载服务
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist

# 等待几秒

# 重新加载
launchctl load ~/Library/LaunchAgents/com.watchdocker.plist

# 启动服务
launchctl start com.watchdocker
```

## 卸载

### 停止并删除服务

```bash
# 停止并卸载服务
launchctl stop com.watchdocker
launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist

# 删除 plist 文件
rm ~/Library/LaunchAgents/com.watchdocker.plist
```

### 卸载 watch-docker

```bash
# 使用 Homebrew 卸载
brew uninstall watch-docker

# 删除 tap（可选）
brew untap jianxcao/tap
```

### 清理配置文件（可选）

```bash
# 删除配置目录
rm -rf ~/.watch-docker
```

## 升级

```bash
# 更新 Homebrew
brew update

# 升级 watch-docker
brew upgrade watch-docker

# 重启服务
launchctl kickstart -k gui/$(id -u)/com.watchdocker
```

## 常见问题

### Q: 为什么不能使用 `brew services`？

A: Watch Docker 是作为 Cask（预编译二进制）发布的，而 `brew services` 只支持 Formula（从源码编译的包）。需要使用 launchd 手动配置自启动。

### Q: 如何更改默认端口？

A: 编辑 `~/.watch-docker/config.yaml` 文件，修改 `server.port` 配置项，然后重启服务。

### Q: 如何查看服务是否在运行？

A: 运行 `launchctl list | grep watchdocker`，如果有输出且第一列有 PID，说明服务正在运行。

### Q: 开机后服务没有自动启动？

A: 检查：

1. plist 文件是否在 `~/Library/LaunchAgents/` 目录下
2. plist 文件中 `RunAtLoad` 是否设置为 `true`
3. 查看系统日志：`log show --predicate 'subsystem == "com.apple.launchd"' --last 5m | grep watchdocker`

## 更多帮助

- 项目主页：https://github.com/jianxcao/watch-docker
- 提交问题：https://github.com/jianxcao/watch-docker/issues
- 文档：查看 `~/.watch-docker/` 目录下的文档文件

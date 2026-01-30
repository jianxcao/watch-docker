#!/bin/bash
# Watch Docker 通用安装脚本
# 支持 Linux 和 macOS

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 打印信息
info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检测操作系统和架构
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case "$ARCH" in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "不支持的架构: $ARCH"
            exit 1
            ;;
    esac
    
    info "检测到系统: $OS-$ARCH"
}

# 检查依赖
check_dependencies() {
    info "检查依赖..."
    
    # 检查 Docker
    if ! command -v docker &> /dev/null; then
        error "Docker 未安装，请先安装 Docker。"
        exit 1
    fi
    
    success "依赖检查通过"
}

# 下载二进制文件
download_binary() {
    info "下载 watch-docker..."
    
    local VERSION="${VERSION:-latest}"
    local BINARY_NAME="watch-docker"
    
    if [ "$OS" = "darwin" ]; then
        BINARY_NAME="watch-docker_darwin_${ARCH}"
    else
        BINARY_NAME="watch-docker_linux_${ARCH}"
    fi
    
    # 如果当前目录有二进制文件，直接使用
    if [ -f "./watch-docker" ]; then
        info "使用当前目录的二进制文件"
        BINARY_PATH="./watch-docker"
        return
    fi
    
    # 否则从 GitHub 下载
    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/jianxcao/watch-docker/releases/latest/download/${BINARY_NAME}.tar.gz"
    else
        DOWNLOAD_URL="https://github.com/jianxcao/watch-docker/releases/download/${VERSION}/${BINARY_NAME}.tar.gz"
    fi
    
    info "下载地址: $DOWNLOAD_URL"
    
    TMP_DIR=$(mktemp -d)
    cd "$TMP_DIR"
    
    if command -v curl &> /dev/null; then
        curl -sL "$DOWNLOAD_URL" -o watch-docker.tar.gz
    elif command -v wget &> /dev/null; then
        wget -q "$DOWNLOAD_URL" -O watch-docker.tar.gz
    else
        error "需要 curl 或 wget 来下载文件"
        exit 1
    fi
    
    tar -xzf watch-docker.tar.gz
    BINARY_PATH="$TMP_DIR/watch-docker"
    
    success "下载完成"
}

# 安装二进制文件
install_binary() {
    info "安装 watch-docker 到 /usr/local/bin/..."
    
    if [ ! -f "$BINARY_PATH" ]; then
        error "找不到二进制文件"
        exit 1
    fi
    
    sudo install -m 755 "$BINARY_PATH" /usr/local/bin/watch-docker
    success "二进制文件安装完成"
}

# 创建配置目录
create_config_dir() {
    CONFIG_DIR="$HOME/.watch-docker"
    
    if [ ! -d "$CONFIG_DIR" ]; then
        info "创建配置目录: $CONFIG_DIR"
        mkdir -p "$CONFIG_DIR"
        success "配置目录创建完成"
    else
        info "配置目录已存在: $CONFIG_DIR"
    fi
}

# 安装 systemd 服务 (Linux)
install_systemd_service() {
    info "安装 systemd 服务..."
    
    # 使用用户服务（不需要 root 权限）
    SERVICE_FILE="$HOME/.config/systemd/user/watch-docker.service"
    mkdir -p "$HOME/.config/systemd/user"
    
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Watch Docker - Docker Container Management
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$HOME/.watch-docker
Environment="CONFIG_PATH=$HOME/.watch-docker"
Environment="USER_NAME=admin"
Environment="USER_PASSWORD=admin"
Environment="STATIC_DIR="
ExecStart=/usr/local/bin/watch-docker
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=default.target
EOF
    
    # 重载 systemd 配置
    systemctl --user daemon-reload
    
    success "systemd 服务已安装"
    info "启用服务: systemctl --user enable watch-docker"
    info "启动服务: systemctl --user start watch-docker"
    info "查看状态: systemctl --user status watch-docker"
}

# 安装 launchd 服务 (macOS)
install_launchd_service() {
    info "安装 launchd 服务..."
    
    PLIST_FILE="$HOME/Library/LaunchAgents/com.watchdocker.plist"
    mkdir -p "$HOME/Library/LaunchAgents"
    
    cat > "$PLIST_FILE" <<EOF
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
        <string>$HOME/.watch-docker</string>
        <key>USER_NAME</key>
        <string>admin</string>
        <key>USER_PASSWORD</key>
        <string>admin</string>
        <key>STATIC_DIR</key>
        <string></string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$HOME/.watch-docker/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>$HOME/.watch-docker/stderr.log</string>
    <key>WorkingDirectory</key>
    <string>$HOME/.watch-docker</string>
</dict>
</plist>
EOF
    
    success "launchd 服务已安装"
    info "加载服务: launchctl load ~/Library/LaunchAgents/com.watchdocker.plist"
    info "启动服务: launchctl start com.watchdocker"
    info "查看状态: launchctl list | grep watchdocker"
}

# 询问是否安装服务
ask_install_service() {
    echo ""
    read -p "是否安装为系统服务（开机自动启动）？ [Y/n] " -n 1 -r
    echo ""
    
    if [[ ! $REPLY =~ ^[Nn]$ ]]; then
        if [ "$OS" = "linux" ]; then
            install_systemd_service
            
            read -p "是否立即启用并启动服务？ [Y/n] " -n 1 -r
            echo ""
            if [[ ! $REPLY =~ ^[Nn]$ ]]; then
                systemctl --user enable watch-docker
                systemctl --user start watch-docker
                success "服务已启动！"
            fi
        elif [ "$OS" = "darwin" ]; then
            install_launchd_service
            
            read -p "是否立即加载并启动服务？ [Y/n] " -n 1 -r
            echo ""
            if [[ ! $REPLY =~ ^[Nn]$ ]]; then
                launchctl load ~/Library/LaunchAgents/com.watchdocker.plist
                success "服务已启动！"
            fi
        fi
    fi
}

# 打印安装信息
print_info() {
    echo ""
    echo "================================================"
    success "Watch Docker 安装完成！"
    echo "================================================"
    echo ""
    info "配置目录: $HOME/.watch-docker"
    info "可执行文件: /usr/local/bin/watch-docker"
    echo ""
    info "快速开始:"
    echo "  1. 直接运行: watch-docker"
    echo "  2. 访问: http://localhost:8080"
    echo "  3. 默认账户: admin / admin"
    echo ""
    
    if [ "$OS" = "linux" ]; then
        info "服务管理命令:"
        echo "  systemctl --user start watch-docker    # 启动"
        echo "  systemctl --user stop watch-docker     # 停止"
        echo "  systemctl --user status watch-docker   # 状态"
        echo "  systemctl --user enable watch-docker   # 开机启动"
    elif [ "$OS" = "darwin" ]; then
        info "服务管理命令:"
        echo "  launchctl load ~/Library/LaunchAgents/com.watchdocker.plist     # 加载"
        echo "  launchctl unload ~/Library/LaunchAgents/com.watchdocker.plist   # 卸载"
        echo "  launchctl start com.watchdocker                                  # 启动"
        echo "  launchctl stop com.watchdocker                                   # 停止"
    fi
    
    echo ""
    warning "首次登录后请立即修改默认密码！"
    echo ""
}

# 主函数
main() {
    echo "================================================"
    info "Watch Docker 安装向导"
    echo "================================================"
    echo ""
    
    detect_platform
    check_dependencies
    download_binary
    install_binary
    create_config_dir
    ask_install_service
    print_info
}

# 运行主函数
main

#!/bin/bash
# Watch Docker 卸载脚本
# 支持 Linux 和 macOS

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

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

# 检测操作系统
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    info "检测到系统: $OS"
}

# 停止并删除 systemd 服务 (Linux)
remove_systemd_service() {
    SERVICE_FILE="$HOME/.config/systemd/user/watch-docker.service"
    
    if [ -f "$SERVICE_FILE" ]; then
        info "停止并删除 systemd 服务..."
        
        # 停止服务
        systemctl --user stop watch-docker 2>/dev/null || true
        # 禁用服务
        systemctl --user disable watch-docker 2>/dev/null || true
        # 删除服务文件
        rm -f "$SERVICE_FILE"
        # 重载配置
        systemctl --user daemon-reload
        
        success "systemd 服务已删除"
    else
        info "未找到 systemd 服务"
    fi
}

# 停止并删除 launchd 服务 (macOS)
remove_launchd_service() {
    PLIST_FILE="$HOME/Library/LaunchAgents/com.watchdocker.plist"
    
    if [ -f "$PLIST_FILE" ]; then
        info "停止并删除 launchd 服务..."
        
        # 停止并卸载服务
        launchctl stop com.watchdocker 2>/dev/null || true
        launchctl unload "$PLIST_FILE" 2>/dev/null || true
        # 删除 plist 文件
        rm -f "$PLIST_FILE"
        
        success "launchd 服务已删除"
    else
        info "未找到 launchd 服务"
    fi
}

# 删除二进制文件
remove_binary() {
    if [ -f "/usr/local/bin/watch-docker" ]; then
        info "删除二进制文件..."
        sudo rm -f /usr/local/bin/watch-docker
        success "二进制文件已删除"
    else
        info "未找到二进制文件"
    fi
}

# 询问是否删除配置
ask_remove_config() {
    CONFIG_DIR="$HOME/.watch-docker"
    
    if [ -d "$CONFIG_DIR" ]; then
        echo ""
        warning "配置目录: $CONFIG_DIR"
        warning "包含配置文件和数据"
        read -p "是否删除配置目录？ [y/N] " -n 1 -r
        echo ""
        
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            info "删除配置目录..."
            rm -rf "$CONFIG_DIR"
            success "配置目录已删除"
        else
            info "保留配置目录"
        fi
    fi
}

# 主函数
main() {
    echo "================================================"
    info "Watch Docker 卸载向导"
    echo "================================================"
    echo ""
    
    detect_platform
    
    # 停止并删除服务
    if [ "$OS" = "linux" ]; then
        remove_systemd_service
    elif [ "$OS" = "darwin" ]; then
        remove_launchd_service
    fi
    
    # 删除二进制文件
    remove_binary
    
    # 询问是否删除配置
    ask_remove_config
    
    echo ""
    echo "================================================"
    success "Watch Docker 卸载完成！"
    echo "================================================"
    echo ""
}

# 运行主函数
main

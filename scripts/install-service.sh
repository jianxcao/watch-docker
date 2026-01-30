#!/bin/bash
# Watch Docker systemd 服务安装脚本（用于 DEB/RPM 包）

set -e

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

main() {
    info "Watch Docker 服务安装脚本"
    echo ""
    info "服务文件已安装到: /lib/systemd/system/watch-docker.service"
    echo ""
    info "启用服务（作为特定用户运行）："
    echo "  sudo systemctl enable watch-docker@\$USER"
    echo ""
    info "启动服务："
    echo "  sudo systemctl start watch-docker@\$USER"
    echo ""
    info "查看状态："
    echo "  sudo systemctl status watch-docker@\$USER"
    echo ""
    warning "注意：服务将以指定用户身份运行，配置保存在该用户的主目录下"
    echo ""
}

main

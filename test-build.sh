#!/bin/bash
# Watch Docker 构建测试脚本

set -e

# 颜色输出
RED='\033[0;31m'
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

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

echo "========================================"
info "Watch Docker 构建测试"
echo "========================================"
echo ""

# 检查依赖
info "检查依赖..."
if ! command -v pnpm &> /dev/null; then
    error "pnpm 未安装"
    exit 1
fi

if ! command -v go &> /dev/null; then
    error "Go 未安装"
    exit 1
fi

success "依赖检查通过"
echo ""

# 构建前端
info "构建前端..."
cd frontend
pnpm install --frozen-lockfile
pnpm build
cd ..
success "前端构建完成"
echo ""

# 复制前端资源
info "复制前端资源到后端..."
rm -rf backend/internal/api/static
mkdir -p backend/internal/api/static
cp -r frontend/dist/* backend/internal/api/static/

# 验证文件
file_count=$(find backend/internal/api/static -type f | wc -l | tr -d ' ')
info "静态文件数量: $file_count"

if [ $file_count -lt 5 ]; then
    error "静态文件数量过少，可能复制失败"
    exit 1
fi

# 列出主要文件
info "主要文件："
ls -lh backend/internal/api/static/ | head -10

success "前端资源复制完成"
echo ""

# 构建后端
info "构建后端..."
cd backend
go build -o watch-docker cmd/watch-docker/main.go
cd ..

# 检查二进制
if [ -f "backend/watch-docker" ]; then
    success "后端构建完成"
    
    size=$(stat -f%z backend/watch-docker 2>/dev/null || stat -c%s backend/watch-docker)
    size_mb=$((size / 1024 / 1024))
    info "二进制大小: ${size_mb}MB"
    
    if [ $size -lt 10000000 ]; then
        warning "二进制文件较小 (<10MB)，可能未正确嵌入前端资源"
    else
        success "二进制大小正常，前端资源可能已正确嵌入"
    fi
else
    error "后端构建失败"
    exit 1
fi

echo ""
echo "========================================"
success "构建测试完成！"
echo "========================================"
echo ""
info "二进制文件位置: backend/watch-docker"
info "测试运行: cd backend && ./watch-docker"
info "访问地址: http://localhost:8080"
echo ""

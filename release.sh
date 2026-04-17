#!/bin/bash
#
# Watch Docker 一键发布脚本
#
# 支持三种发布模式：
#   1. 二进制发布 (GoReleaser) — 构建前端 + 多平台二进制 + 上传 GitHub Release
#   2. Docker 发布             — 构建多架构 Docker 镜像 + 推送 Docker Hub
#   3. 全量发布                — 以上两者都做
#
# 使用方式:
#   ./release.sh -v v0.1.12-beta                         # 全量发布（二进制 + Docker）
#   ./release.sh -v v0.1.12-beta --binary-only           # 仅 GoReleaser 二进制发布
#   ./release.sh -v v0.1.12-beta --docker-only           # 仅 Docker 镜像发布
#   ./release.sh -v v0.1.12 --dry-run                    # 只构建不推送，不创建 Release
#   ./release.sh -v v0.1.12 --docker-only -p linux/amd64 # 单架构 Docker 快速构建

set -euo pipefail

# ── 颜色 ──
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
error()   { echo -e "${RED}[ERROR]${NC} $1"; }

# ── 默认值 ──
VERSION=""
DOCKER_USER="${DOCKER_USER:-jianxcao}"
IMAGE_NAME="watch-docker"
PLATFORMS="linux/amd64,linux/arm64"
DRY_RUN=false
BINARY_ONLY=false
DOCKER_ONLY=false
SKIP_BINARY=false
SKIP_DOCKER=false
PRERELEASE=false
SKIP_FRONTEND=false

# ── 帮助 ──
show_help() {
    cat <<EOF
Watch Docker 一键发布脚本

用法: $0 [选项]

选项:
    -v, --version VERSION    版本号 (例: v0.1.12, v0.1.12-beta)，必填
    -u, --user USER          Docker Hub 用户名 (默认: $DOCKER_USER)
    -p, --platform PLATFORMS Docker 目标架构 (默认: linux/amd64,linux/arm64)
    --binary-only            仅构建二进制并发布到 GitHub Release (GoReleaser)
    --docker-only            仅构建并推送 Docker 镜像
    --dry-run                只构建不推送/发布
    --prerelease             GitHub Release 标记为预发布版本
    --skip-frontend          跳过前端构建（使用已有的 static 文件）
    -h, --help               显示帮助

示例:
    $0 -v v0.1.12-beta                          # 全量发布（二进制 + Docker）
    $0 -v v0.1.12-beta --binary-only             # 仅 GoReleaser 二进制
    $0 -v v0.1.12-beta --docker-only             # 仅 Docker 镜像
    $0 -v v0.1.12 --dry-run                      # 本地构建测试
    $0 -v v0.1.12 --docker-only -p linux/arm64   # 单架构 Docker

环境变量:
    DOCKER_USER              Docker Hub 用户名
    GITHUB_TOKEN             GitHub Token (不提供时自动从 gh auth token 获取)
EOF
}

# ── 解析参数 ──
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--version)       VERSION="$2"; shift 2 ;;
        -u|--user)          DOCKER_USER="$2"; shift 2 ;;
        -p|--platform)      PLATFORMS="$2"; shift 2 ;;
        --binary-only)      BINARY_ONLY=true; SKIP_DOCKER=true; shift ;;
        --docker-only)      DOCKER_ONLY=true; SKIP_BINARY=true; shift ;;
        --dry-run)          DRY_RUN=true; shift ;;
        --prerelease)       PRERELEASE=true; shift ;;
        --skip-frontend)    SKIP_FRONTEND=true; shift ;;
        -h|--help)          show_help; exit 0 ;;
        *) error "未知参数: $1"; show_help; exit 1 ;;
    esac
done

# ── 交互式输入版本号 ──
if [[ -z "$VERSION" ]]; then
    echo -e "${CYAN}请输入版本号 (例: v0.1.12, v0.1.12-beta):${NC}"
    read -r VERSION
    if [[ -z "$VERSION" ]]; then
        error "版本号不能为空"
        exit 1
    fi
fi

# 确保版本号以 v 开头
if [[ ! "$VERSION" =~ ^v ]]; then
    VERSION="v${VERSION}"
fi

# 检测是否为预发布版本
if [[ "$VERSION" =~ -(alpha|beta|rc|dev) ]]; then
    PRERELEASE=true
fi

VERSION_NUM="${VERSION#v}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
COMMIT_FULL=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
FULL_IMAGE="${DOCKER_USER}/${IMAGE_NAME}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── 打印配置 ──
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Watch Docker 发布脚本${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  版本:          ${YELLOW}${VERSION}${NC}"
echo -e "  提交:          ${YELLOW}${COMMIT}${NC}"
echo -e "  预发布:        ${YELLOW}${PRERELEASE}${NC}"
echo -e "  Dry Run:       ${YELLOW}${DRY_RUN}${NC}"
if ! $SKIP_BINARY; then
echo -e "  二进制发布:    ${YELLOW}是 (GoReleaser)${NC}"
fi
if ! $SKIP_DOCKER; then
echo -e "  Docker 发布:   ${YELLOW}是 (${FULL_IMAGE})${NC}"
echo -e "  Docker 架构:   ${YELLOW}${PLATFORMS}${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── 依赖检查 ──
check_dep() {
    if ! command -v "$1" &>/dev/null; then
        error "$1 未安装，请先安装"
        return 1
    fi
}

info "检查依赖..."
DEPS_OK=true
if ! $SKIP_BINARY; then
    check_dep goreleaser || DEPS_OK=false
    check_dep go || DEPS_OK=false
    check_dep gh || DEPS_OK=false
    check_dep pnpm || DEPS_OK=false
fi
if ! $SKIP_DOCKER; then
    check_dep docker || DEPS_OK=false
fi
check_dep git || DEPS_OK=false
$DEPS_OK || exit 1
success "依赖检查通过"
echo ""

# ── 记录开始时间 ──
START_TIME=$(date +%s)

# ═══════════════════════════════════════════
# 前端构建（二进制发布需要嵌入前端静态文件）
# ═══════════════════════════════════════════
if ! $SKIP_BINARY && ! $SKIP_FRONTEND; then
    info "构建前端..."
    cd "${SCRIPT_DIR}/frontend"
    pnpm install --frozen-lockfile
    pnpm build
    cd "${SCRIPT_DIR}"
    success "前端构建完成"

    info "复制前端资源到后端 static 目录..."
    rm -rf backend/internal/api/static
    mkdir -p backend/internal/api/static
    cp -r frontend/dist/* backend/internal/api/static/
    FILE_COUNT=$(find backend/internal/api/static -type f | wc -l | tr -d ' ')
    success "已复制 ${FILE_COUNT} 个静态文件"

    # 检查 git 是否有脏文件（GoReleaser 要求干净工作区）
    if ! git diff --quiet -- backend/internal/api/static/; then
        info "提交更新后的静态文件..."
        git add backend/internal/api/static/
        git commit -m "chore(frontend): update static files for ${VERSION} release"
    fi
    echo ""
fi

# ═══════════════════════════════════════════
# 创建/更新 Git Tag
# ═══════════════════════════════════════════
if ! $DRY_RUN; then
    if git rev-parse "${VERSION}" &>/dev/null; then
        # tag 已存在，检查是否指向当前 commit
        TAG_COMMIT=$(git rev-parse "${VERSION}")
        HEAD_COMMIT=$(git rev-parse HEAD)
        if [[ "$TAG_COMMIT" != "$HEAD_COMMIT" ]]; then
            info "更新 tag ${VERSION} 到当前 commit..."
            git tag -f "${VERSION}"
            git push origin "${VERSION}" --force
            success "Tag ${VERSION} 已更新"
        else
            info "Tag ${VERSION} 已指向当前 commit"
        fi
    else
        info "创建 Git tag: ${VERSION}"
        git tag -a "${VERSION}" -m "Release ${VERSION}"
        git push origin "${VERSION}"
        success "Tag ${VERSION} 已创建并推送"
    fi
    echo ""
fi

# ═══════════════════════════════════════════
# GoReleaser 二进制构建 & GitHub Release
# ═══════════════════════════════════════════
RELEASE_URL=""
if ! $SKIP_BINARY; then
    info "开始 GoReleaser 构建..."

    # 获取 GitHub Token
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        GITHUB_TOKEN=$(gh auth token 2>/dev/null || true)
        if [[ -z "$GITHUB_TOKEN" ]]; then
            error "无法获取 GitHub Token，请先运行 gh auth login 或设置 GITHUB_TOKEN 环境变量"
            exit 1
        fi
    fi
    export GITHUB_TOKEN

    GORELEASER_FLAGS="--clean"
    if $DRY_RUN; then
        GORELEASER_FLAGS="${GORELEASER_FLAGS} --skip=publish"
        info "[DRY RUN] 仅构建，不发布到 GitHub"
    fi

    cd "${SCRIPT_DIR}"
    GORELEASER_CURRENT_TAG="${VERSION}" goreleaser release ${GORELEASER_FLAGS}

    if ! $DRY_RUN; then
        RELEASE_URL=$(gh release view "${VERSION}" --json url -q '.url' 2>/dev/null || echo "")
        success "二进制已发布到 GitHub Release"
        if [[ -n "$RELEASE_URL" ]]; then
            info "Release URL: ${RELEASE_URL}"
        fi
    else
        success "[DRY RUN] 二进制构建完成 (未发布)"
        info "构建产物在 dist/ 目录"
    fi
    echo ""
fi

# ═══════════════════════════════════════════
# Docker 构建 & 推送
# ═══════════════════════════════════════════
if ! $SKIP_DOCKER; then
    info "开始构建 Docker 镜像..."

    cd "${SCRIPT_DIR}"

    # 确保 buildx builder 存在
    if ! docker buildx inspect watch-docker-builder &>/dev/null; then
        info "创建 buildx builder..."
        docker buildx create --name watch-docker-builder --use
    else
        docker buildx use watch-docker-builder
    fi

    # 构建 tags
    TAGS="-t ${FULL_IMAGE}:${VERSION}"
    if ! $PRERELEASE; then
        TAGS="${TAGS} -t ${FULL_IMAGE}:latest"
    fi

    BUILD_ARGS="--build-arg VERSION=${VERSION_NUM} --build-arg COMMIT=${COMMIT_FULL} --build-arg BUILD_TIME=${BUILD_TIME}"

    if $DRY_RUN; then
        info "[DRY RUN] 仅本地构建，不推送"
        CURRENT_ARCH=$(uname -m)
        case "$CURRENT_ARCH" in
            x86_64)  BUILD_PLATFORM="linux/amd64" ;;
            aarch64|arm64) BUILD_PLATFORM="linux/arm64" ;;
            *) BUILD_PLATFORM="linux/amd64" ;;
        esac
        docker buildx build \
            --platform "${BUILD_PLATFORM}" \
            ${BUILD_ARGS} \
            ${TAGS} \
            --load \
            -f Dockerfile .
        success "[DRY RUN] Docker 镜像构建成功 (本地)"
    else
        info "验证 Docker Hub 凭据..."
        if ! docker buildx imagetools inspect "${FULL_IMAGE}:latest" &>/dev/null 2>&1; then
            warn "Docker Hub 凭据可能未配置，请先在终端运行: docker login -u ${DOCKER_USER}"
            warn "如果使用 Keychain/凭据存储，忽略此警告即可"
        fi

        info "构建并推送多架构镜像 (${PLATFORMS})..."
        docker buildx build \
            --platform "${PLATFORMS}" \
            ${BUILD_ARGS} \
            ${TAGS} \
            --push \
            -f Dockerfile .
        success "Docker 镜像已推送: ${FULL_IMAGE}:${VERSION}"
        if ! $PRERELEASE; then
            success "Docker 镜像已推送: ${FULL_IMAGE}:latest"
        fi
    fi
    echo ""
fi

# ── 汇总 ──
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))
MINUTES=$((DURATION / 60))
SECS=$((DURATION % 60))

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  发布完成!${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  版本:    ${YELLOW}${VERSION}${NC}"
echo -e "  耗时:    ${YELLOW}${MINUTES}分 ${SECS}秒${NC}"
if ! $SKIP_BINARY; then
    if $DRY_RUN; then
        echo -e "  二进制:  ${YELLOW}本地构建完成 (dist/)${NC}"
    else
        echo -e "  二进制:  ${GREEN}${RELEASE_URL:-已发布到 GitHub Release}${NC}"
    fi
fi
if ! $SKIP_DOCKER; then
    if $DRY_RUN; then
        echo -e "  Docker:  ${YELLOW}本地构建完成 (未推送)${NC}"
    else
        echo -e "  Docker:  ${GREEN}${FULL_IMAGE}:${VERSION}${NC}"
    fi
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

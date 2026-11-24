#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==================== 可按需修改的测试配置 ====================
# 直接修改下方默认值，或通过环境变量临时覆盖，例如：
#   CONFIG_PATH=/my/config APP_PATH=/my/compose ./test-run.sh
PORT="${PORT:-18080}"
CONFIG_PATH="${CONFIG_PATH:-/tmp/wd-local/config}"
APP_PATH="${APP_PATH:-/tmp/wd-local/compose}"
USER_NAME="${USER_NAME:-admin}"
USER_PASSWORD="${USER_PASSWORD:-admin}"
IS_OPEN_DOCKER_SHELL="${IS_OPEN_DOCKER_SHELL:-true}"
IS_SECONDARY_VERIFICATION="${IS_SECONDARY_VERIFICATION:-false}"
SKIP_FRONTEND_BUILD="${SKIP_FRONTEND_BUILD:-false}"
# ==============================================================

print_usage() {
  cat <<'EOF'
用法: ./test-run.sh [选项]

本地构建前端静态资源并启动 Go 后端服务，用于快速测试当前分支代码。

选项:
  -s, --skip-build   跳过前端 pnpm build（若 frontend/dist/index.html 不存在仍会自动构建）
  -h, --help         显示帮助信息

常用变量（可在脚本顶部直接修改，或通过环境变量传入）:
  PORT                        监听端口（默认: 18080）
  CONFIG_PATH                 配置与数据目录（默认: /tmp/wd-local/config）
  APP_PATH                    Compose 项目目录（默认: /tmp/wd-local/compose）
  USER_NAME                   登录用户名（默认: admin）
  USER_PASSWORD               登录密码（默认: admin）
  IS_OPEN_DOCKER_SHELL        是否开启终端 Shell（默认: true）
  IS_SECONDARY_VERIFICATION   是否开启二次验证（默认: false）
  SKIP_FRONTEND_BUILD         是否跳过前端构建（默认: false）
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -s|--skip-build)
      SKIP_FRONTEND_BUILD="true"
      shift
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      echo "错误: 未知参数 '$1'" >&2
      print_usage >&2
      exit 1
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "错误: 未检测到 go 命令，请先安装 Go。" >&2
  exit 1
fi

if [[ "$SKIP_FRONTEND_BUILD" != "true" || ! -f "$ROOT_DIR/frontend/dist/index.html" ]]; then
  if ! command -v pnpm >/dev/null 2>&1; then
    echo "错误: 未检测到 pnpm 命令，请先安装 pnpm。" >&2
    exit 1
  fi

  if [[ ! -d "$ROOT_DIR/frontend/node_modules" ]]; then
    echo "[1/2] 未检测到 frontend/node_modules，正在安装前端依赖..."
    (cd "$ROOT_DIR/frontend" && pnpm install --frozen-lockfile)
  fi

  echo "[1/2] 正在构建前端产物到 frontend/dist..."
  (cd "$ROOT_DIR/frontend" && pnpm build)
else
  echo "[1/2] 已跳过前端构建，复用现有产物: $ROOT_DIR/frontend/dist"
fi

mkdir -p "$CONFIG_PATH" "$APP_PATH"
STATIC_DIR="${STATIC_DIR:-$ROOT_DIR/frontend/dist}"

echo ""
echo "[2/2] 正在启动 Watch Docker 本地测试服务..."
echo "  访问地址:    http://localhost:${PORT}"
echo "  登录账号:    ${USER_NAME}"
echo "  CONFIG_PATH: ${CONFIG_PATH}"
echo "  APP_PATH:    ${APP_PATH}"
echo "  STATIC_DIR:  ${STATIC_DIR}"
echo "  按 Ctrl+C 停止服务"
echo ""

cd "$ROOT_DIR/backend"
exec env \
  PORT="$PORT" \
  CONFIG_PATH="$CONFIG_PATH" \
  APP_PATH="$APP_PATH" \
  STATIC_DIR="$STATIC_DIR" \
  IS_OPEN_DOCKER_SHELL="$IS_OPEN_DOCKER_SHELL" \
  IS_SECONDARY_VERIFICATION="$IS_SECONDARY_VERIFICATION" \
  USER_NAME="$USER_NAME" \
  USER_PASSWORD="$USER_PASSWORD" \
  go run ./cmd/watch-docker

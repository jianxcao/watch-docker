#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==================== 可按需修改的测试配置 ====================
# 直接修改下方默认值，或通过环境变量临时覆盖，例如：
#   USER_PASSWORD=my-secret CONFIG_PATH=/my/config APP_PATH=/my/compose ./test-run.sh
PORT="${PORT:-18080}"
if [[ -z "${WATCH_SERVER_ADDR:-}" ]]; then
  case "$PORT" in
    *:*) WATCH_SERVER_ADDR="$PORT" ;;
    *)   WATCH_SERVER_ADDR="127.0.0.1:$PORT" ;;
  esac
fi
CONFIG_PATH="${CONFIG_PATH:-/tmp/wd-local/config}"
APP_PATH="${APP_PATH:-/tmp/wd-local/compose}"
USER_NAME="${USER_NAME-admin}"
IS_OPEN_DOCKER_SHELL="${IS_OPEN_DOCKER_SHELL:-false}"
IS_SECONDARY_VERIFICATION="${IS_SECONDARY_VERIFICATION:-false}"
SKIP_FRONTEND_BUILD="${SKIP_FRONTEND_BUILD:-false}"
# ==============================================================

print_usage() {
  cat <<EOF
用法: ./test-run.sh [选项]

本地构建前端静态资源并启动 Go 后端服务（默认仅监听回环地址且关闭容器 Shell）。

选项:
  -s, --skip-build   跳过前端 pnpm build（若 frontend/dist/index.html 不存在仍会自动构建）
  -h, --help         显示帮助信息

常用变量（可在脚本顶部直接修改，或通过环境变量传入）:
  PORT                        监听端口或主机:端口（默认: 18080 -> 127.0.0.1:18080）
  WATCH_SERVER_ADDR           显式监听地址（当前解析值: ${WATCH_SERVER_ADDR}）
  CONFIG_PATH                 配置与数据目录（默认: /tmp/wd-local/config）
  APP_PATH                    Compose 项目目录（默认: /tmp/wd-local/compose）
  USER_NAME                   登录用户名（默认: admin）
  USER_PASSWORD               登录密码（无默认密码，须通过环境变量或 CONFIG_PATH/app.yaml 设置非默认密码）
  IS_OPEN_DOCKER_SHELL        是否开启终端 Shell（默认: false）
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
CONFIG_PATH="$(cd "$CONFIG_PATH" && pwd)"
APP_PATH="$(cd "$APP_PATH" && pwd)"
STATIC_DIR="${STATIC_DIR:-$ROOT_DIR/frontend/dist}"
if [[ "$STATIC_DIR" != /* ]]; then
  if [[ -d "$STATIC_DIR" ]]; then
    STATIC_DIR="$(cd "$STATIC_DIR" && pwd)"
  else
    STATIC_DIR="$(pwd)/${STATIC_DIR#./}"
  fi
fi

echo ""
echo "[2/2] 正在启动 Watch Docker 本地测试服务..."
echo "  监听地址:    ${WATCH_SERVER_ADDR}"
echo "  登录账号:    ${USER_NAME}"
echo "  登录密码:    请通过 USER_PASSWORD 环境变量或 ${CONFIG_PATH}/app.yaml 配置非默认密码"
echo "  终端 Shell:  ${IS_OPEN_DOCKER_SHELL}"
echo "  CONFIG_PATH: ${CONFIG_PATH}"
echo "  APP_PATH:    ${APP_PATH}"
echo "  STATIC_DIR:  ${STATIC_DIR}"
echo "  按 Ctrl+C 停止服务"
echo ""

export WATCH_SERVER_ADDR PORT CONFIG_PATH APP_PATH STATIC_DIR USER_NAME IS_OPEN_DOCKER_SHELL IS_SECONDARY_VERIFICATION
if [[ -n "${USER_PASSWORD+x}" ]]; then
  export USER_PASSWORD
fi

cd "$ROOT_DIR/backend"
exec go run ./cmd/watch-docker --local-test

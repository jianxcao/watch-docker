#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_BASE="$(mktemp -d)"
trap 'rm -rf "$TMP_BASE"' EXIT

setup_sandbox() {
  local name="$1"
  local box="$TMP_BASE/$name"
  mkdir -p "$box/frontend/dist" "$box/backend" "$box/bin" "$box/cfg" "$box/apps"
  cp "$ROOT_DIR/test-run.sh" "$box/test-run.sh"
  chmod +x "$box/test-run.sh"
  : > "$box/frontend/dist/index.html"

  cat <<'EOF' > "$box/bin/go"
#!/usr/bin/env bash
set -euo pipefail
pass_set="false"
if [[ -n "${USER_PASSWORD:-}" ]]; then
  pass_set="true"
fi
printf 'ARGS=%s\n' "$*" > "$FAKE_GO_RECORD"
printf 'ADDR=%s\n' "${WATCH_SERVER_ADDR:-}" >> "$FAKE_GO_RECORD"
printf 'SHELL=%s\n' "${IS_OPEN_DOCKER_SHELL:-}" >> "$FAKE_GO_RECORD"
printf 'PASS_SET=%s\n' "$pass_set" >> "$FAKE_GO_RECORD"
printf 'USER_NAME=%s\n' "${USER_NAME:-}" >> "$FAKE_GO_RECORD"
EOF
  chmod +x "$box/bin/go"
  printf '%s' "$box"
}

assert_eq() {
  local got="$1" want="$2" msg="$3"
  if [[ "$got" != "$want" ]]; then
    echo "ASSERT FAILED: $msg: got='$got' want='$want'" >&2
    exit 1
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "ASSERT FAILED: $msg (missing '$needle')" >&2
    exit 1
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "ASSERT FAILED: $msg (unexpectedly contained secret)" >&2
    exit 1
  fi
}

# 1. 默认配置: 127.0.0.1:18080、IS_OPEN_DOCKER_SHELL=false、未设置密码且传递 --local-test
box1="$(setup_sandbox case1)"
out1="$(env -i PATH="$box1/bin:/usr/bin:/bin" HOME="$HOME" \
  CONFIG_PATH="$box1/cfg" APP_PATH="$box1/apps" FAKE_GO_RECORD="$box1/record.txt" \
  bash "$box1/test-run.sh" --skip-build 2>&1)"
rec1="$(cat "$box1/record.txt")"
assert_contains "$rec1" "ARGS=run ./cmd/watch-docker --local-test" "default args"
assert_contains "$rec1" "ADDR=127.0.0.1:18080" "default addr"
assert_contains "$rec1" "SHELL=false" "default shell disabled"
assert_contains "$rec1" "PASS_SET=false" "no default password"
assert_contains "$out1" "127.0.0.1:18080" "startup banner shows bind addr"

# 2. 单独设置 PORT=28080 -> 127.0.0.1:28080；PORT=0.0.0.0:28081 -> 0.0.0.0:28081
box2="$(setup_sandbox case2)"
env -i PATH="$box2/bin:/usr/bin:/bin" HOME="$HOME" \
  PORT="28080" CONFIG_PATH="$box2/cfg" APP_PATH="$box2/apps" FAKE_GO_RECORD="$box2/record.txt" \
  bash "$box2/test-run.sh" --skip-build >/dev/null
rec2="$(cat "$box2/record.txt")"
assert_contains "$rec2" "ADDR=127.0.0.1:28080" "PORT-only binds loopback"

box2b="$(setup_sandbox case2b)"
env -i PATH="$box2b/bin:/usr/bin:/bin" HOME="$HOME" \
  PORT="0.0.0.0:28081" CONFIG_PATH="$box2b/cfg" APP_PATH="$box2b/apps" FAKE_GO_RECORD="$box2b/record.txt" \
  bash "$box2b/test-run.sh" --skip-build >/dev/null
rec2b="$(cat "$box2b/record.txt")"
assert_contains "$rec2b" "ADDR=0.0.0.0:28081" "host:port PORT stays intact"

# 3. 显式 WATCH_SERVER_ADDR 优先于 PORT
box3="$(setup_sandbox case3)"
env -i PATH="$box3/bin:/usr/bin:/bin" HOME="$HOME" \
  PORT="28080" WATCH_SERVER_ADDR="127.0.0.1:29090" \
  CONFIG_PATH="$box3/cfg" APP_PATH="$box3/apps" FAKE_GO_RECORD="$box3/record.txt" \
  bash "$box3/test-run.sh" --skip-build >/dev/null
rec3="$(cat "$box3/record.txt")"
assert_contains "$rec3" "ADDR=127.0.0.1:29090" "explicit WATCH_SERVER_ADDR wins"

# 4. 显式开启 Shell 与设置密码（确保终端输出与记录都不泄漏密码）
box4="$(setup_sandbox case4)"
secret="LocalTestSecret_NoLeak_987!"
out4="$(env -i PATH="$box4/bin:/usr/bin:/bin" HOME="$HOME" \
  IS_OPEN_DOCKER_SHELL="true" USER_PASSWORD="$secret" \
  CONFIG_PATH="$box4/cfg" APP_PATH="$box4/apps" FAKE_GO_RECORD="$box4/record.txt" \
  bash "$box4/test-run.sh" --skip-build 2>&1)"
rec4="$(cat "$box4/record.txt")"
assert_contains "$rec4" "SHELL=true" "explicit shell enabled"
assert_contains "$rec4" "PASS_SET=true" "password passed via env"
assert_not_contains "$out4" "$secret" "stdout/stderr must not leak password"
assert_not_contains "$rec4" "$secret" "record must not contain password"

# 5. --help 输出监听地址、Shell 默认与密码配置指引
box5="$(setup_sandbox case5)"
help_out="$(env -i PATH="$box5/bin:/usr/bin:/bin" HOME="$HOME" WATCH_SERVER_ADDR="127.0.0.1:19999" \
  bash "$box5/test-run.sh" --help 2>&1)"
assert_contains "$help_out" "127.0.0.1:19999" "help displays effective addr"
assert_contains "$help_out" "IS_OPEN_DOCKER_SHELL" "help documents shell flag"
assert_contains "$help_out" "USER_PASSWORD" "help documents password env"
assert_contains "$help_out" "app.yaml" "help documents app.yaml"

echo "PASS: scripts/test-run_test.sh all checks succeeded"

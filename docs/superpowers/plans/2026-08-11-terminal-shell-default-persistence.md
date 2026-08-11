# 终端默认 Shell 与浏览器记忆实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 让 `/terminal` 自动使用当前运行用户的系统默认 Shell，在当前浏览器中按终端目标记住上次选择，并将 `local` 的展示改为“当前运行环境”。

**架构：** Go 后端从目标根目录的 `/etc/passwd` 和进程环境变量中解析安全白名单内的默认 Shell。TypeScript 纯工具函数负责浏览器存储键、有效性校验和回退，Vue 页面只负责加载选项、保存用户操作并展示中性文案。

**技术栈：** Go、Vue 3 `<script setup lang="ts">`、TypeScript、Vitest、Naive UI。

## 全局约束

- Shell 选择优先级：浏览器有效记录、后端系统默认值、第一个可用 Shell。
- `local` 和 `host` 使用不同的浏览器存储键。
- 浏览器存储异常不得阻止终端启动。
- 读取目标 `/etc/passwd` 失败时只记录 debug 日志，不得阻止 Shell 回退或终端启动。
- 后端只允许已探测到且可执行的已知 Shell。
- `local` 显示“当前运行环境”，`host` 显示“Docker 宿主机”。
- 不修改现有终端鉴权和 WebSocket 安全边界。

---

### 任务 1：识别目标用户的系统默认 Shell

**文件：**
- 修改：`backend/internal/api/shell_command.go`
- 修改：`backend/internal/api/shell_socket.go`
- 测试：`backend/internal/api/shell_socket_test.go`

**接口：**
- 生成：`discoverShells(root string, uid int, envShell string) ShellOptions`
- 生成：`loginShellForUID(root string, uid int) string`
- 使用：`currentShellOptions()` 对 `local` 传入 `os.Geteuid()`，对 `host` 传入 UID 0。

- [x] **步骤 1：编写失败测试**

在临时根目录创建 `/etc/passwd` 和可执行 Shell，验证登录 Shell 优先于 `SHELL`；同时覆盖登录 Shell 不可用、用户不存在和环境变量不可用时的回退。

```go
func TestDiscoverShellsPrefersLoginShell(t *testing.T) {
    root := t.TempDir()
    writeShellFixture(t, root, "/bin/sh", 0o755)
    writeShellFixture(t, root, "/bin/zsh", 0o755)
    writePasswdFixture(t, root, "tester:x:1000:1000::/home/tester:/bin/zsh\n")
    got := discoverShells(root, 1000, "/bin/sh")
    if got.DefaultShell != "/bin/zsh" {
        t.Fatalf("expected login shell, got %q", got.DefaultShell)
    }
}
```

- [x] **步骤 2：运行测试并确认失败原因正确**

运行：`cd backend && go test ./internal/api -run '^TestDiscoverShells' -count=1`

预期：因为函数尚未接收 UID、也未读取 `/etc/passwd` 而失败。

- [x] **步骤 3：实现最小后端逻辑**

`loginShellForUID` 读取 `<root>/etc/passwd`，按 UID 精确匹配第 3 列并返回第 7 列。`discoverShells` 只接受已探测选项中的登录 Shell 或环境变量，最终回退到第一个选项。

```go
targetUID := os.Geteuid()
if target == terminalTargetHost {
    targetUID = 0
}
options := discoverShells(root, targetUID, os.Getenv("SHELL"))
```

- [x] **步骤 4：运行后端专项测试**

运行：`cd backend && go test ./internal/api -run '^Test(DiscoverShells|ResolveShellCommand|ValidateHostRoot|SynchronizedWebSocketWriter)' -count=1`

预期：全部通过。

---

### 任务 2：按终端目标记住浏览器选择

**文件：**
- 修改：`frontend/src/utils/terminal.ts`
- 测试：`frontend/src/utils/terminal.test.ts`

**接口：**
- 生成：`terminalShellStorageKey(target: 'local' | 'host'): string`
- 生成：`selectInitialShell(target, shells, defaultShell, getStorage): string`
- 生成：`rememberShell(target, shell, shells, getStorage): void`

- [x] **步骤 1：编写失败测试**

覆盖 `local/host` 独立恢复、失效记录回退、后端默认值回退、只保存有效选项和存储异常。

```ts
expect(selectInitialShell('local', shells, '/bin/sh', () => storage)).toBe('/bin/zsh')
expect(selectInitialShell('host', shells, '/bin/sh', () => storage)).toBe('/bin/bash')
expect(selectInitialShell('local', shells, '/bin/bash', throwingStorage)).toBe('/bin/bash')
```

- [x] **步骤 2：运行测试并确认失败原因正确**

运行：`cd frontend && pnpm test -- src/utils/terminal.test.ts`

预期：新增工具函数尚未导出，测试失败。

- [x] **步骤 3：实现纯工具函数**

存储键固定为 `watch-docker:terminal-shell:<target>`。读取、写入及取得 Storage 的过程全部放在 `try/catch` 中；返回值必须存在于 `ShellOption[]`。

- [x] **步骤 4：运行前端工具测试**

运行：`cd frontend && pnpm test -- src/utils/terminal.test.ts`

预期：全部通过。

---

### 任务 3：接入终端页面并更新文案

**文件：**
- 修改：`frontend/src/pages/TerminalView.vue`

**接口：**
- 使用：任务 2 的 `selectInitialShell` 和 `rememberShell`。
- 保留：现有 `shellApi.getOptions()`、单 WebSocket 连接入口和安全重连逻辑。

- [x] **步骤 1：接入初始选择和保存逻辑**

加载后端选项后先设置 `terminalTarget`，再调用 `selectInitialShell`。用户主动选择时调用 `rememberShell`，随后沿用现有 `reconnect()`，不得增加第二个 WebSocket 打开入口。

```ts
selectedShell.value = selectInitialShell(
  response.data.target,
  response.data.shells,
  response.data.defaultShell,
  () => window.localStorage,
)
```

- [x] **步骤 2：更新目标文案**

```ts
const terminalTargetLabel = computed(() => {
  if (terminalTarget.value === 'host') return 'Docker 宿主机'
  if (terminalTarget.value === 'local') return '当前运行环境'
  return ''
})
```

模板只展示 `terminalTargetLabel`，不再出现“本地容器”。

- [x] **步骤 3：运行完整前端验证**

运行：`cd frontend && pnpm test && pnpm check && pnpm build`

预期：测试、类型检查和生产构建全部通过。

- [x] **步骤 4：运行最终仓库验证**

运行后端专项测试和 `git diff --check`。完整 Go 测试中的既有全局 logger 空指针问题单独记录，不归入本功能范围。

---

### 任务 4：记录默认 Shell 配置读取失败

**文件：**
- 修改：`backend/internal/api/shell_command.go`
- 测试：`backend/internal/api/shell_socket_test.go`

**接口：**
- 保留：`loginShellForUID(root string, uid int) string`，读取失败仍返回空字符串。
- 使用：项目全局 debug logger；logger 尚未初始化时不得调用其方法。

- [x] **步骤 1：编写失败测试**

用 `zaptest/observer` 安装 debug logger，传入不存在 `/etc/passwd` 的临时根目录，验证函数返回空字符串并记录包含文件路径和错误的 debug 日志。

- [x] **步骤 2：运行测试并确认失败原因正确**

运行：`cd backend && go test ./internal/api -run '^TestLoginShellForUIDLogsReadFailureAndFallsBack$' -count=1`

预期：函数仍能安全返回，但因为尚未记录日志而断言失败。

- [x] **步骤 3：实现最小日志逻辑**

读取失败时，仅在全局 logger 非空时调用 `Debug`，记录 `path` 和 `error` 字段，然后继续返回空字符串。

- [x] **步骤 4：验证失败路径和后端专项测试**

运行：`cd backend && go test ./internal/api -run '^Test(LoginShellForUID|DiscoverShells|ResolveShellCommand|ValidateHostRoot|SynchronizedWebSocketWriter)' -count=1`

预期：全部通过，且 logger 未初始化时也不会发生 panic。

# Host Terminal, Image Download, and Compose Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a selectable, explicitly enabled Linux host terminal, reliable native image downloads, and case-safe Compose project merging.

**Architecture:** Keep shell discovery and command construction as testable backend helpers used by the existing WebSocket handler. Reuse the browser's native anchor download mechanism for streamed image tar files. Merge Compose discovery and runtime rows through a pure identity-merging function keyed primarily by canonical configuration paths.

**Tech Stack:** Go 1.25+, Gin, gorilla/websocket, creack/pty, Vue 3.5, TypeScript, Pinia, Vitest, Docker Compose, ego-browser.

## Global Constraints

- Host terminal mode is Linux-only, disabled by default, and never silently falls back to a local container shell.
- Existing authentication and `IS_OPEN_DOCKER_SHELL` checks remain mandatory.
- User-provided shell values never become arbitrary commands or arguments.
- Compose display names retain original directory casing.
- Docker verification must not stop or delete any pre-existing container.

---

### Task 1: Compose project identity

**Files:**
- Modify: `backend/internal/composecli/client.go`
- Create: `backend/internal/composecli/client_test.go`

**Interfaces:**
- Consumes: scanned `[]ComposeProject` and Docker `[]ListOutput`.
- Produces: `mergeComposeProjects(scanned []ComposeProject, runtime []ListOutput) []ComposeProject`.

- [ ] Write a test where `/apps/Watch-Docker/docker-compose.yml` and the runtime row `watch-docker` with the same config path produce one project named `Watch-Docker` with runtime counts.
- [ ] Write tests for comma-separated config paths, unique case-insensitive fallback, ambiguous fallback, and unmatched runtime projects.
- [ ] Run `go test ./internal/composecli -run 'TestMergeComposeProjects' -count=1` and verify the missing helper fails.
- [ ] Extract runtime parsing and implement path-first, ambiguity-safe merging.
- [ ] Re-run the focused tests and verify they pass.

### Task 2: Shell discovery and host command construction

**Files:**
- Modify: `backend/internal/conf/envConfig.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/shell_socket.go`
- Create: `backend/internal/api/shell_socket_test.go`

**Interfaces:**
- Consumes: `TERMINAL_TARGET`, selected shell query, target filesystem root.
- Produces: `GET /api/v1/shell/options` and a validated command specification for the WebSocket PTY.

- [ ] Write tests for local and host shell discovery using temporary roots and executable fixtures.
- [ ] Write tests proving invalid shells and unsupported target modes are rejected.
- [ ] Write tests proving host mode constructs `nsenter` with PID 1 namespaces, target root, target working directory, and the selected shell.
- [ ] Run `go test ./internal/api -run 'Test(DiscoverShells|ResolveShellCommand)' -count=1` and verify failures occur for missing behavior.
- [ ] Add `TERMINAL_TARGET` with `local` default and optional `terminal_target` app YAML value.
- [ ] Add the authenticated shell-options route and implement shell validation/command resolution.
- [ ] Use the resolved command in the existing PTY WebSocket handler and return actionable startup errors.
- [ ] Re-run focused API tests and verify they pass.

### Task 3: Terminal shell selector

**Files:**
- Modify: `frontend/src/common/types.ts`
- Modify: `frontend/src/common/api.ts`
- Modify: `frontend/src/pages/TerminalView.vue`

**Interfaces:**
- Consumes: `{ shells: Array<{label: string; value: string}>, defaultShell: string, target: string }`.
- Produces: a header selector and WebSocket URL containing the encoded selected shell.

- [ ] Add typed API contracts for the shell-options response.
- [ ] Load options before the initial terminal connection.
- [ ] Add a selector to the terminal header and reconnect on selection changes.
- [ ] Preserve one source of truth for the selected shell and derive the WebSocket URL with `computed`.
- [ ] Run `pnpm check` and fix only errors caused by this task.

### Task 4: Native image download

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/pnpm-lock.yaml`
- Create: `frontend/src/utils/download.ts`
- Create: `frontend/src/utils/download.test.ts`
- Modify: `frontend/src/store/image.ts`
- Modify: `frontend/src/hooks/useImage.ts`

**Interfaces:**
- Produces: `buildAuthenticatedDownloadUrl(path: string, token: string, now?: number): string` and `triggerBrowserDownload(url: string): void`.

- [ ] Add Vitest and a `test` script.
- [ ] Write tests proving image IDs and tokens are encoded and a hidden anchor is clicked while `window.open` is never required.
- [ ] Run the focused Vitest file and verify it fails because the helpers do not exist.
- [ ] Implement the helpers and update the image store to use them synchronously.
- [ ] Change success feedback to “开始下载”.
- [ ] Re-run the focused tests and verify they pass.

### Task 5: Runtime packaging and documentation

**Files:**
- Modify: `Dockerfile`
- Modify: `docker-compose.yaml`
- Modify: `README.md`
- Modify: `app.yaml.example`
- Modify: `doc/user-guide/configuration.md`

**Interfaces:**
- Produces: an image containing `nsenter` and an explicit host-terminal deployment example.

- [ ] Install `util-linux` in the runtime image.
- [ ] Document `TERMINAL_TARGET=host`, `pid: host`, `privileged: true`, and `PUID=0` with a root-equivalent security warning.
- [ ] Correct documentation that currently implies a containerized local terminal is automatically the Docker host.
- [ ] Verify `docker compose config` parses the example.

### Task 6: Verification and browser QA

**Files:**
- No production files expected.

**Interfaces:**
- Consumes: all completed tasks.
- Produces: fresh command and UI evidence.

- [ ] Run focused Go tests for Compose and shell behavior.
- [ ] Run frontend Vitest, `pnpm check`, and `pnpm build`.
- [ ] Run `go test ./...`, recording the known baseline logger failure separately if it remains.
- [ ] Record `docker ps` IDs/names before testing.
- [ ] Build and start only uniquely named `watch-docker-codex-test-*` resources without changing existing containers.
- [ ] Use ego-browser to verify `/terminal` shows host shell options and changing selection reconnects.
- [ ] Use ego-browser/CDP download events to verify clicking an image Download control initiates a browser download without opening a popup.
- [ ] Remove only the uniquely named test resources and compare pre/post existing container IDs.
- [ ] Review `git diff --check` and the final diff.


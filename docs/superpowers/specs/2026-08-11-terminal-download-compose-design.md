# Host Terminal, Image Download, and Compose Identity Design

## Goal

Fix image downloads that do not start, make `/terminal` an explicitly enabled Linux host terminal with selectable installed shells, and merge Compose projects without losing the original directory name casing.

## Host terminal

`IS_OPEN_DOCKER_SHELL` remains the master safety switch. A new `TERMINAL_TARGET` application setting accepts `local` (the backwards-compatible default) or `host`. Host mode is intentionally opt-in and requires the Watch Docker container to run with the host PID namespace and elevated privileges.

The backend exposes an authenticated `GET /api/v1/shell/options` endpoint. It discovers only known shell paths that are executable in the selected target root and returns the available values plus the default. The WebSocket endpoint accepts a `shell` query parameter and rejects values that were not discovered. It never passes arbitrary user input to a command interpreter.

For `local`, the backend starts the selected executable directly. For `host`, it runs `nsenter` against host PID 1, enters the mount, UTS, IPC, network, and PID namespaces, adopts the target root and working directory, and then starts the selected host shell. Startup errors are returned through the WebSocket so missing host PID visibility, `nsenter`, privileges, or shells are actionable.

The `/terminal` page loads shell options before opening the WebSocket. Its header contains a typed shell selector. Changing the shell closes the existing connection and reconnects using the newly selected executable. If options cannot be loaded, the page shows the backend error rather than silently opening a local shell.

The official image installs `util-linux` for `nsenter`. The Compose example documents that host mode requires `pid: host`, `privileged: true`, `PUID=0`, `IS_OPEN_DOCKER_SHELL=true`, and `TERMINAL_TARGET=host`. Docker Desktop enters the Linux VM host, not the macOS or Windows host.

## Image download

The image store no longer calls `window.open()` after an asynchronous import. It builds an authenticated, URL-encoded download URL synchronously and triggers a hidden anchor click, following the existing container-file download pattern. This avoids popup blocking, preserves backend streaming, and does not buffer an image tar in browser memory.

The UI reports that the download has started. It does not claim completion because a native browser download provides no completion callback to the application.

## Compose project identity

Scanned projects retain the base directory spelling as their display name. Docker Compose runtime rows are merged into scanned rows primarily by canonical Compose file path, including any path in a comma-separated `ConfigFiles` value. A successful merge updates runtime status and counts but keeps the scanned name and Compose file.

When paths cannot match, a case-insensitive name fallback is allowed only when it identifies exactly one scanned project. Unmatched Docker projects remain visible under the name Docker reports. This prevents `Watch-Docker` and `watch-docker` from appearing twice without merging distinct ambiguous directories.

## Error handling and security

- Host mode remains disabled unless both existing authentication requirements and the explicit target setting are satisfied.
- Shell values are selected from discovered executable paths; arbitrary commands and arguments are rejected.
- Host-mode prerequisite failures are visible in the terminal instead of falling back to the application container.
- Image export errors continue to be returned by the backend stream; popup blocking is eliminated on the frontend.
- Compose matching never collapses an ambiguous case-insensitive name match.

## Testing

- Go unit tests cover shell discovery, default selection, local and host command construction, invalid shell rejection, and Compose path/name merging.
- Frontend unit tests cover encoded image download URLs and anchor-triggered downloads without `window.open`.
- Frontend type checking and production builds cover the Vue integration.
- Browser verification uses ego-browser against a disposable Watch Docker test container and confirms shell selection and image download initiation.
- Docker verification records existing containers before and after and removes only resources created with the dedicated `watch-docker-codex-test-*` prefix.


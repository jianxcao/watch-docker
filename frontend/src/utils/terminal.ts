import type { ShellOption } from '@/common/types'

type PageLocation = Pick<Location, 'protocol' | 'host'>
type TerminalTarget = 'local' | 'host'
type TerminalShellStorage = Pick<Storage, 'getItem' | 'setItem'>
type StorageGetter = () => TerminalShellStorage

const terminalShellStoragePrefix = 'watch-docker:terminal-shell'

export function buildTerminalSocketUrl(
  location: PageLocation,
  shell: string,
  token: string,
): string {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${location.host}/api/v1/shell?shell=${encodeURIComponent(shell)}&token=${encodeURIComponent(token)}`
}

export function terminalShellStorageKey(target: TerminalTarget): string {
  return `${terminalShellStoragePrefix}:${target}`
}

export function selectInitialShell(
  target: TerminalTarget,
  shells: ShellOption[],
  defaultShell: string,
  getStorage: StorageGetter,
): string {
  let rememberedShell = ''
  try {
    rememberedShell = getStorage().getItem(terminalShellStorageKey(target)) ?? ''
  } catch {
    // 浏览器禁用存储时继续使用后端默认值。
  }

  if (isAvailableShell(shells, rememberedShell)) {
    return rememberedShell
  }
  if (isAvailableShell(shells, defaultShell)) {
    return defaultShell
  }
  return shells[0]?.value ?? ''
}

export function rememberShell(
  target: TerminalTarget,
  shell: string,
  shells: ShellOption[],
  getStorage: StorageGetter,
): void {
  if (!isAvailableShell(shells, shell)) {
    return
  }
  try {
    getStorage().setItem(terminalShellStorageKey(target), shell)
  } catch {
    // 浏览器禁用存储时不影响终端切换。
  }
}

function isAvailableShell(shells: ShellOption[], shell: string): boolean {
  return shells.some((option) => option.value === shell)
}

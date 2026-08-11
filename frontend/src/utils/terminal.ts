type PageLocation = Pick<Location, 'protocol' | 'host'>

export function buildTerminalSocketUrl(
  location: PageLocation,
  shell: string,
  token: string,
): string {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${location.host}/api/v1/shell?shell=${encodeURIComponent(shell)}&token=${encodeURIComponent(token)}`
}

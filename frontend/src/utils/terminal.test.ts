import { describe, expect, it } from 'vitest'
import {
  buildTerminalSocketUrl,
  rememberShell,
  selectInitialShell,
  terminalShellStorageKey,
} from './terminal'

class MemoryStorage {
  private values = new Map<string, string>()

  constructor(initial: Record<string, string> = {}) {
    for (const [key, value] of Object.entries(initial)) {
      this.values.set(key, value)
    }
  }

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }
}

const shells = [
  { label: 'sh', value: '/bin/sh' },
  { label: 'bash', value: '/bin/bash' },
  { label: 'zsh', value: '/bin/zsh' },
]

describe('buildTerminalSocketUrl', () => {
  it('uses wss and encodes the selected shell and token on HTTPS pages', () => {
    expect(
      buildTerminalSocketUrl(
        { protocol: 'https:', host: 'watch.example:8443' },
        '/bin/bash',
        'token with spaces&scope=all',
      ),
    ).toBe(
      'wss://watch.example:8443/api/v1/shell?shell=%2Fbin%2Fbash&token=token%20with%20spaces%26scope%3Dall',
    )
  })

  it('uses ws on HTTP pages', () => {
    expect(
      buildTerminalSocketUrl({ protocol: 'http:', host: 'localhost:8080' }, '/bin/sh', ''),
    ).toBe('ws://localhost:8080/api/v1/shell?shell=%2Fbin%2Fsh&token=')
  })
})

describe('terminal shell preference', () => {
  it('restores a valid shell independently for each target', () => {
    const storage = new MemoryStorage({
      'watch-docker:terminal-shell:local': '/bin/zsh',
      'watch-docker:terminal-shell:host': '/bin/bash',
    })

    expect(selectInitialShell('local', shells, '/bin/sh', () => storage)).toBe('/bin/zsh')
    expect(selectInitialShell('host', shells, '/bin/sh', () => storage)).toBe('/bin/bash')
  })

  it('ignores an unavailable remembered shell and uses the backend default', () => {
    const storage = new MemoryStorage({
      'watch-docker:terminal-shell:local': '/bin/fish',
    })

    expect(selectInitialShell('local', shells, '/bin/bash', () => storage)).toBe('/bin/bash')
  })

  it('falls back to the first available shell when storage and default are unavailable', () => {
    const throwingStorage = () => {
      throw new Error('storage disabled')
    }

    expect(selectInitialShell('local', shells, '/bin/fish', throwingStorage)).toBe('/bin/sh')
  })

  it('stores only an available shell and tolerates storage failures', () => {
    const storage = new MemoryStorage()

    rememberShell('local', '/bin/zsh', shells, () => storage)
    rememberShell('local', '/bin/fish', shells, () => storage)
    rememberShell('host', '/bin/bash', shells, () => {
      throw new Error('storage disabled')
    })

    expect(storage.getItem(terminalShellStorageKey('local'))).toBe('/bin/zsh')
    expect(storage.getItem(terminalShellStorageKey('host'))).toBeNull()
  })
})

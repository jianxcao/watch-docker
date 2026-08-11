import { describe, expect, it } from 'vitest'
import { buildTerminalSocketUrl } from './terminal'

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

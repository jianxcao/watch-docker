import { describe, expect, it } from 'vitest'
import { formatBytes, formatBytesPerSecond, formatTime, validateComposeYaml } from './utils'
import { doesContainerUseImage, normalizeImageRef, parseImageRef } from './imageRef'

describe('formatBytes & formatBytesPerSecond', () => {
  it('handles zero, fractional (<1), negative, and PB+ values safely', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(-5)).toBe('0 B')
    expect(formatBytes(Number.NaN)).toBe('0 B')
    expect(formatBytes(0.5)).toBe('0.50 B')
    expect(formatBytes(1024)).toBe('1.00 KB')
    expect(formatBytes(1024 ** 5 * 2)).toBe('2.00 PB')

    expect(formatBytesPerSecond(0)).toBe('0 B/s')
    expect(formatBytesPerSecond(0.25)).toBe('0.25 B/s')
    expect(formatBytesPerSecond(1024 ** 5)).toBe('1.00 PB/s')
  })
})

describe('formatTime', () => {
  it('returns "-" for empty or Go zero timestamp', () => {
    expect(formatTime('')).toBe('-')
    expect(formatTime('0001-01-01T00:00:00Z')).toBe('-')
  })
})

describe('validateComposeYaml', () => {
  it('accepts modern compose v2 YAML without version and comments with apostrophes', () => {
    const yaml = `# app's compose config (don't remove)
services:
  web:
    image: nginx:alpine # shouldn't fail on apostrophe
    ports:
      - "8080:80"
`
    expect(validateComposeYaml(yaml)).toEqual({ isValid: true, errorMessage: '' })
  })

  it('rejects unclosed quotes and missing services', () => {
    expect(validateComposeYaml('name: demo\n').isValid).toBe(false)
    expect(validateComposeYaml('services:\n  web:\n    image: "nginx\n').isValid).toBe(false)
  })

  it('ignores unmatched quotes inside YAML block scalars and treats only whitespace-preceded # as comments', () => {
    const yaml = `services:
  web:
    image: nginx:latest
    command: |
      echo "hello" && sh -c 'don''t fail unmatched '
    labels:
      com.example.url: https://example.com#section
`
    expect(validateComposeYaml(yaml)).toEqual({ isValid: true, errorMessage: '' })
  })
})

describe('imageRef (:port-aware matching)', () => {
  it('distinguishes registry port from tag', () => {
    expect(parseImageRef('localhost:5000/my-app')).toEqual({
      repository: 'localhost:5000/my-app',
      tag: '',
      digest: '',
    })
    expect(parseImageRef('localhost:5000/my-app:v1.2.3')).toEqual({
      repository: 'localhost:5000/my-app',
      tag: 'v1.2.3',
      digest: '',
    })
    expect(normalizeImageRef('nginx')).toBe('nginx:latest')
    expect(normalizeImageRef('docker.io/library/nginx:latest')).toBe('nginx:latest')
    expect(normalizeImageRef('registry-1.docker.io/library/nginx')).toBe('nginx:latest')
    expect(normalizeImageRef('nginx:1.25@sha256:abcdef')).toBe('nginx@sha256:abcdef')
    expect(normalizeImageRef('localhost:5000/my-app')).toBe('localhost:5000/my-app:latest')
  })

  it('matches containers to exact image tags or imageID without cross-tag false positives', () => {
    const img125 = {
      id: 'sha256:aaa',
      repoTags: ['nginx:1.25'],
      repoDigests: [],
    }
    const img127 = {
      id: 'sha256:bbb',
      repoTags: ['nginx:1.27'],
      repoDigests: [],
    }
    const container = {
      image: 'nginx:1.25',
      imageID: 'sha256:aaa',
    }
    expect(doesContainerUseImage(container, img125)).toBe(true)
    expect(doesContainerUseImage(container, img127)).toBe(false)

    // When both imageIDs exist and differ, never fall back to repoTags
    expect(
      doesContainerUseImage(
        { image: 'nginx:1.25', imageID: 'sha256:111111111111222222222222' },
        { id: 'sha256:999999999999888888888888', repoTags: ['nginx:1.25'], repoDigests: [] },
      ),
    ).toBe(false)

    // 12-char short ID prefix match
    expect(
      doesContainerUseImage(
        { image: 'nginx:1.25', imageID: 'abcdef123456' },
        {
          id: 'sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890',
          repoTags: [],
          repoDigests: [],
        },
      ),
    ).toBe(true)

    // Digest ref must only match repoDigests and never fall back to validTags
    expect(
      doesContainerUseImage(
        { image: 'nginx:latest@sha256:deadbeef' },
        { id: 'sha256:other', repoTags: ['nginx:latest'], repoDigests: ['nginx@sha256:cafebabe'] },
      ),
    ).toBe(false)

    const registryPortImg = {
      id: 'sha256:ccc',
      repoTags: ['registry.local:5000/app:latest'],
      repoDigests: [],
    }
    expect(
      doesContainerUseImage({ image: 'registry.local:5000/app' }, registryPortImg),
    ).toBe(true)
    expect(
      doesContainerUseImage({ image: 'registry.local:5000/app:v2' }, registryPortImg),
    ).toBe(false)
  })
})

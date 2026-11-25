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

  it('accepts unquoted apostrophes in plain scalars and |2- / colon-key block scalars', () => {
    const yaml = `services:
  web:
    image: nginx:latest
    command: echo it's fine
    labels:
      org.opencontainers.image.description: Bob's server
      "a:b": |2-
        unmatched 'inside
`
    expect(validateComposeYaml(yaml)).toEqual({ isValid: true, errorMessage: '' })
  })

  it('accepts flow mappings, quoted services key, empty services map, anchors/aliases, root merges, and compose tags', () => {
    expect(validateComposeYaml('{services: {web: {image: nginx}}}')).toEqual({
      isValid: true,
      errorMessage: '',
    })
    expect(validateComposeYaml('"services":\n  web:\n    image: nginx\n')).toEqual({
      isValid: true,
      errorMessage: '',
    })
    expect(validateComposeYaml('services: {}\n')).toEqual({
      isValid: true,
      errorMessage: '',
    })
    expect(
      validateComposeYaml('x-def: &defs\n  web:\n    image: nginx\nservices: *defs\n'),
    ).toEqual({
      isValid: true,
      errorMessage: '',
    })
    expect(
      validateComposeYaml('x-base: &base\n  services:\n    web:\n      image: nginx\n<<: *base\n'),
    ).toEqual({
      isValid: true,
      errorMessage: '',
    })
    expect(
      validateComposeYaml(
        'services:\n  web:\n    image: nginx\n    ports: !reset []\n    volumes: !override\n      - /a:/b\n',
      ),
    ).toEqual({
      isValid: true,
      errorMessage: '',
    })
  })

  it('rejects missing top-level services, non-mapping root/services, and syntax errors with line info', () => {
    expect(validateComposeYaml('   \n')).toEqual({
      isValid: false,
      errorMessage: '请输入 YAML 配置',
    })

    for (const nested of [
      'x-template: {services: {}}\n',
      '# services:\nname: demo\n',
      'x-doc:\n  text: |\n    services:\n      web:\n        image: nginx\n',
    ]) {
      const res = validateComposeYaml(nested)
      expect(res.isValid).toBe(false)
      expect(res.errorMessage).toContain('services')
    }

    for (const badStruct of ['- web\n', 'services: null\n', 'services: []\n', 'services: text\n']) {
      const res = validateComposeYaml(badStruct)
      expect(res.isValid).toBe(false)
      expect(res.errorMessage).toContain('映射')
    }

    for (const badSyntax of [
      'services:\n  web:\n    image: "nginx\n',
      '{services: {web: {image: nginx}}\n',
    ]) {
      const res = validateComposeYaml(badSyntax)
      expect(res.isValid).toBe(false)
      expect(res.errorMessage).toContain('语法错误')
      expect(res.errorMessage).toMatch(/第 \d+ 行/)
    }
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

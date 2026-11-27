import { describe, expect, it, vi } from 'vitest'
import { buildComposeCreateUpPayload } from './composeCreateUpPayload'
import {
  createComposeCreateProgressProtocol,
  type ComposeCreateProgressStatus,
} from './composeCreateProgressProtocol'

function encodeStatusFrame(status: 'success' | 'error', message: string): ArrayBuffer {
  const bytes = new TextEncoder().encode(`\x00JSON:${JSON.stringify({ status, message })}`)
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
}

function encodeTextBytes(text: string): ArrayBuffer {
  const bytes = new TextEncoder().encode(text)
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
}

describe('buildComposeCreateUpPayload', () => {
  it('includes composeFile alongside name, yamlContent and force when redeploying', () => {
    const payload = buildComposeCreateUpPayload({
      projectName: 'demo-stack',
      composeFile: '/data/compose/demo-stack/compose.yaml',
      yamlContent: 'services:\n  web:\n    image: nginx:alpine\n',
      force: true,
    })

    expect(payload).toEqual({
      name: 'demo-stack',
      composeFile: '/data/compose/demo-stack/compose.yaml',
      yamlContent: 'services:\n  web:\n    image: nginx:alpine\n',
      force: true,
    })
  })

  it('omits composeFile when creating a new project', () => {
    const payload = buildComposeCreateUpPayload({
      projectName: 'fresh-stack',
      yamlContent: 'services:\n  app:\n    image: alpine:3.20\n',
    })

    expect(payload).toEqual({
      name: 'fresh-stack',
      yamlContent: 'services:\n  app:\n    image: alpine:3.20\n',
      force: false,
    })
    expect('composeFile' in payload).toBe(false)
  })
})

describe('createComposeCreateProgressProtocol', () => {
  it('handles binary conflict error frame as ArrayBuffer and preserves exact reason across disconnect', async () => {
    let status: ComposeCreateProgressStatus = 'creating'
    const chunks: Array<string | Uint8Array> = []
    const lines: string[] = []
    const onSuccess = vi.fn()
    const onError = vi.fn()
    const onComplete = vi.fn()

    const protocol = createComposeCreateProgressProtocol({
      onAppendChunk: (chunk) => chunks.push(chunk),
      onAppendLine: (line) => lines.push(line),
      onStatusChange: (next) => {
        status = next
      },
      getStatus: () => status,
      onSuccess,
      onError,
      onComplete,
    })

    const conflictReason = '项目正在部署中，请等待当前部署完成后重试'
    protocol.handleMessage(encodeTextBytes(`\x1b[31m${conflictReason}\x1b[0m\r\n`))
    protocol.handleMessage(encodeStatusFrame('error', conflictReason))
    await protocol.handleDisconnected()

    expect(status).toBe('error')
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith(conflictReason)
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onComplete).not.toHaveBeenCalled()
    expect(chunks).toHaveLength(1)
    expect(new TextDecoder().decode(chunks[0] as Uint8Array)).toContain(conflictReason)
    expect(lines).toEqual([])
  })

  it('awaits queued Blob status frame before evaluating disconnect fallback', async () => {
    let status: ComposeCreateProgressStatus = 'creating'
    const lines: string[] = []
    const onError = vi.fn()

    const protocol = createComposeCreateProgressProtocol({
      onAppendChunk: () => {},
      onAppendLine: (line) => lines.push(line),
      onStatusChange: (next) => {
        status = next
      },
      getStatus: () => status,
      onSuccess: () => {},
      onError,
      onComplete: () => {},
    })

    const conflictReason = '项目正在使用其他配置部署中，请等待当前部署完成后重试'
    protocol.handleMessage(new Blob([encodeStatusFrame('error', conflictReason)]))
    await protocol.handleDisconnected()

    expect(status).toBe('error')
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith(conflictReason)
    expect(lines).toEqual([])
  })

  it('reports unexpected disconnect only when creating ends without a status frame', async () => {
    let status: ComposeCreateProgressStatus = 'creating'
    const lines: string[] = []
    const onError = vi.fn()

    const protocol = createComposeCreateProgressProtocol({
      onAppendChunk: () => {},
      onAppendLine: (line) => lines.push(line),
      onStatusChange: (next) => {
        status = next
      },
      getStatus: () => status,
      onSuccess: () => {},
      onError,
      onComplete: () => {},
    })

    await protocol.handleDisconnected()

    expect(status).toBe('error')
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith('连接异常断开')
    expect(lines).toEqual(['\x1b[31m✗ 连接异常断开\x1b[0m\r\n'])
  })
})

export type ComposeCreateProgressStatus = 'idle' | 'connecting' | 'creating' | 'success' | 'error'

export interface ComposeCreateProgressHandlers {
  onAppendChunk: (chunk: string | Uint8Array) => void
  onAppendLine: (line: string) => void
  onStatusChange: (status: ComposeCreateProgressStatus) => void
  getStatus: () => ComposeCreateProgressStatus
  onSuccess: (message: string) => void
  onError: (message: string) => void
  onComplete: () => void
}

const JSON_PREFIX = 'JSON:'

export function createComposeCreateProgressProtocol(handlers: ComposeCreateProgressHandlers) {
  let sessionToken = 0
  const pendingReads = new Set<Promise<void>>()

  const handleBinaryFrame = (data: Uint8Array, seq = sessionToken) => {
    if (seq !== sessionToken) {
      return
    }
    if (
      data.length > 6 &&
      data[0] === 0 &&
      String.fromCharCode(...data.slice(1, 6)) === JSON_PREFIX
    ) {
      try {
        const jsonStr = new TextDecoder().decode(data.slice(6))
        const statusMsg = JSON.parse(jsonStr) as { status?: string; message?: string }
        const msg = typeof statusMsg.message === 'string' ? statusMsg.message : ''
        if (statusMsg.status === 'success') {
          handlers.onStatusChange('success')
          handlers.onSuccess(msg)
          handlers.onComplete()
        } else if (statusMsg.status === 'error') {
          handlers.onStatusChange('error')
          handlers.onError(msg)
        }
      } catch (e) {
        console.error('Failed to parse status message:', e)
      }
      return
    }
    handlers.onAppendChunk(data)
  }

  const handleMessage = (data: ArrayBuffer | Blob | Uint8Array | string | null | undefined) => {
    if (!data) {
      return
    }
    const seq = sessionToken
    if (data instanceof Uint8Array) {
      handleBinaryFrame(data, seq)
      return
    }
    if (data instanceof ArrayBuffer) {
      handleBinaryFrame(new Uint8Array(data), seq)
      return
    }
    if (typeof Blob !== 'undefined' && data instanceof Blob) {
      const readPromise = data
        .arrayBuffer()
        .then((buffer) => {
          handleBinaryFrame(new Uint8Array(buffer), seq)
        })
        .catch((err) => {
          console.error('Failed to read binary frame blob:', err)
        })
        .finally(() => {
          pendingReads.delete(readPromise)
        })
      pendingReads.add(readPromise)
      return
    }
    if (typeof data === 'string') {
      handlers.onAppendChunk(data)
    }
  }

  const handleDisconnected = async () => {
    const seq = sessionToken
    while (pendingReads.size > 0 && seq === sessionToken) {
      await Promise.all(Array.from(pendingReads))
    }
    if (seq !== sessionToken) {
      return
    }
    if (handlers.getStatus() === 'creating') {
      handlers.onStatusChange('error')
      handlers.onAppendLine('\x1b[31m✗ 连接异常断开\x1b[0m\r\n')
      handlers.onError('连接异常断开')
    }
  }

  const reset = () => {
    sessionToken++
    pendingReads.clear()
  }

  return {
    handleBinaryFrame,
    handleMessage,
    handleDisconnected,
    reset,
  }
}

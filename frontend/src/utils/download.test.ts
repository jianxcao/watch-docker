import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  buildContainerExportUrl,
  buildImageDownloadUrl,
  triggerBrowserDownload,
} from './download'

describe('buildImageDownloadUrl', () => {
  it('encodes the complete image id and authentication token', () => {
    expect(buildImageDownloadUrl('sha256:abc/def', 'token with spaces&scope=all', 1234)).toBe(
      '/api/v1/images/sha256%3Aabc%2Fdef/download?token=token%20with%20spaces%26scope%3Dall&_t=1234',
    )
  })

  it('omits token query parameter when authentication token is empty', () => {
    expect(buildImageDownloadUrl('sha256:abc', '', 5678)).toBe(
      '/api/v1/images/sha256%3Aabc/download?_t=5678',
    )
  })
})

describe('buildContainerExportUrl', () => {
  it('encodes container id and optional token cleanly', () => {
    expect(buildContainerExportUrl('ctr/123', 'tok=1&a=2', 42)).toBe(
      '/api/v1/containers/ctr%2F123/export?token=tok%3D1%26a%3D2&_t=42',
    )
    expect(buildContainerExportUrl('ctr123', '', 42)).toBe(
      '/api/v1/containers/ctr123/export?_t=42',
    )
  })
})

describe('triggerBrowserDownload', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses a temporary anchor instead of a popup window', () => {
    const click = vi.fn()
    const appendChild = vi.fn()
    const removeChild = vi.fn()
    const link = { href: '', download: '', style: { display: '' }, click }
    const open = vi.fn()
    vi.stubGlobal('window', { open })
    vi.stubGlobal('document', {
      createElement: vi.fn(() => link),
      body: { appendChild, removeChild },
    })

    triggerBrowserDownload('/api/v1/images/id/download?token=abc')

    expect(link).toMatchObject({
      href: '/api/v1/images/id/download?token=abc',
      download: '',
      style: { display: 'none' },
    })
    expect(appendChild).toHaveBeenCalledWith(link)
    expect(click).toHaveBeenCalledOnce()
    expect(removeChild).toHaveBeenCalledWith(link)
    expect(open).not.toHaveBeenCalled()
  })
})

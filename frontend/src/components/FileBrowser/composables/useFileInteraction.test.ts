import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FileEntry } from '@/common/types'

const isMobileRef = ref(false)

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({
    isMobile: isMobileRef,
  }),
}))

import { useFileInteraction } from './useFileInteraction'

const makeEntry = (name: string, type: FileEntry['type']): FileEntry => ({
  name,
  type,
  size: 0,
  permissions: type === 'directory' ? 'drwxr-xr-x' : '-rw-r--r--',
  owner: 'root',
  group: 'root',
  modified: '2026-11-25T00:00:00Z',
  readonly: false,
})

describe('useFileInteraction', () => {
  beforeEach(() => {
    isMobileRef.value = false
  })

  it('desktop openEntry (context menu "打开") navigates directories and opens files', () => {
    const { handleEntryClick, handleDoubleClick, openEntry } = useFileInteraction()
    const onNavigate = vi.fn()
    const onOpenFile = vi.fn()
    const onSelect = vi.fn()

    const dir = makeEntry('etc', 'directory')
    const subDir = makeEntry('log', 'directory')
    const file = makeEntry('app.conf', 'file')
    const fifo = makeEntry('events', 'other')

    // Desktop single click only selects
    handleEntryClick(dir, '/', { onNavigate, onOpenFile, onSelect }, true)
    expect(onSelect).toHaveBeenCalledWith(dir)
    expect(onNavigate).not.toHaveBeenCalled()

    // Desktop context menu "打开" uses openEntry and navigates into directory
    openEntry(dir, '/', { onNavigate, onOpenFile, onSelect }, true)
    expect(onNavigate).toHaveBeenNthCalledWith(1, '/etc')

    openEntry(subDir, '/var', { onNavigate, onOpenFile, onSelect }, true)
    expect(onNavigate).toHaveBeenNthCalledWith(2, '/var/log')

    // Double click opens files and respects non-regular entries
    handleDoubleClick(file, '/etc', { onNavigate, onOpenFile }, false)
    expect(onOpenFile).toHaveBeenCalledWith(file, true)

    onOpenFile.mockClear()
    openEntry(fifo, '/tmp', { onNavigate, onOpenFile, onSelect }, true)
    expect(onOpenFile).not.toHaveBeenCalled()
    expect(onNavigate).toHaveBeenCalledTimes(2)
  })

  it('mobile handleEntryClick navigates directories, opens files, and skips other entries', () => {
    isMobileRef.value = true
    const { handleEntryClick, handleDoubleClick } = useFileInteraction()
    const onNavigate = vi.fn()
    const onOpenFile = vi.fn()
    const onSelect = vi.fn()

    const dir = makeEntry('tmp', 'directory')
    const file = makeEntry('notes.txt', 'file')
    const fifo = makeEntry('pipe', 'other')

    handleEntryClick(dir, '/', { onNavigate, onOpenFile, onSelect }, true)
    expect(onNavigate).toHaveBeenCalledWith('/tmp')

    handleEntryClick(file, '/tmp', { onNavigate, onOpenFile, onSelect }, true)
    expect(onOpenFile).toHaveBeenCalledWith(file, false)

    onOpenFile.mockClear()
    handleEntryClick(fifo, '/tmp', { onNavigate, onOpenFile, onSelect }, true)
    expect(onOpenFile).not.toHaveBeenCalled()

    handleDoubleClick(dir, '/', { onNavigate, onOpenFile }, true)
    expect(onNavigate).toHaveBeenCalledTimes(1)
  })
})

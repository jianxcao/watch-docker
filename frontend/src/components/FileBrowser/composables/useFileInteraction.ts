import type { FileEntry } from '@/common/types'
import { useResponsive } from '@/hooks/useResponsive'

/**
 * 文件交互（点击、双击）
 */
export function useFileInteraction() {
  const { isMobile } = useResponsive()

  // 处理文件条目点击（移动端单击打开；桌面端单击仅选中，由双击打开）
  const handleEntryClick = (
    entry: FileEntry,
    currentPath: string,
    callbacks: {
      onNavigate: (path: string) => void
      onOpenFile: (entry: FileEntry, readOnly: boolean) => void
      onSelect: (entry: FileEntry) => void
    },
    canEdit: boolean,
  ) => {
    callbacks.onSelect(entry)

    if (isMobile.value) {
      if (entry.type === 'directory') {
        const newPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`
        callbacks.onNavigate(newPath)
      } else {
        callbacks.onOpenFile(entry, !canEdit)
      }
    }
  }

  // 处理双击
  const handleDoubleClick = (
    entry: FileEntry,
    currentPath: string,
    callbacks: {
      onNavigate: (path: string) => void
      onOpenFile: (entry: FileEntry, readOnly: boolean) => void
    },
    canEdit: boolean,
  ) => {
    if (isMobile.value) {
      return // 移动端忽略双击
    }

    if (entry.type === 'file') {
      callbacks.onOpenFile(entry, !canEdit)
    } else if (entry.type === 'directory') {
      const newPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`
      callbacks.onNavigate(newPath)
    }
  }

  return {
    handleEntryClick,
    handleDoubleClick,
  }
}

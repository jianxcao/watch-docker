import type { FileEntry } from '@/common/types'

export type SortField = 'name' | 'size' | 'modified' | 'type'
export type SortDirection = 'asc' | 'desc'

export interface FileBrowserProps {
  containerId: string
  initialPath?: string
  canEdit?: boolean
}

export interface FileEditState {
  name: string
  path: string
  content: string
}

export interface ContextMenuState {
  show: boolean
  x: number
  y: number
  entry: FileEntry | null
}

export type FileOperation = 'view' | 'edit' | 'download' | 'delete' | 'rename' | 'chmod'

export type { FileEntry }

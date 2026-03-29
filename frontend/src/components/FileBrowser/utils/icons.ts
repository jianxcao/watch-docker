import { Document, Folder, Link as LinkIcon } from '@vicons/ionicons5'
import type { Component } from 'vue'
import type { FileEntry } from '../types'

/**
 * 获取文件图标组件
 */
export function getIconComponent(entry: FileEntry): Component {
  switch (entry.type) {
    case 'directory':
      return Folder
    case 'symlink':
      return LinkIcon
    case 'file':
      return Document
    default:
      return Document
  }
}

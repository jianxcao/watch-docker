import type { FileEntry } from '@/common/types'
import { ref } from 'vue'

/**
 * 文件编辑器状态管理
 */
export function useFileEditor() {
  const fileEditor = ref({
    show: false,
    filePath: '',
    fileName: '',
    readOnly: false,
  })

  // 打开文件编辑器
  const openFileEditor = (entry: FileEntry, currentPath: string, readOnly: boolean = false) => {
    const fullPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`

    fileEditor.value = {
      show: true,
      filePath: fullPath,
      fileName: entry.name,
      readOnly,
    }
  }

  // 关闭文件编辑器
  const closeFileEditor = () => {
    fileEditor.value.show = false
  }

  return {
    fileEditor,
    openFileEditor,
    closeFileEditor,
  }
}

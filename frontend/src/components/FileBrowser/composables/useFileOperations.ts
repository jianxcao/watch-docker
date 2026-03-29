import { deleteContainerPath } from '@/common/api'
import type { FileEntry } from '@/common/types'
import { useMessage } from 'naive-ui'
import { ref } from 'vue'
import { downloadContainerFile } from '../utils'

/**
 * 文件操作（下载、删除等）
 */
export function useFileOperations(containerId: string) {
  const message = useMessage()
  const deleting = ref<string | null>(null)

  // 下载文件
  const downloadFile = async (entry: FileEntry, currentPath: string) => {
    const filePath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`

    try {
      await downloadContainerFile(containerId, filePath, entry.name)
      message.success(`${entry.name} 开始下载`)
    } catch (err: any) {
      message.error(err.message || '下载失败')
    }
  }

  // 删除文件/目录
  const deleteEntry = async (entry: FileEntry, currentPath: string, onSuccess?: () => void) => {
    const fullPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`
    deleting.value = entry.name

    try {
      const res = await deleteContainerPath(containerId, fullPath)
      if (res.code === 0) {
        message.success(`已删除 ${entry.name}`)
        onSuccess?.()
      } else {
        throw new Error(res.msg || '删除失败')
      }
    } catch (err: any) {
      message.error(err.message || '删除失败')
    } finally {
      deleting.value = null
    }
  }

  return {
    deleting,
    downloadFile,
    deleteEntry,
  }
}

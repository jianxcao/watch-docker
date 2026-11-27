import { deleteContainerPath } from '@/common/api'
import type { FileEntry } from '@/common/types'
import { useDialog, useMessage } from 'naive-ui'
import { ref, toValue, type MaybeRefOrGetter } from 'vue'
import { downloadContainerFile } from '../utils'

/**
 * 文件操作（下载、删除等）
 */
export function useFileOperations(containerId: MaybeRefOrGetter<string>) {
  const message = useMessage()
  const dialog = useDialog()
  const deleting = ref<string | null>(null)

  // 下载文件
  const downloadFile = async (entry: FileEntry, currentPath: string) => {
    const filePath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`

    try {
      await downloadContainerFile(toValue(containerId), filePath, entry.name)
      message.success(`${entry.name} 开始下载`)
    } catch (err: any) {
      message.error(err.message || '下载失败')
    }
  }

  // 删除文件/目录（二次确认，防止误删）
  const deleteEntry = (entry: FileEntry, currentPath: string, onSuccess?: () => void) => {
    const fullPath = currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`
    if (fullPath === '/') {
      message.warning('禁止删除根目录')
      return
    }
    const targetType = entry.type === 'directory' ? '目录' : '文件'
    dialog.warning({
      title: `确认删除${targetType}`,
      content: `确定要永久删除${targetType} "${fullPath}" 吗？此操作不可撤销。`,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        deleting.value = entry.name
        try {
          const res = await deleteContainerPath(toValue(containerId), fullPath)
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
      },
    })
  }

  return {
    deleting,
    downloadFile,
    deleteEntry,
  }
}

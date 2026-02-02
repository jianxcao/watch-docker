import { uploadContainerFiles } from '@/common/api'
import { useMessage } from 'naive-ui'
import { ref } from 'vue'

/**
 * 文件上传功能
 */
export function useFileUpload(containerId: string) {
  const message = useMessage()
  const uploading = ref(false)
  const uploadProgress = ref(0)

  // 上传文件
  const uploadFiles = async (files: File[], targetPath: string, onSuccess?: () => void) => {
    if (!files || files.length === 0) {
      return
    }

    uploading.value = true
    uploadProgress.value = 0

    try {
      const res = await uploadContainerFiles(containerId, targetPath, files)
      
      if (res.code === 0) {
        const count = res.data.uploaded.length
        message.success(`成功上传 ${count} 个文件`)
        onSuccess?.()
      } else {
        throw new Error(res.msg || '上传失败')
      }
    } catch (err: any) {
      message.error(err.message || '上传失败')
    } finally {
      uploading.value = false
      uploadProgress.value = 0
    }
  }

  // 处理文件选择
  const handleFileSelect = (event: Event, targetPath: string, onSuccess?: () => void) => {
    const input = event.target as HTMLInputElement
    if (input.files && input.files.length > 0) {
      const files = Array.from(input.files)
      uploadFiles(files, targetPath, onSuccess)
      // 清空 input，允许重复上传相同文件
      input.value = ''
    }
  }

  // 处理拖拽上传
  const handleDrop = async (event: DragEvent, targetPath: string, onSuccess?: () => void) => {
    event.preventDefault()
    event.stopPropagation()

    const files = event.dataTransfer?.files
    if (files && files.length > 0) {
      const fileArray = Array.from(files)
      await uploadFiles(fileArray, targetPath, onSuccess)
    }
  }

  return {
    uploading,
    uploadProgress,
    uploadFiles,
    handleFileSelect,
    handleDrop,
  }
}

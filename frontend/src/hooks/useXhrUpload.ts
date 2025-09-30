import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { formatBytes } from '@/common/utils'

export interface UploadOptions {
  url: string
  file: File
  formData?: FormData
  getToken?: () => string
  onSuccess?: (response: any) => void
  onError?: (error: any) => void
}

export function useXhrUpload() {
  const message = useMessage()

  // 上传状态
  const uploading = ref(false)
  const uploadProgress = ref(0)
  const uploadedSize = ref(0)
  const totalSize = ref(0)
  const currentFileName = ref('')
  const uploadSpeed = ref('')

  // 重置状态
  const resetState = () => {
    uploading.value = false
    uploadProgress.value = 0
    uploadedSize.value = 0
    totalSize.value = 0
    currentFileName.value = ''
    uploadSpeed.value = ''
  }

  // 执行上传
  const upload = async (options: UploadOptions): Promise<void> => {
    const { url, file, formData = new FormData(), getToken, onSuccess, onError } = options

    if (!formData.has('file')) {
      formData.append('file', file)
    }

    uploading.value = true
    currentFileName.value = file.name
    totalSize.value = file.size

    const startTime = Date.now()
    let lastLoaded = 0
    let lastTime = startTime

    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()

      // 监听上传进度
      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
          const progress = Math.round((event.loaded / event.total) * 100)
          uploadProgress.value = progress
          uploadedSize.value = event.loaded

          // 计算上传速度
          const currentTime = Date.now()
          const timeDiff = currentTime - lastTime
          if (timeDiff > 1000) {
            // 每秒更新一次速度
            const sizeDiff = event.loaded - lastLoaded
            const speedBps = sizeDiff / (timeDiff / 1000)
            uploadSpeed.value = `${formatBytes(speedBps)}/s`
            lastLoaded = event.loaded
            lastTime = currentTime
          }
        }
      })

      // 处理响应
      xhr.addEventListener('load', () => {
        uploading.value = false
        if (xhr.status === 200) {
          try {
            const response = JSON.parse(xhr.responseText)
            if (response.code === 0) {
              onSuccess?.(response)
              resolve()
            } else {
              const errorMsg = response.msg || '上传失败'
              message.error(errorMsg)
              onError?.(response)
              reject(new Error(errorMsg))
            }
          } catch (err) {
            console.error('响应解析失败:', err)
            const errorMsg = '响应解析失败'
            message.error(errorMsg)
            onError?.(err)
            reject(new Error(errorMsg))
          }
        } else {
          const errorMsg = `上传失败 (${xhr.status})`
          message.error(errorMsg)
          onError?.(new Error(errorMsg))
          reject(new Error(errorMsg))
        }
      })

      xhr.addEventListener('error', () => {
        const errorMsg = '网络错误'
        message.error(errorMsg)
        uploading.value = false
        onError?.(new Error(errorMsg))
        reject(new Error(errorMsg))
      })

      // 获取认证token并发送请求
      try {
        const token = getToken ? getToken() : ''

        xhr.open('POST', url)
        if (token) {
          xhr.setRequestHeader('Authorization', `Bearer ${token}`)
        }
        xhr.send(formData)
      } catch (err) {
        console.error('获取token失败:', err)
        const errorMsg = '获取认证信息失败'
        message.error(errorMsg)
        uploading.value = false
        onError?.(err)
        reject(new Error(errorMsg))
      }
    })
  }

  return {
    // 状态
    uploading,
    uploadProgress,
    uploadedSize,
    totalSize,
    currentFileName,
    uploadSpeed,
    // 方法
    resetState,
    upload,
    formatFileSize: formatBytes,
  }
}

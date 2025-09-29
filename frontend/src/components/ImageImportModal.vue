<template>
  <n-modal v-model:show="showModal" preset="dialog" title="导入镜像" style="width: 600px">
    <template #header>
      <div class="modal-header">
        <n-icon size="20" class="header-icon">
          <CloudUploadOutline />
        </n-icon>
        <span>导入镜像</span>
      </div>
    </template>

    <div class="import-content">
      <n-upload ref="uploadRef" :max="1" :file-list="fileList" :custom-request="handleCustomRequest" accept=".tar"
        :show-file-list="false" directory-dnd @update:file-list="handleFileListUpdate"
        @before-upload="handleBeforeUpload">
        <n-upload-dragger>
          <div class="upload-area">
            <n-icon size="48" :depth="3">
              <CloudUploadOutline />
            </n-icon>
            <n-text class="upload-title">
              点击或者拖动文件到该区域来上传
            </n-text>
            <n-p depth="3" class="upload-hint">
              仅支持 .tar 格式的 Docker 镜像文件
            </n-p>
          </div>
        </n-upload-dragger>
      </n-upload>

      <!-- 上传进度 -->
      <div v-if="uploading" class="upload-progress">
        <n-space vertical>
          <div class="progress-info">
            <n-text>{{ currentFileName }}</n-text>
            <n-text depth="3">{{ formatFileSize(uploadedSize) }} / {{ formatFileSize(totalSize) }}</n-text>
          </div>
          <n-progress type="line" :percentage="uploadProgress" :show-indicator="false" :height="8" />
          <div class="progress-details">
            <n-text depth="3">{{ uploadProgress.toFixed(1) }}% - {{ uploadSpeed }}</n-text>
          </div>
        </n-space>
      </div>

      <!-- 上传结果 -->
      <div v-if="uploadResult" class="upload-result">
        <n-result :status="uploadResult.success ? 'success' : 'error'" :title="uploadResult.success ? '导入成功' : '导入失败'">
          <template #footer>
            <n-text depth="3">{{ uploadResult.message }}</n-text>
          </template>
        </n-result>
      </div>
    </div>

    <template #action>
      <n-space justify="end">
        <n-button @click="handleCancel" :disabled="uploading">
          {{ uploading ? '上传中...' : '取消' }}
        </n-button>
        <n-button v-if="uploadResult?.success" type="primary" @click="handleConfirm">
          确定
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage, type UploadCustomRequestOptions, type UploadFileInfo } from 'naive-ui'
import { CloudUploadOutline } from '@vicons/ionicons5'
import { formatBytes } from '@/common/utils'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'update:show', show: boolean): void
  (e: 'success'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const message = useMessage()

// 响应式状态
const showModal = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value)
})

const uploadRef = ref()
const fileList = ref<UploadFileInfo[]>([])
const uploading = ref(false)
const uploadProgress = ref(0)
const uploadedSize = ref(0)
const totalSize = ref(0)
const currentFileName = ref('')
const uploadSpeed = ref('')
const uploadResult = ref<{ success: boolean; message: string } | null>(null)

// 重置状态
const resetState = () => {
  fileList.value = []
  uploading.value = false
  uploadProgress.value = 0
  uploadedSize.value = 0
  totalSize.value = 0
  currentFileName.value = ''
  uploadSpeed.value = ''
  uploadResult.value = null
}

// 监听弹窗显示状态
watch(() => props.show, (show) => {
  if (show) {
    resetState()
  }
})

// 文件列表更新
const handleFileListUpdate = (files: UploadFileInfo[]) => {
  fileList.value = files
}

// 上传前验证
const handleBeforeUpload = (data: { file: UploadFileInfo }) => {
  const { file } = data

  // 验证文件类型
  if (!file.name?.endsWith('.tar')) {
    message.error('请选择 .tar 格式的镜像文件')
    return false
  }

  // 验证文件大小（可选，比如限制最大5GB）
  const maxSize = 5 * 1024 * 1024 * 1024 // 5GB
  if (file.file && file.file.size > maxSize) {
    message.error('文件大小不能超过 5GB')
    return false
  }

  return true
}

// 自定义上传处理
const handleCustomRequest = async (options: UploadCustomRequestOptions) => {
  const { file, onProgress, onFinish, onError } = options

  if (!file.file) {
    onError()
    return
  }

  uploading.value = true
  uploadResult.value = null
  currentFileName.value = file.name || 'unknown'
  totalSize.value = file.file.size

  const startTime = Date.now()
  let lastLoaded = 0
  let lastTime = startTime

  try {
    // 创建 FormData
    const formData = new FormData()
    formData.append('file', file.file)

    // 创建 XMLHttpRequest 以便监听上传进度
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
        if (timeDiff > 1000) { // 每秒更新一次速度
          const sizeDiff = event.loaded - lastLoaded
          const speedBps = sizeDiff / (timeDiff / 1000)
          uploadSpeed.value = `${formatBytes(speedBps)}/s`
          lastLoaded = event.loaded
          lastTime = currentTime
        }

        onProgress({ percent: progress })
      }
    })

    // 处理响应
    xhr.addEventListener('load', () => {
      if (xhr.status === 200) {
        try {
          const response = JSON.parse(xhr.responseText)
          if (response.code === 0) {
            uploadResult.value = { success: true, message: response.data?.message || '镜像导入成功' }
            onFinish()
            emit('success')
          } else {
            uploadResult.value = { success: false, message: response.msg || '导入失败' }
            onError()
          }
        } catch (err) {
          uploadResult.value = { success: false, message: '响应解析失败' }
          onError()
        }
      } else {
        uploadResult.value = { success: false, message: `上传失败 (${xhr.status})` }
        onError()
      }
      uploading.value = false
    })

    xhr.addEventListener('error', () => {
      uploadResult.value = { success: false, message: '网络错误' }
      uploading.value = false
      onError()
    })

    // 获取认证token
    const { getToken } = await import('@/common/axiosConfig')
    const token = getToken()

    // 发送请求
    xhr.open('POST', '/api/v1/images/import')
    if (token) {
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    }
    xhr.send(formData)

  } catch (error: any) {
    console.error('上传错误:', error)
    uploadResult.value = { success: false, message: error.message || '上传失败' }
    uploading.value = false
    onError()
  }
}

// 格式化文件大小
const formatFileSize = (bytes: number): string => {
  return formatBytes(bytes)
}

// 取消操作
const handleCancel = () => {
  if (!uploading.value) {
    showModal.value = false
  }
}

// 确认操作
const handleConfirm = () => {
  showModal.value = false
}
</script>

<style scoped lang="less">
.modal-header {
  display: flex;
  align-items: center;
  gap: 8px;

  .header-icon {
    color: var(--primary-color);
  }
}

.import-content {
  .upload-area {
    text-align: center;
    padding: 40px 20px;

    .upload-title {
      display: block;
      font-size: 16px;
      margin: 16px 0 8px;
    }

    .upload-hint {
      margin: 0;
      font-size: 14px;
    }
  }

  .upload-progress {
    margin-top: 24px;
    padding: 20px;
    background: var(--card-color);
    border-radius: 8px;
    border: 1px solid var(--border-color);

    .progress-info {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 12px;
    }

    .progress-details {
      text-align: center;
      margin-top: 8px;
    }
  }

  .upload-result {
    margin-top: 24px;
  }
}
</style>

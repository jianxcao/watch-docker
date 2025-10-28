<template>
  <n-modal
    v-model:show="showModal"
    :icon="getIcon()"
    preset="dialog"
    style="padding: 12px; width: 90vw; max-width: 600px"
  >
    <template #header>
      <span>导入镜像</span>
    </template>
    <div class="import-content">
      <n-upload
        ref="uploadRef"
        :max="1"
        :file-list="fileList"
        :custom-request="handleCustomRequest"
        accept=".tar"
        :show-file-list="false"
        directory-dnd
        @update:file-list="handleFileListUpdate"
        @before-upload="handleBeforeUpload"
      >
        <n-upload-dragger>
          <div class="upload-area">
            <n-icon size="48" :depth="3">
              <CloudUploadOutline />
            </n-icon>
            <n-text class="upload-title"> 点击或者拖动文件到该区域来上传 </n-text>
            <n-p depth="3" class="upload-hint"> 仅支持 .tar 格式的 Docker 镜像文件 </n-p>
          </div>
        </n-upload-dragger>
      </n-upload>

      <!-- 上传进度 -->
      <div v-if="uploading" class="upload-progress">
        <n-space vertical>
          <div class="progress-info">
            <n-text>{{ currentFileName }}</n-text>
            <n-text depth="3"
              >{{ formatFileSize(uploadedSize) }} / {{ formatFileSize(totalSize) }}</n-text
            >
          </div>
          <n-progress
            type="line"
            :percentage="uploadProgress"
            :show-indicator="false"
            :height="8"
          />
          <div class="progress-details">
            <n-text depth="3">{{ uploadProgress.toFixed(1) }}% - {{ uploadSpeed }}</n-text>
          </div>
        </n-space>
      </div>
    </div>
    <template #action>
      <n-space justify="end">
        <n-button @click="handleCancel" :disabled="uploading">
          {{ uploading ? '上传中...' : '取消' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import {
  useMessage,
  useThemeVars,
  type UploadCustomRequestOptions,
  type UploadFileInfo,
} from 'naive-ui'
import { CloudUploadOutline } from '@vicons/ionicons5'
import { renderIcon } from '@/common/utils'
import { useXhrUpload } from '@/hooks/useXhrUpload'
import { useSettingStore } from '@/store/setting'

const theme = useThemeVars()
const settingStore = useSettingStore()

const getIcon = () => {
  return renderIcon(CloudUploadOutline, {
    color: theme.value.primaryColor,
    size: 20,
  })
}

interface Emits {
  (e: 'success'): void
}

const showModal = defineModel<boolean>('show')
const emit = defineEmits<Emits>()
const message = useMessage()
const uploadRef = ref()
const fileList = ref<UploadFileInfo[]>([])

// 使用上传hooks
const {
  uploading,
  uploadProgress,
  uploadedSize,
  totalSize,
  currentFileName,
  uploadSpeed,
  resetState,
  upload,
  formatFileSize,
} = useXhrUpload()

// 监听弹窗显示状态
watch(showModal, (show) => {
  if (show) {
    fileList.value = []
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
  const { file, onFinish, onError } = options

  if (!file.file) {
    onError()
    return
  }

  try {
    await upload({
      url: '/api/v1/images/import',
      file: file.file,
      getToken: () => settingStore.getToken(),
      onSuccess: () => {
        onFinish()
        emit('success')
        showModal.value = false
      },
      onError: () => {
        onError()
      },
    })
  } catch (error) {
    console.error('导入镜像失败:', error)
    onError()
  }
}

// 取消操作
const handleCancel = () => {
  if (!uploading.value) {
    showModal.value = false
  }
}
</script>

<style scoped lang="less">
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
}
</style>

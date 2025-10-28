<template>
  <n-modal
    v-model:show="show"
    :icon="getIcon()"
    preset="dialog"
    style="padding: 12px; width: 90vw; max-width: 600px"
  >
    <template #header>
      <span>导入容器</span>
    </template>

    <div class="import-content">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <n-grid :cols="24" :x-gap="12">
          <n-form-item-gi :span="12" label="镜像名称" path="repository">
            <n-input v-model:value="form.repository" placeholder="例如: myapp" />
          </n-form-item-gi>
          <n-form-item-gi :span="12" label="标签" path="tag">
            <n-input v-model:value="form.tag" placeholder="例如: latest" />
          </n-form-item-gi>
        </n-grid>
      </n-form>

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
            <n-p depth="3" class="upload-hint"> 仅支持 .tar 格式的 Docker 容器文件 </n-p>
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

      <!-- 文件信息 -->
      <div v-if="selectedFile" class="file-info">
        <n-card size="small">
          <div class="file-detail">
            <n-icon size="20" :color="theme.primaryColor">
              <DocumentTextOutline />
            </n-icon>
            <div class="file-meta">
              <n-text class="file-name">{{ selectedFile.name }}</n-text>
              <n-text depth="3" class="file-size">{{ formatBytes(selectedFile.size) }}</n-text>
            </div>
          </div>
        </n-card>
      </div>
    </div>

    <n-alert type="info" class="mt-2">
      注意：容器上传后会生成对应的镜像，请用镜像在继续容器的创建
    </n-alert>

    <template #action>
      <div class="modal-actions">
        <n-button @click="handleCancel" :disabled="uploading">
          {{ uploading ? '上传中...' : '取消' }}
        </n-button>
        <n-button
          type="primary"
          :loading="uploading"
          :disabled="!selectedFile || !form.repository || uploading"
          @click="handleImport"
        >
          {{ uploading ? '导入中...' : '开始导入' }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { formatBytes, renderIcon } from '@/common/utils'
import { useXhrUpload } from '@/hooks/useXhrUpload'
import { useSettingStore } from '@/store/setting'
import { CloudUploadOutline, DocumentTextOutline } from '@vicons/ionicons5'
import type { FormRules, UploadFileInfo } from 'naive-ui'
import { useMessage, useThemeVars } from 'naive-ui'
import { ref, watch } from 'vue'

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

const show = defineModel<boolean>('show')
const emit = defineEmits<Emits>()

// 响应式数据
const message = useMessage()
const formRef = ref()
const uploadRef = ref()
const fileList = ref<UploadFileInfo[]>([])
const selectedFile = ref<File | null>(null)

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

// 表单数据
const form = ref({
  repository: '',
  tag: 'latest',
})

// 表单验证规则
const rules: FormRules = {
  repository: {
    required: true,
    message: '请输入镜像名称',
    trigger: ['input', 'blur'],
  },
}

// 监听弹窗显示状态
watch(show, (showModal) => {
  if (showModal) {
    resetState()
  }
})

const handleBeforeUpload = (data: { file: UploadFileInfo }) => {
  const file = data.file.file
  if (!file) {
    return false
  }

  // 检查文件类型
  if (!file.name.endsWith('.tar')) {
    message.error('仅支持 .tar 格式的文件')
    return false
  }

  // 检查文件大小 (限制为 5GB)
  const maxSize = 5 * 1024 * 1024 * 1024 // 5GB
  if (file.size > maxSize) {
    message.error('文件大小不能超过 5GB')
    return false
  }

  selectedFile.value = file
  return false // 阻止自动上传
}

const handleFileListUpdate = (files: UploadFileInfo[]) => {
  fileList.value = files
  if (files.length === 0) {
    selectedFile.value = null
  }
}

const handleCustomRequest = () => {
  // 阻止默认上传行为
}

const handleImport = async () => {
  if (!selectedFile.value || !form.value.repository) {
    message.error('请选择文件并填写镜像名称')
    return
  }

  // 表单验证
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  try {
    // 创建 FormData
    const formData = new FormData()
    formData.append('file', selectedFile.value)
    formData.append('repository', form.value.repository)
    formData.append('tag', form.value.tag)

    // 使用公共上传hooks
    await upload({
      url: '/api/v1/containers/import',
      file: selectedFile.value,
      formData,
      getToken: () => settingStore.getToken(),
      onSuccess: () => {
        message.success('容器导入成功')
        emit('success')
        handleCancel()
      },
    })
  } catch (error: any) {
    console.error('导入失败:', error)
  }
}

const handleCancel = () => {
  if (!uploading.value) {
    show.value = false
    // 重置状态
    resetState()
    selectedFile.value = null
    fileList.value = []
    form.value = {
      repository: '',
      tag: 'latest',
    }

    // 清空 upload 组件
    uploadRef.value?.clear()
  }
}
</script>

<style scoped>
.import-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.upload-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 20px;
}

.upload-title {
  font-size: 16px;
  margin: 12px 0 8px;
}

.upload-hint {
  margin: 0;
  font-size: 14px;
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

.file-info {
  margin-top: 16px;
}

.file-detail {
  display: flex;
  align-items: center;
  gap: 12px;
}

.file-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.file-name {
  font-weight: 500;
}

.file-size {
  font-size: 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>

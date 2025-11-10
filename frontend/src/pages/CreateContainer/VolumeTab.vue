<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="volume-tab">
      <n-space vertical size="large">
        <div>
          <n-h3 prefix="bar" class="mt-0">数据卷挂载</n-h3>
          <n-space vertical size="small">
            <div v-for="(volume, index) in formValue.volumeList" :key="index" class="volume-item">
              <div class="volume-field">
                <span class="volume-label">源路径</span>
                <n-input
                  v-model:value="volume.source"
                  placeholder="源路径或卷名"
                  size="small"
                  @blur="updateVolumes"
                />
              </div>
              <div class="volume-separator">
                <span>:</span>
              </div>
              <div class="volume-field">
                <span class="volume-label">容器路径</span>
                <n-input
                  v-model:value="volume.target"
                  placeholder="容器路径"
                  size="small"
                  @blur="updateVolumes"
                />
              </div>
              <div class="volume-options">
                <n-checkbox
                  v-model:checked="volume.readonly"
                  size="small"
                  @update:checked="updateVolumes"
                >
                  只读
                </n-checkbox>
              </div>
              <div class="volume-delete">
                <n-button size="small" tertiary type="error" @click="removeVolume(index)">
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                  <span v-if="isMobile">删除</span>
                </n-button>
              </div>
            </div>
            <n-button dashed block @click="addVolume" size="small">
              <template #icon>
                <n-icon><AddOutline /></n-icon>
              </template>
              添加数据卷挂载
            </n-button>
          </n-space>
        </div>

        <n-divider />

        <div>
          <n-h3 prefix="bar">文本格式</n-h3>
          <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">
            每行一个挂载,格式: /host/path:/container/path 或 /host/path:/container/path:ro
          </n-text>
          <n-input
            v-model:value="formValue.volumeText"
            type="textarea"
            placeholder="/host/path:/container/path&#10;volume_name:/container/path:ro"
            :rows="6"
            @blur="handleVolumeTextChange"
          />
        </div>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import type { FormInst } from 'naive-ui'
import type { VolumeFormValue, VolumeItem } from './types'
import { useResponsive } from '@/hooks/useResponsive'

const { isMobile } = useResponsive()

const formValue = defineModel<VolumeFormValue>({
  default: () => ({
    binds: [],
    volumeList: [],
    volumeText: '',
  }),
})

const formRef = ref<FormInst | null>(null)

const addVolume = () => {
  formValue.value.volumeList.push({ source: '', target: '', readonly: false })
}

const removeVolume = (index: number) => {
  formValue.value.volumeList.splice(index, 1)
  updateVolumes()
}

const updateVolumes = () => {
  const newBinds = formValue.value.volumeList
    .filter((item) => item.source.trim() && item.target.trim())
    .map((item) => {
      let bind = `${item.source}:${item.target}`
      if (item.readonly) {
        bind += ':ro'
      }
      return bind
    })

  formValue.value.binds = newBinds
  formValue.value.volumeText = newBinds.join('\n')
}

const handleVolumeTextChange = () => {
  const lines = formValue.value.volumeText.split('\n').filter((line) => line.trim())
  const newBinds: string[] = []
  const newVolumeList: VolumeItem[] = []

  lines.forEach((line) => {
    const trimmedLine = line.trim()
    if (trimmedLine && trimmedLine.includes(':')) {
      newBinds.push(trimmedLine)
      const parts = trimmedLine.split(':')
      newVolumeList.push({
        source: parts[0] || '',
        target: parts[1] || '',
        readonly: parts[2] === 'ro',
      })
    }
  })

  formValue.value.volumeList = newVolumeList
  formValue.value.binds = newBinds
  formValue.value.volumeText = newBinds.join('\n')
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.volume-tab {
  padding: 0;
}

.volume-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.volume-field {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.volume-field :deep(.n-input) {
  width: 100%;
}

.volume-label {
  font-size: 14px;
  white-space: nowrap;
  display: none;
}

.volume-separator {
  display: flex;
  align-items: center;
  opacity: 0.6;
  font-size: 16px;
}

.volume-options {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
}

.volume-delete {
  flex: 0 0 44px;
  width: 44px;
}

/* 移动端响应式布局 */
@media (max-width: 768px) {
  .volume-item {
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    border-radius: 12px;
    align-items: stretch;
    border: 1px solid var(--border-color);
  }

  .volume-field {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
  }

  .volume-label {
    display: block;
    font-size: 12px;
    opacity: 0.8;
  }

  .volume-separator {
    align-self: center;
    transform: rotate(90deg);
    display: none;
  }

  .volume-options {
    flex: 1;
    justify-content: flex-start;
  }

  .volume-delete {
    flex: 0 0 auto;
    width: auto;
  }

  .volume-delete :deep(.n-button) {
    width: 100%;
  }
}
</style>

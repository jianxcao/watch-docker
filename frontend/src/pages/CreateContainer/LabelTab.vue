<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="label-tab">
      <n-space vertical size="large">
        <div>
          <n-h3 prefix="bar" class="mt-0">容器标签</n-h3>
          <n-text depth="3" class="text-sm mb-4 block">
            标签是键值对形式的元数据，用于组织和分类容器。可以用于标识容器的用途、版本、环境等信息。
          </n-text>

          <n-space vertical size="small">
            <div v-for="(label, index) in formValue.labelList" :key="index" class="label-item">
              <div class="label-field">
                <span class="label-label">KEY</span>
                <n-input
                  v-model:value="label.key"
                  placeholder="键"
                  size="small"
                  @blur="updateLabels"
                />
              </div>
              <div class="label-separator">
                <span>=</span>
              </div>
              <div class="label-field">
                <span class="label-label">VALUE</span>
                <n-input
                  v-model:value="label.value"
                  placeholder="值"
                  size="small"
                  @blur="updateLabels"
                />
              </div>
              <div class="label-delete">
                <n-button size="small" tertiary type="error" @click="removeLabel(index)">
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                  <span v-if="isMobile">删除</span>
                </n-button>
              </div>
            </div>
            <n-button dashed block @click="addLabel" size="small">
              <template #icon>
                <n-icon><AddOutline /></n-icon>
              </template>
              添加标签
            </n-button>
          </n-space>

          <n-divider />

          <n-text depth="3" style="font-size: 13px">
            <p style="margin: 8px 0"><strong>常用标签示例：</strong></p>
            <ul style="margin: 8px 0; padding-left: 20px">
              <li>environment=production (环境标识)</li>
              <li>version=1.0.0 (版本号)</li>
              <li>team=backend (团队标识)</li>
              <li>project=myapp (项目名称)</li>
            </ul>
          </n-text>
        </div>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import type { FormInst } from 'naive-ui'
import type { LabelFormValue } from './types'
import { useResponsive } from '@/hooks/useResponsive'

const { isMobile } = useResponsive()

const formValue = defineModel<LabelFormValue>({
  default: () => ({
    labelList: [],
    labels: {},
  }),
})

const formRef = ref<FormInst | null>(null)

const addLabel = () => {
  formValue.value.labelList.push({ key: '', value: '' })
}

const removeLabel = (index: number) => {
  formValue.value.labelList.splice(index, 1)
  updateLabels()
}

const updateLabels = () => {
  const labels: Record<string, string> = {}
  formValue.value.labelList.forEach((label) => {
    if (label.key.trim()) {
      labels[label.key] = label.value
    }
  })
  formValue.value.labels = labels
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.label-tab {
  padding: 0;
}

.label-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.label-field {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.label-field :deep(.n-input) {
  width: 100%;
}

.label-label {
  font-size: 14px;
  white-space: nowrap;
  display: none;
}

.label-separator {
  display: flex;
  align-items: center;
  opacity: 0.6;
  font-size: 16px;
}

.label-delete {
  flex: 0 0 44px;
  width: 44px;
}

/* 移动端响应式布局 */
@media (max-width: 768px) {
  .label-item {
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid var(--border-color);
    align-items: stretch;
  }

  .label-field {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
  }

  .label-label {
    display: block;
    font-size: 12px;
    opacity: 0.8;
  }

  .label-separator {
    align-self: center;
    transform: rotate(90deg);
    display: none;
  }

  .label-delete {
    flex: 0 0 auto;
    width: auto;
  }

  .label-delete :deep(.n-button) {
    width: 100%;
  }
}
</style>

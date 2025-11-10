<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="env-tab">
      <n-space vertical size="large">
        <div>
          <n-h3 prefix="bar" class="mt-0">环境变量</n-h3>
          <n-space vertical size="small">
            <div v-for="(envItem, index) in formValue.envList" :key="index" class="env-item">
              <div class="env-field">
                <span class="env-label">KEY</span>
                <n-input
                  v-model:value="envItem.key"
                  placeholder="KEY"
                  size="small"
                  @blur="updateEnv(index)"
                />
              </div>
              <div class="env-separator">
                <span>=</span>
              </div>
              <div class="env-field">
                <span class="env-label">VALUE</span>
                <n-input
                  v-model:value="envItem.value"
                  placeholder="value"
                  size="small"
                  @blur="updateEnv(index)"
                />
              </div>
              <div class="env-delete">
                <n-button size="small" tertiary type="error" @click="removeEnv(index)">
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                  <span v-if="isMobile">删除</span>
                </n-button>
              </div>
            </div>
            <n-button dashed block @click="addEnv" size="small">
              <template #icon>
                <n-icon><AddOutline /></n-icon>
              </template>
              添加环境变量
            </n-button>
          </n-space>
        </div>

        <n-divider />

        <div>
          <n-h3 prefix="bar">文本格式</n-h3>
          <n-text depth="3" class="text-sm mb-4 block"> 每行一个环境变量,格式: KEY=value </n-text>
          <n-input
            v-model:value="formValue.envText"
            type="textarea"
            placeholder="KEY=value&#10;ANOTHER_KEY=另一个值"
            :rows="8"
            @blur="handleEnvTextChange"
          />
        </div>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import type { FormInst } from 'naive-ui'
import type { EnvFormValue } from './types'
import { useResponsive } from '@/hooks/useResponsive'

const { isMobile } = useResponsive()

const formValue = defineModel<EnvFormValue>({
  default: () => ({
    env: [],
    envList: [],
    envText: '',
  }),
})

const formRef = ref<FormInst | null>(null)

const addEnv = () => {
  formValue.value.envList.push({ key: '', value: '' })
}

const removeEnv = (index: number) => {
  formValue.value.env.splice(index, 1)
  formValue.value.envList.splice(index, 1)
  formValue.value.envText = formValue.value.env.join('\n')
}

const updateEnv = (index: number) => {
  const item = formValue.value.envList[index]
  formValue.value.env[index] = `${item.key}=${item.value}`
  formValue.value.envText = formValue.value.env.join('\n')
}

const handleEnvTextChange = () => {
  nextTick(() => {
    const lines = formValue.value.envText.split('\n').filter((line) => line.trim())
    const newEnvList = lines.reduce((acc, line) => {
      let [key, value] = line.split('=')
      key = (key || '').trim()
      value = (value || '').trim()
      if (key) {
        acc.push(`${key}=${value}`)
      }
      return acc
    }, [] as string[])

    formValue.value.env = newEnvList
    formValue.value.envList = newEnvList.map((item) => {
      const [key, ...valueParts] = item.split('=')
      return {
        key: key || '',
        value: valueParts.join('=') || '',
      }
    })
  })
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.env-tab {
  padding: 0;
}

.env-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.env-field {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.env-field :deep(.n-input) {
  width: 100%;
}

.env-label {
  font-size: 14px;
  white-space: nowrap;
  display: none;
}

.env-separator {
  display: flex;
  align-items: center;
  opacity: 0.6;
  font-size: 16px;
}

.env-delete {
  flex: 0 0 44px;
  width: 44px;
}

/* 移动端响应式布局 */
@media (max-width: 768px) {
  .env-item {
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid var(--border-color);
    align-items: stretch;
  }

  .env-field {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
  }

  .env-label {
    display: block;
    font-size: 12px;
    opacity: 0.8;
  }

  .env-separator {
    align-self: center;
    transform: rotate(90deg);
    display: none;
  }

  .env-delete {
    flex: 0 0 auto;
    width: auto;
  }

  .env-delete :deep(.n-button) {
    width: 100%;
  }
}
</style>

<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="port-tab">
      <n-space vertical size="large">
        <div>
          <n-h3 prefix="bar" class="mt-0">端口映射</n-h3>
          <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">
            将主机端口映射到容器端口
          </n-text>
          <n-space vertical size="small">
            <div v-for="(port, index) in formValue.portList" :key="index" class="port-item">
              <div class="port-field">
                <span class="port-label">主机</span>
                <n-input-number
                  v-model:value="port.hostPort"
                  placeholder="8080"
                  :min="1"
                  :max="65535"
                  size="small"
                  @blur="updatePorts"
                />
              </div>
              <div class="port-arrow">
                <n-icon><ArrowForwardOutline /></n-icon>
              </div>
              <div class="port-field">
                <span class="port-label">容器</span>
                <n-input-number
                  v-model:value="port.containerPort"
                  placeholder="80"
                  :min="1"
                  :max="65535"
                  size="small"
                  @blur="updatePorts"
                />
              </div>
              <div class="port-protocol">
                <n-select
                  v-model:value="port.protocol"
                  :options="protocolOptions"
                  size="small"
                  multiple
                  @update:value="updatePorts"
                />
              </div>
              <div class="port-delete">
                <n-button size="small" tertiary type="error" @click="removePort(index)">
                  <template #icon>
                    <n-icon><CloseOutline /></n-icon>
                  </template>
                  <span v-if="isMobile">删除</span>
                </n-button>
              </div>
            </div>
            <n-button dashed block @click="addPort" size="small">
              <template #icon>
                <n-icon><AddOutline /></n-icon>
              </template>
              添加端口映射
            </n-button>
          </n-space>
        </div>

        <n-divider />

        <div v-if="formValue.exposedPorts && Object.keys(formValue.exposedPorts).length > 0">
          <n-h3 prefix="bar">预制端口列表</n-h3>
          <n-alert title="预制端口列表" type="info">
            <template #icon>
              <n-icon><InformationCircleOutline /></n-icon>
            </template>
            <div>
              <n-text>
                {{ Object.keys(formValue.exposedPorts).join(', ') }}
              </n-text>
            </div>
          </n-alert>
        </div>

        <n-form-item label="映射所有预设端口">
          <n-switch v-model:value="formValue.publishAllPorts" />
        </n-form-item>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import {
  AddOutline,
  ArrowForwardOutline,
  CloseOutline,
  InformationCircleOutline,
} from '@vicons/ionicons5'
import type { PortBinding } from '@/common/types'
import type { FormInst } from 'naive-ui'
import type { PortFormValue } from './types'
import { useResponsive } from '@/hooks/useResponsive'

const { isMobile } = useResponsive()

const formValue = defineModel<PortFormValue>({
  default: () => ({
    portList: [],
    portBindings: {},
    publishAllPorts: false,
    exposedPorts: {},
  }),
})

const formRef = ref<FormInst | null>(null)

const protocolOptions = [
  { label: 'TCP', value: 'tcp' },
  { label: 'UDP', value: 'udp' },
]

const addPort = () => {
  formValue.value.portList.push({
    hostPort: null,
    containerPort: null,
    protocol: ['tcp', 'udp'] as ('tcp' | 'udp')[],
  })
}

const removePort = (index: number) => {
  formValue.value.portList.splice(index, 1)
  updatePorts()
}

const updatePorts = () => {
  const portBindings: Record<string, PortBinding[]> = {}

  formValue.value.portList.forEach((port) => {
    if (port.containerPort && port.protocol.length > 0) {
      port.protocol.forEach((protocol) => {
        const key = `${port.containerPort}/${protocol}`
        if (port.hostPort) {
          if (!portBindings[key]) {
            portBindings[key] = []
          }
          portBindings[key].push({
            hostIP: '',
            hostPort: port.hostPort.toString(),
          })
        }
      })
    }
  })

  formValue.value.portBindings = portBindings
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.port-tab {
  padding: 0;
}

.port-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.port-field {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.port-field :deep(.n-input-number) {
  width: 100%;
}

.port-label {
  font-size: 14px;
  white-space: nowrap;
}

.port-arrow {
  display: flex;
  align-items: center;
  opacity: 0.6;
}

.port-protocol {
  flex: 0 0 180px;
  min-width: 0;
}

.port-delete {
  flex: 0 0 44px;
  width: 44px;
}

/* 移动端响应式布局 */
@media (max-width: 768px) {
  .port-item {
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid var(--border-color);
    align-items: stretch;
  }

  .port-field {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
  }

  .port-label {
    font-size: 12px;
    opacity: 0.8;
  }

  .port-arrow {
    align-self: center;
    transform: rotate(90deg);
  }

  .port-protocol {
    flex: 1;
  }

  .port-delete {
    flex: 0 0 auto;
    width: auto;
  }

  .port-delete :deep(.n-button) {
    width: 100%;
  }
}
</style>

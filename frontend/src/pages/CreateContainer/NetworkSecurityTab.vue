<template>
  <div class="network-security-tab">
    <n-space vertical size="large">
      <div>
        <n-h3 prefix="bar" class="mt-0">网络设置</n-h3>
        
        <n-form-item label="网络模式" path="networkMode">
          <n-select
            v-model:value="formData.networkMode"
            :options="networkModeOptions"
            placeholder="选择网络模式"
            @update:value="updateNetworkMode"
          />
        </n-form-item>

        <n-form-item label="发布所有已曝光的端口">
          <n-switch v-model:value="formData.publishAllPorts" @update:value="updateField" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              自动将容器的所有曝光端口映射到主机的随机端口
            </n-text>
          </template>
        </n-form-item>
      </div>

      <n-divider />

      <div>
        <n-h3 prefix="bar">安全性设置</n-h3>

        <n-form-item label="特权模式">
          <n-switch v-model:value="formData.privileged" @update:value="updateField" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              授予容器扩展权限,允许访问所有设备
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="只读根文件系统">
          <n-switch v-model:value="formData.readonlyRootfs" @update:value="updateField" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              将容器的根文件系统挂载为只读
            </n-text>
          </template>
        </n-form-item>

        <n-form-item label="退出时自动移除容器">
          <n-switch v-model:value="formData.autoRemove" @update:value="updateField" />
          <template #feedback>
            <n-text depth="3" style="font-size: 12px">
              容器退出时自动删除
            </n-text>
          </template>
        </n-form-item>
      </div>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ContainerCreateRequest } from '@/common/types'

interface Props {
  modelValue: Partial<ContainerCreateRequest>
}

interface Emits {
  (e: 'update:modelValue', value: Partial<ContainerCreateRequest>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const formData = ref({
  networkMode: props.modelValue.networkMode || 'bridge',
  publishAllPorts: props.modelValue.publishAllPorts || false,
  privileged: props.modelValue.privileged || false,
  readonlyRootfs: props.modelValue.readonlyRootfs || false,
  autoRemove: props.modelValue.autoRemove || false,
})

const networkModeOptions = [
  { label: 'Bridge (默认)', value: 'bridge' },
  { label: 'Host', value: 'host' },
  { label: 'None', value: 'none' },
  { label: 'Container', value: 'container' },
]

const updateField = () => {
  emit('update:modelValue', {
    ...props.modelValue,
    ...formData.value,
  })
}

const updateNetworkMode = () => {
  updateField()
}

watch(
  () => props.modelValue,
  (newVal) => {
    formData.value = {
      networkMode: newVal.networkMode || 'bridge',
      publishAllPorts: newVal.publishAllPorts || false,
      privileged: newVal.privileged || false,
      readonlyRootfs: newVal.readonlyRootfs || false,
      autoRemove: newVal.autoRemove || false,
    }
  },
  { deep: true },
)
</script>

<style scoped>
.network-security-tab {
  padding: 0;
}
</style>


<template>
  <n-form ref="formRef" :model="formValue" label-placement="top">
    <div class="runtime-resource-tab">
      <n-space vertical size="large">
        <!-- 安全性设置 -->
        <div>
          <n-h3 prefix="bar" class="mt-0">安全性设置</n-h3>
          <n-grid :cols="isMobile ? 1 : 2" :x-gap="12">
            <n-gi>
              <n-form-item label="特权模式">
                <n-switch v-model:value="formValue.privileged" />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px">
                    授予容器扩展权限,允许访问所有设备
                  </n-text>
                </template>
              </n-form-item>
            </n-gi>
            <n-gi>
              <n-form-item label="只读根文件系统">
                <n-switch v-model:value="formValue.readonlyRootfs" />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px"> 将容器的根文件系统挂载为只读 </n-text>
                </template>
              </n-form-item>
            </n-gi>
            <n-gi>
              <n-form-item label="退出时自动移除容器">
                <n-switch v-model:value="formValue.autoRemove" />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px"> 容器退出时自动删除 </n-text>
                </template>
              </n-form-item></n-gi
            >
          </n-grid>
        </div>

        <n-divider />

        <!-- 重启策略 -->
        <div>
          <n-h3 prefix="bar">重启策略</n-h3>
          <n-grid :cols="isMobile ? 1 : 2" :x-gap="12">
            <n-gi>
              <n-form-item label="策略" path="restartPolicyName">
                <n-select
                  v-model:value="formValue.restartPolicyName"
                  :options="restartPolicyOptions"
                />
              </n-form-item>
            </n-gi>
            <n-gi>
              <n-form-item
                :label="formValue.restartPolicyName === 'on-failure' ? '最大重试次数' : ''"
                path="restartPolicyMaxRetry"
              >
                <n-input-number
                  v-model:value="formValue.restartPolicyMaxRetry"
                  :disabled="formValue.restartPolicyName !== 'on-failure'"
                  :min="0"
                  placeholder="0 表示无限制"
                  class="w-full"
                />
              </n-form-item>
            </n-gi>
          </n-grid>
        </div>

        <n-divider />

        <!-- 资源限制 -->
        <div>
          <n-h3 prefix="bar">资源限制</n-h3>

          <n-grid :cols="isMobile ? 1 : 2" :x-gap="12">
            <n-gi>
              <n-form-item label="内存限制 (MB)">
                <n-input-number
                  v-model:value="formValue.memoryMB"
                  :min="0"
                  placeholder="0 表示不限制"
                  style="width: 100%"
                />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px">
                    容器可使用的最大内存量（硬限制），超过会被杀掉
                  </n-text>
                </template>
              </n-form-item>
            </n-gi>
            <n-gi>
              <n-form-item label="内存预留 (MB)">
                <n-input-number
                  v-model:value="formValue.memoryReservationMB"
                  :min="0"
                  placeholder="0 表示不限制"
                  style="width: 100%"
                />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px">
                    内存软限制。内存紧张时优先回收超过此值的内存，必须小于内存限制
                  </n-text>
                </template>
              </n-form-item>
            </n-gi>
          </n-grid>

          <n-grid :cols="isMobile ? 1 : 2" :x-gap="12">
            <n-gi>
              <n-form-item label="CPU 核心 (CPUs)">
                <n-input v-model:value="formValue.cpusetCpus" placeholder="例如: 0-3, 0,1" />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px">
                    限制容器只能使用指定的 CPU 核心。格式：0-3（核心0到3）或 0,2（核心0和2）
                  </n-text>
                </template>
              </n-form-item>
            </n-gi>
            <n-gi>
              <n-form-item label="共享内存大小 (MB)">
                <n-input-number
                  v-model:value="formValue.shmSizeMB"
                  :min="0"
                  placeholder="默认 64"
                  style="width: 100%"
                />
                <template #feedback>
                  <n-text depth="3" style="font-size: 12px"> 设置 /dev/shm 的大小 </n-text>
                </template>
              </n-form-item>
            </n-gi>
          </n-grid>
        </div>
      </n-space>
    </div>
  </n-form>
</template>

<script setup lang="ts">
import type { FormInst } from 'naive-ui'
import type { RuntimeResourceFormValue } from './types'
import { useResponsive } from '@/hooks/useResponsive'
const { isMobile } = useResponsive()

const formValue = defineModel<RuntimeResourceFormValue>({
  default: () => ({
    privileged: false,
    readonlyRootfs: false,
    autoRemove: false,
    restartPolicyName: 'unless-stopped',
    restartPolicyMaxRetry: 0,
    memoryMB: 0,
    memoryReservationMB: 0,
    cpusetCpus: '',
    shmSizeMB: 0,
  }),
})

const formRef = ref<FormInst | null>(null)

const restartPolicyOptions = [
  { label: '不适用 (no)', value: 'no' },
  { label: '总是重启 (always)', value: 'always' },
  { label: '除非停止 (unless-stopped)', value: 'unless-stopped' },
  { label: '失败时 (on-failure)', value: 'on-failure' },
]

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.runtime-resource-tab {
  padding: 0;
}
</style>

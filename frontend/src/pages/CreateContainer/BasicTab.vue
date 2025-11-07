<template>
  <div class="basic-tab">
    <n-form-item label="容器名称" path="name" required>
      <n-input
        v-model:value="formData.name"
        placeholder="请输入容器名称(可选,留空则自动生成)"
        :maxlength="100"
        show-count
      />
    </n-form-item>

    <n-form-item label="镜像" path="image" required>
      <n-input v-model:value="formData.image" placeholder="例如: nginx:latest" :maxlength="200">
        <template #suffix>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-icon :component="InformationCircleOutline" />
            </template>
            输入完整的镜像名称,包括标签
          </n-tooltip>
        </template>
      </n-input>
    </n-form-item>

    <n-grid :cols="2" :x-gap="12">
      <n-gi>
        <n-form-item label="命令" path="cmd">
          <n-input v-model:value="cmdString" placeholder="例如: /bin/bash" @blur="handleCmdChange">
            <template #suffix>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon :component="InformationCircleOutline" />
                </template>
                覆盖镜像的默认命令,多个参数用空格分隔
              </n-tooltip>
            </template>
          </n-input>
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="工作目录" path="workingDir">
          <n-input v-model:value="formData.workingDir" placeholder="例如: /app" />
        </n-form-item>
      </n-gi>
    </n-grid>

    <n-grid :cols="2" :x-gap="12">
      <n-gi>
        <n-form-item label="用户" path="user">
          <n-input v-model:value="formData.user" placeholder="例如: 1000:1000 或 username">
            <template #suffix>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon :component="InformationCircleOutline" />
                </template>
                格式: uid:gid 或 username
              </n-tooltip>
            </template>
          </n-input>
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="主机名" path="hostname">
          <n-input v-model:value="formData.hostname" placeholder="容器的主机名" />
        </n-form-item>
      </n-gi>
    </n-grid>

    <!-- <n-form-item label="域名" path="domainname">
      <n-input v-model:value="formData.domainname" placeholder="容器的域名" />
    </n-form-item> -->

    <n-divider />
    <n-h3 prefix="bar" class="mt-0">I/O 设置</n-h3>

    <!-- <n-grid :cols="3" :x-gap="12">
      <n-gi>
        <n-form-item label="Attach stdout">
          <n-switch v-model:value="formData.attachStdout" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="Attach stderr">
          <n-switch v-model:value="formData.attachStderr" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="附加 stdin">
          <n-switch v-model:value="formData.attachStdin" />
        </n-form-item>
      </n-gi>
    </n-grid> -->

    <n-grid :cols="3" :x-gap="12">
      <n-gi>
        <n-form-item label="TTY">
          <n-switch v-model:value="formData.tty" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="stdin">
          <n-switch v-model:value="formData.openStdin" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="StdinOnce">
          <n-switch v-model:value="formData.stdinOnce" />
        </n-form-item>
      </n-gi>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { InformationCircleOutline } from '@vicons/ionicons5'
import { ref, watch } from 'vue'
import type { ContainerCreateRequest } from '@/common/types'
import { NGrid } from 'naive-ui'

interface Props {
  modelValue: Partial<ContainerCreateRequest>
}

interface Emits {
  (e: 'update:modelValue', value: Partial<ContainerCreateRequest>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const formData = ref<Partial<ContainerCreateRequest>>({
  name: '',
  image: '',
  cmd: [],
  workingDir: '',
  user: '',
  hostname: '',
  domainname: '',
  attachStdout: true,
  attachStderr: true,
  attachStdin: false,
  tty: false,
  openStdin: false,
  stdinOnce: false,
  ...props.modelValue,
})

const cmdString = ref(formData.value.cmd?.join(' ') || '')

const handleCmdChange = () => {
  if (cmdString.value.trim()) {
    formData.value.cmd = cmdString.value.trim().split(/\s+/)
  } else {
    formData.value.cmd = []
  }
}

watch(
  formData,
  (newVal) => {
    emit('update:modelValue', newVal)
  },
  { deep: true },
)

watch(
  () => props.modelValue,
  (newVal) => {
    formData.value = { ...formData.value, ...newVal }
    cmdString.value = formData.value.cmd?.join(' ') || ''
  },
  { deep: true },
)
</script>

<style scoped>
.basic-tab {
  padding: 0;
}
</style>

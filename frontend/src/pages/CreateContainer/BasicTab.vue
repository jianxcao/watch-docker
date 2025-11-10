<template>
  <n-form
    ref="formRef"
    :model="formValue"
    :show-feedback="false"
    :rules="rules"
    label-placement="top"
    class="basic-tab"
  >
    <n-form-item label="容器名称" path="name" class="mb-4" required>
      <n-input
        v-model:value="formValue.name"
        placeholder="请输入容器名称"
        :maxlength="100"
        show-count
      />
    </n-form-item>

    <n-form-item label="镜像" path="image" class="mb-4" required>
      <n-input v-model:value="formValue.image" placeholder="例如: nginx:latest" :maxlength="200">
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

    <n-grid cols="1 m:2" :x-gap="12" :y-gap="12" class="mb-4" responsive="screen">
      <n-gi>
        <n-form-item label="Entrypoint" path="entrypointString">
          <n-input v-model:value="formValue.entrypointString" placeholder="例如: /usr/bin/nginx">
            <template #suffix>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon :component="InformationCircleOutline" />
                </template>
                配置容器启动时要运行的可执行文件,多个参数用空格分隔
              </n-tooltip>
            </template>
          </n-input>
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="Cmd" path="cmdString">
          <n-input v-model:value="formValue.cmdString" placeholder="例如: /bin/bash">
            <template #suffix>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon :component="InformationCircleOutline" />
                </template>
                提供给 Entrypoint 的参数或默认命令,多个参数用空格分隔
              </n-tooltip>
            </template>
          </n-input>
        </n-form-item>
      </n-gi>
    </n-grid>

    <n-form-item label="工作目录" path="workingDir" class="mb-4">
      <n-input v-model:value="formValue.workingDir" placeholder="例如: /app" />
    </n-form-item>

    <n-form-item label="用户" path="user" class="mb-4">
      <n-input v-model:value="formValue.user" placeholder="例如: 1000:1000 或 username">
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

    <n-grid cols="1 m:2" :x-gap="12" :y-gap="12" class="mb-4" responsive="screen">
      <n-gi>
        <n-form-item label="主机名 (Hostname)" path="hostname">
          <n-input v-model:value="formValue.hostname" placeholder="例如: mycontainer" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="域名 (Domain Name)" path="domainname">
          <n-input v-model:value="formValue.domainname" placeholder="例如: example.com" />
        </n-form-item>
      </n-gi>
    </n-grid>

    <n-divider />
    <n-h3 prefix="bar" class="mt-0">I/O 设置</n-h3>

    <n-grid :cols="3" :x-gap="12" class="mb-4">
      <n-gi>
        <n-form-item label="TTY">
          <n-switch v-model:value="formValue.tty" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="stdin">
          <n-switch v-model:value="formValue.openStdin" />
        </n-form-item>
      </n-gi>
      <n-gi>
        <n-form-item label="StdinOnce">
          <n-switch v-model:value="formValue.stdinOnce" />
        </n-form-item>
      </n-gi>
    </n-grid>
  </n-form>
</template>

<script setup lang="ts">
import { InformationCircleOutline } from '@vicons/ionicons5'
import type { FormInst, FormRules } from 'naive-ui'
import type { BasicFormValue } from './types'

const formValue = defineModel<BasicFormValue>({
  default: () => ({
    name: '',
    image: '',
    entrypointString: '',
    cmdString: '',
    workingDir: '',
    user: '',
    hostname: '',
    domainname: '',
    tty: false,
    openStdin: false,
    stdinOnce: false,
  }),
})

const formRef = ref<FormInst | null>(null)

const rules: FormRules = {
  name: [
    {
      required: true,
      message: '请输入容器名称',
      trigger: ['input', 'blur'],
    },
    {
      pattern: /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/,
      message: '容器名称只能包含字母、数字、下划线、点和连字符，且必须以字母或数字开头',
      trigger: ['input', 'blur'],
    },
  ],
  image: [
    {
      required: true,
      message: '请输入镜像名称',
      trigger: ['input', 'blur'],
    },
  ],
}

const validate = () => formRef.value?.validate()
const restoreValidation = () => formRef.value?.restoreValidation()

defineExpose({
  validate,
  restoreValidation,
})
</script>

<style scoped>
.basic-tab {
  padding: 0;
}
</style>

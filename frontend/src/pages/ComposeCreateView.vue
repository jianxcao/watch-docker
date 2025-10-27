<template>
  <div class="compose-create-page">
    <!-- 页面头部 -->
    <Teleport to="#header" defer>
      <div class="page-header">
        <n-button text circle @click="handleBack">
          <template #icon>
            <n-icon size="20">
              <ArrowBackOutline />
            </n-icon>
          </template>
        </n-button>
        <n-h2 class="m-0 text-lg">创建 Compose 项目</n-h2>
      </div>
    </Teleport>

    <!-- 表单内容 -->
    <n-card title="基本信息" size="small" :bordered="false" class="mb-2">
      <n-form
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-placement="left"
        label-width="100"
      >
        <!-- 项目名称 -->
        <n-form-item label="项目名称" path="name">
          <n-input
            v-model:value="formData.name"
            placeholder="请输入项目名称（支持字母、数字、下划线、连字符）"
            @input="handleNameChange"
            clearable
          />
        </n-form-item>

        <!-- 存放路径 -->
        <n-form-item label="存放路径" path="path">
          <n-input
            v-model:value="formData.path"
            placeholder="路径将根据项目名称自动生成"
            disabled
            readonly
          >
            <template #suffix>
              <n-icon>
                <FolderOpenOutline />
              </n-icon>
            </template>
          </n-input>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card title="App 配置" size="small" :bordered="false">
      <template #header-extra>
        <n-space align="center">
          <n-text depth="3" class="text-xs" :style="{ color: theme.errorColor }">
            {{ yamlValidationMessage }}
          </n-text>
          <n-button
            text
            size="small"
            @click="handleImportFile"
            :disabled="importLoading"
            :loading="importLoading"
          >
            <template #icon>
              <n-icon>
                <CloudUploadOutline />
              </n-icon>
            </template>
            导入文件
          </n-button>
        </n-space>
      </template>

      <!-- YAML 编辑器 -->
      <div ref="yamlEditorContainerRef">
        <YamlEditor
          v-model="formData.yaml"
          placeholder="请输入 docker-compose.yml 配置内容"
          :min-height="yamlEditorMinHeight"
          max-height="100vh"
          @change="handleYamlChange"
        />
      </div>

      <!-- 隐藏的文件输入 -->
      <input
        ref="fileInputRef"
        type="file"
        accept=".yml,.yaml"
        style="display: none"
        @change="handleFileSelect"
      />
    </n-card>

    <!-- 创建进度组件 -->
    <ComposeCreateProgress
      ref="createProgressRef"
      :show="showProgress"
      :project-name="formData.name"
      :yaml-content="formData.yaml"
      :force="true"
      @success="handleCreateSuccess"
      @error="handleCreateError"
      @complete="handleCreateComplete"
    />

    <Teleport to="#footer" defer>
      <n-space justify="end" class="pr-2">
        <n-button @click="handleCancel" :disabled="submitting">取消</n-button>
        <n-button
          type="primary"
          @click="handleSubmit"
          :loading="submitting"
          :disabled="!isFormValid || submitting"
        >
          {{ submitting ? '创建中...' : '创建并启动项目' }}
        </n-button>
      </n-space>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useThemeVars, type FormInst, type FormRules } from 'naive-ui'
import { ArrowBackOutline, FolderOpenOutline, CloudUploadOutline } from '@vicons/ionicons5'
import { useSettingStore } from '@/store/setting'
import { validateComposeYaml } from '@/common/utils'
import YamlEditor from '@/components/YamlEditor/index.vue'
import ComposeCreateProgress from '@/components/ComposeCreateProgress.vue'
import { useComposeStore } from '@/store/compose'

const router = useRouter()
const message = useMessage()
const settingStore = useSettingStore()
const theme = useThemeVars()
const composeStore = useComposeStore()

// 表单引用和数据
const formRef = ref<FormInst | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const submitting = ref(false)
const importLoading = ref(false)
const yamlEditorContainerRef = ref<HTMLElement | null>(null)
const yamlEditorMinHeight = computed(() => {
  if (yamlEditorContainerRef.value) {
    const t = yamlEditorContainerRef.value.getBoundingClientRect()
    return `calc(100vh - ${t.top + settingStore.safeArea.bottom + 56}px)`
  }
  return '400px'
})

// 创建进度组件引用
const createProgressRef = ref<InstanceType<typeof ComposeCreateProgress>>()
const showProgress = ref(false)

// 表单数据
const formData = ref({
  name: '',
  path: '',
  yaml: '',
})

// 获取 APP_PATH（从系统信息中获取）
const appPath = computed(() => {
  return settingStore.systemInfo?.appPath || '/data/compose'
})

// YAML 验证状态
const yamlValidationMessage = ref('')
const isYamlValid = ref(true)

// 表单验证规则
const rules: FormRules = {
  name: [
    { required: true, message: '请输入项目名称', trigger: 'blur' },
    {
      pattern: /^[a-zA-Z0-9_-]+$/,
      message: '项目名称只能包含字母、数字、下划线和连字符',
      trigger: 'blur',
    },
    { min: 1, max: 50, message: '项目名称长度应在 1-50 之间', trigger: 'blur' },
  ],
  path: [{ required: true, message: '存放路径不能为空' }],
  yaml: [
    { required: true, message: '请输入 Compose 配置', trigger: 'blur' },
    {
      validator: () => {
        if (!isYamlValid.value) {
          return new Error('YAML 格式不正确')
        }
        return true
      },
      trigger: 'change',
    },
  ],
}

// 表单是否有效
const isFormValid = computed(() => {
  return formData.value.name && formData.value.path && formData.value.yaml && isYamlValid.value
})

// 处理项目名称变化，自动生成路径
const handleNameChange = () => {
  const name = formData.value.name.trim()
  if (name) {
    formData.value.path = `${appPath.value}/${name}`
  } else {
    formData.value.path = ''
  }
}

// YAML 变化时进行简单验证
const handleYamlChange = () => {
  const result = validateComposeYaml(formData.value.yaml)
  isYamlValid.value = result.isValid
  yamlValidationMessage.value = result.errorMessage
}

// 导入 YAML 文件
const handleImportFile = () => {
  fileInputRef.value?.click()
}

// 处理文件选择
const handleFileSelect = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) {
    return
  }

  importLoading.value = true
  try {
    const content = await file.text()
    formData.value.yaml = content
    handleYamlChange()
    message.success('导入成功')
  } catch (error) {
    console.error('导入文件失败:', error)
    message.error(`导入文件失败: ${(error as Error).message}`)
  } finally {
    importLoading.value = false
    if (target) {
      target.value = ''
    }
  }
}

// 提交表单
const handleSubmit = async () => {
  if (!formRef.value) {
    return
  }

  try {
    await formRef.value.validate()
  } catch (error) {
    console.error('表单验证失败:', error)
    return
  }

  submitting.value = true
  showProgress.value = true

  // 启动创建进度
  createProgressRef.value?.start()
}

// 创建成功
const handleCreateSuccess = (composeFile: string) => {
  message.success('项目创建并启动成功')
  console.log('Compose file:', composeFile)
}

// 创建失败
const handleCreateError = (errorMessage: string) => {
  message.error(`创建失败: ${errorMessage}`)
  submitting.value = false
}

// 创建完成（无论成功失败）
const handleCreateComplete = () => {
  submitting.value = false
  // 延迟跳转，让用户看到完成信息
  setTimeout(() => {
    // 刷新 compose
    composeStore.fetchProjects(true)
    router.push({ name: 'compose' })
  }, 2000)
}

// 返回
const handleBack = () => {
  router.back()
}

// 取消
const handleCancel = () => {
  router.push({ name: 'compose' })
}

// 初始化
onMounted(() => {
  // 加载系统信息
  if (!settingStore.systemInfo) {
    settingStore.fetchSystemInfo()
  }
})
</script>

<style lang="less">
.layout-compose-create {
  .page-header {
    display: flex;
    flex-direction: row;
    align-items: center;
    height: 100%;
    gap: 16px;
  }
}
</style>

<template>
  <div class="login-page">
    <div class="login-container">
      <!-- Logo 和标题 -->
      <div class="login-header">
        <img src="/logo.svg" alt="Watch Docker" class="logo" />
        <div> <n-h2 class="app-title">Watch Docker</n-h2>
          <span class="text-subtitle">容器版本监控与自动更新平台</span>
        </div>
      </div>

      <!-- 登录表单 -->
      <n-card class="login-card" embedded>
        <n-form ref="formRef" :model="loginForm" :rules="rules" label-placement="top"
          require-mark-placement="right-hanging" size="large" :show-feedback="false" :show-label="false">
          <n-form-item label="用户名" path="username" class="pb-2">
            <n-input v-model:value="loginForm.username" placeholder="请输入用户名" :disabled="loginLoading"
              @keydown.enter="handleLogin" />
          </n-form-item>

          <n-form-item label="密码" path="password" class="pb-2">
            <n-input v-model:value="loginForm.password" type="password" placeholder="请输入密码" show-password-on="click"
              :disabled="loginLoading" @keydown.enter="handleLogin" />
          </n-form-item>

          <n-form-item>
            <n-checkbox v-model:checked="rememberUsername" :disabled="loginLoading">
              记住用户名
            </n-checkbox>
          </n-form-item>

          <n-form-item>
            <n-button type="primary" block size="large" :loading="loginLoading" @click="handleLogin">
              登录
            </n-button>
          </n-form-item>
        </n-form>
      </n-card>

      <!-- 系统信息 -->
      <div class="system-info" v-if="settingStore.systemInfo">
        <n-card size="small" embedded>
          <n-space justify="space-between" align="center">
            <n-text depth="3">Docker: {{ settingStore.systemInfo.dockerVersion }}</n-text>
            <n-text depth="3">{{ settingStore.systemInfo.version }}</n-text>
          </n-space>
        </n-card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, type FormInst, type FormRules } from 'naive-ui'
import { useAuthStore } from '@/store/auth'
import { useSettingStore } from '@/store/setting'

// 路由和消息
const router = useRouter()
const message = useMessage()

// 状态管理
const authStore = useAuthStore()
const settingStore = useSettingStore()

// 表单相关
const formRef = ref<FormInst | null>(null)
const loginForm = ref({
  username: '',
  password: ''
})

// 响应式状态
const loginLoading = ref(false)
const rememberUsername = ref(false)

// 表单验证规则
const rules: FormRules = {
  username: {
    required: true,
    message: '请输入用户名',
    trigger: ['input', 'blur']
  },
  password: {
    required: true,
    message: '请输入密码',
    trigger: ['input', 'blur']
  }
}

// 用户名操作（安全优化：不再保存密码）
const loadSavedUsername = () => {
  if (settingStore.setting.rememberUsername) {
    loginForm.value.username = settingStore.setting.rememberedUsername
    rememberUsername.value = true
  }
}

// 登录处理
const handleLogin = async () => {
  try {
    await formRef.value?.validate()
    loginLoading.value = true

    const result = await authStore.login(loginForm.value.username, loginForm.value.password)

    if (result.success) {
      // 登录成功后保存用户名（根据用户选择）
      settingStore.saveRememberedUsername(
        loginForm.value.username,
        rememberUsername.value
      )
      message.success('登录成功')
      // 登录成功后跳转到主页
      router.push('/')
    } else {
      // 登录失败时，如果用户没有选择记住用户名，清除已保存的用户名
      if (!rememberUsername.value) {
        settingStore.clearRememberedUsername()
      }
      message.error(result.message || '登录失败')
    }
  } catch (error) {
    console.error('Login validation failed:', error)
    // 登录异常时，如果用户没有选择记住用户名，清除已保存的用户名
    if (!rememberUsername.value) {
      settingStore.clearRememberedUsername()
    }
  } finally {
    loginLoading.value = false
  }
}

// 监听记住用户名选项变化
watch(rememberUsername, (newValue) => {
  // 如果用户取消勾选记住用户名，立即清除保存的用户名
  if (!newValue) {
    settingStore.clearRememberedUsername()
  }
})

// 组件挂载时
onMounted(() => {
  // 加载保存的用户名（安全优化：不再加载密码）
  loadSavedUsername()
  // 获取系统信息
  settingStore.fetchSystemInfo()
})
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 20px;
}

.login-container {
  width: 100%;
  max-width: 400px;
}

.login-header {
  text-align: center;
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: center;
  margin-bottom: 32px;
}

.logo {
  width: 72px;
  height: 72px;
}

.app-title {
  margin: 16px 0 8px 0;
  color: white;
}

.text-subtitle {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
}

.login-card {
  margin-bottom: 20px;
}

.system-info {
  text-align: center;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .login-container {
    max-width: 100%;
    margin: 0 16px;
  }

  .logo {
    width: 64px;
    height: 64px;
  }
}
</style>

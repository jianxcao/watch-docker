import { defineStore } from 'pinia'
import { ref, reactive } from 'vue'
import { useStorage } from '@vueuse/core'
import { useThemeVars, type CustomThemeCommonVars, type ThemeCommonVars } from 'naive-ui'
import { authApi } from '@/common/api'
import type { SystemInfo } from '@/common/types'

export const useSettingStore = defineStore('setting', () => {
  const setting = useStorage(
    'setting',
    {
      theme: 'light',
      language: 'zh-CN',
      rememberedUsername: '', // 重命名为记住的用户名
      rememberUsername: false, // 改为只记住用户名
      token: '',
    },
    localStorage,
  )

  const tmpToken = ref('')

  const currentUsername = ref(setting.value.rememberedUsername || '')

  const headerHeight = ref(56)

  const safeArea = reactive({
    top: 0,
    bottom: 0,
  })

  const doc = document.documentElement
  const docStyle = window.getComputedStyle(doc)
  safeArea.top = parseInt(docStyle.getPropertyValue('--top-inset')) || 0
  safeArea.bottom = parseInt(docStyle.getPropertyValue('--bottom-inset')) || 0
  const layoutPadding = parseInt(docStyle.getPropertyValue('--layout-padding')) || 0
  const contentSafeTop = ref(safeArea.top + headerHeight.value + layoutPadding)
  const contentSafeBottom = ref(safeArea.bottom + layoutPadding)

  watchEffect(() => {
    document.body.style.setProperty('--content-safe-top', `${contentSafeTop.value}px`)
    document.body.style.setProperty('--content-safe-bottom', `${contentSafeBottom.value}px`)
  })

  const themeDefault = useThemeVars()

  const themeVars = ref<ThemeCommonVars & CustomThemeCommonVars>(themeDefault.value)

  // 系统信息相关状态
  const systemInfo = ref<SystemInfo | null>(null)
  const systemInfoLoading = ref(false)

  function setTheme(val: string) {
    setting.value.theme = val
  }

  function setThemeVars(val: ThemeCommonVars & CustomThemeCommonVars) {
    themeVars.value = val
  }

  // 获取系统信息
  async function fetchSystemInfo() {
    if (systemInfoLoading.value) {
      return
    }

    systemInfoLoading.value = true
    try {
      const res = await authApi.getInfo()

      systemInfo.value = res.data.info
    } catch (error) {
      console.log('Failed to fetch system info:', error)
    } finally {
      systemInfoLoading.value = false
    }
  }

  // 保存记住的用户名（安全优化：不再保存密码）
  function saveRememberedUsername(username: string, remember: boolean) {
    if (remember) {
      setting.value.rememberedUsername = username
      setting.value.rememberUsername = true
    } else {
      clearRememberedUsername()
    }
  }

  // 清除记住的用户名
  function clearRememberedUsername() {
    setting.value.rememberedUsername = ''
    setting.value.rememberUsername = false
  }

  // 设置当前登录用户名
  function setCurrentUsername(username: string) {
    currentUsername.value = username
  }

  // 清除当前登录用户名
  function clearCurrentUsername() {
    currentUsername.value = ''
  }

  // 获取记住的用户名
  function getRememberedUsername() {
    return setting.value.rememberUsername ? setting.value.rememberedUsername : ''
  }

  // Token 管理
  function setToken(token: string) {
    setting.value.token = token
  }

  function setTmpToken(token: string) {
    tmpToken.value = token
  }

  function getToken() {
    return setting.value.token || tmpToken.value
  }

  function clearToken() {
    setting.value.token = ''
  }

  return {
    setting,
    setTheme,
    themeVars,
    setThemeVars,
    safeArea,
    headerHeight,
    systemInfo,
    systemInfoLoading,
    fetchSystemInfo,
    currentUsername,
    saveRememberedUsername,
    clearRememberedUsername,
    setCurrentUsername,
    clearCurrentUsername,
    getRememberedUsername,
    setToken,
    setTmpToken,
    getToken,
    clearToken,
    contentSafeTop,
    contentSafeBottom,
  }
})

import { defineStore } from 'pinia'
import { ref, reactive, computed, watchEffect } from 'vue'
import { useStorage } from '@vueuse/core'
import { lightTheme, type CustomThemeCommonVars, type ThemeCommonVars } from 'naive-ui'
import { authApi } from '@/common/api'
import type { SystemInfo } from '@/common/types'

const CURRENT_USERNAME_STORAGE_KEY = 'watch-docker:current-username'

function decodeJwtUsername(token: string): string {
  try {
    const parts = token.split('.')
    if (parts.length < 2) {
      return ''
    }
    const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
    const bytes = Uint8Array.from(atob(padded), (c) => c.charCodeAt(0))
    const payload = JSON.parse(new TextDecoder().decode(bytes))
    return typeof payload?.username === 'string' ? payload.username : ''
  } catch {
    return ''
  }
}

function loadInitialUsername(rememberedUsername: string, token: string): string {
  if (rememberedUsername) {
    return rememberedUsername
  }
  try {
    const stored = sessionStorage.getItem(CURRENT_USERNAME_STORAGE_KEY)
    if (stored) {
      return stored
    }
  } catch {
    // ignore storage errors
  }
  return token ? decodeJwtUsername(token) : ''
}

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

  const currentUsername = ref(
    loadInitialUsername(setting.value.rememberedUsername, setting.value.token),
  )

  const headerHeight = ref(56)

  const safeArea = reactive({
    top: 0,
    bottom: 0,
  })

  const doc = document.documentElement
  const docStyle = window.getComputedStyle(doc)
  safeArea.top = parseInt(docStyle.getPropertyValue('--top-inset')) || 0
  safeArea.bottom = parseInt(docStyle.getPropertyValue('--bottom-inset')) || 0
  const layoutPadding = ref(parseInt(docStyle.getPropertyValue('--layout-padding')) || 0)
  const contentSafeTop = computed(() => safeArea.top + headerHeight.value + layoutPadding.value)
  const contentSafeBottom = computed(() => safeArea.bottom + layoutPadding.value)

  watchEffect(() => {
    document.body.style.setProperty('--content-safe-top', `${contentSafeTop.value}px`)
    document.body.style.setProperty('--content-safe-bottom', `${contentSafeBottom.value}px`)
  })

  const themeVars = ref<ThemeCommonVars & CustomThemeCommonVars>(
    { ...lightTheme.common } as ThemeCommonVars & CustomThemeCommonVars,
  )

  // 系统信息相关状态
  const systemInfo = ref<SystemInfo | null>(null)
  const systemInfoLoading = ref(false)
  let systemInfoInFlight: Promise<SystemInfo | null> | null = null
  let lastFetchedToken: string | undefined

  function setTheme(val: string) {
    setting.value.theme = val
  }

  function setThemeVars(val: ThemeCommonVars & CustomThemeCommonVars) {
    themeVars.value = val
  }

  // 获取系统信息（按当前 token 去重并发请求，避免未认证请求覆盖已认证结果）
  async function fetchSystemInfo(force = false): Promise<SystemInfo | null> {
    const reqToken = getToken() || ''
    if (!force && systemInfoInFlight && lastFetchedToken === reqToken) {
      return systemInfoInFlight
    }

    lastFetchedToken = reqToken
    systemInfoLoading.value = true

    let reqPromise!: Promise<SystemInfo | null>
    reqPromise = (async (): Promise<SystemInfo | null> => {
      try {
        const res = await authApi.getInfo()
        if (res.code === 0 && res.data?.info && (getToken() || '') === reqToken) {
          systemInfo.value = res.data.info
          return res.data.info
        }
        return systemInfo.value
      } catch (error) {
        console.log('Failed to fetch system info:', error)
        return null
      } finally {
        if (systemInfoInFlight === reqPromise) {
          systemInfoInFlight = null
          systemInfoLoading.value = false
        }
      }
    })()

    systemInfoInFlight = reqPromise
    return reqPromise
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
    try {
      if (username) {
        sessionStorage.setItem(CURRENT_USERNAME_STORAGE_KEY, username)
      } else {
        sessionStorage.removeItem(CURRENT_USERNAME_STORAGE_KEY)
      }
    } catch {
      // ignore storage errors
    }
  }

  // 清除当前登录用户名
  function clearCurrentUsername() {
    currentUsername.value = ''
    try {
      sessionStorage.removeItem(CURRENT_USERNAME_STORAGE_KEY)
    } catch {
      // ignore storage errors
    }
  }

  // 获取记住的用户名
  function getRememberedUsername() {
    return setting.value.rememberUsername ? setting.value.rememberedUsername : ''
  }

  // Token 管理（严格区分完整访问令牌与 2FA 临时令牌）
  function setToken(token: string) {
    setting.value.token = token
  }

  function setTmpToken(token: string) {
    tmpToken.value = token
  }

  function getToken() {
    return setting.value.token
  }

  function getTmpToken() {
    return tmpToken.value
  }

  function clearToken() {
    setting.value.token = ''
    tmpToken.value = ''
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
    getTmpToken,
    clearToken,
    contentSafeTop,
    contentSafeBottom,
  }
})

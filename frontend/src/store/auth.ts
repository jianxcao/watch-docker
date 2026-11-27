import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/common/api'
import { navigateTo } from '@/router'
import { useSettingStore } from '@/store/setting'

export const useAuthStore = defineStore('auth', () => {
  // 获取 setting store 实例
  const settingStore = useSettingStore()

  // 登录状态
  const isLoggedIn = ref(false)

  // 身份验证是否启用
  const authEnabled = ref(false)

  // 二次验证状态
  const twoFARequired = ref(false)
  const twoFASetupRequired = ref(false)
  const twoFAAccountSetup = ref(false)
  const tempToken = ref('')
  const twoFAMethod = ref('')

  // 登录加载状态
  const loginLoading = ref(false)

  // 检查认证状态加载中
  const checkingAuth = ref(false)
  let initPromise: Promise<void> | null = null

  // 计算属性：是否需要登录
  const requiresAuth = computed(() => {
    return authEnabled.value && !isLoggedIn.value
  })

  // 计算属性：当前用户名
  const username = computed(() => {
    return settingStore.currentUsername
  })

  const setAuthStatus = (enabled: boolean) => {
    authEnabled.value = enabled
  }

  // 初始化：从setting store恢复token（不将 tempToken 视作完整登录态）
  const initAuth = () => {
    if (initPromise) {
      return initPromise
    }
    checkingAuth.value = true
    initPromise = (async () => {
      try {
        const authStatusRes = await authApi.checkAuthStatus()
        if (authStatusRes.code !== 0 || authStatusRes.data?.authEnabled === undefined) {
          throw new Error(authStatusRes.msg || '检查认证状态失败')
        }
        authEnabled.value = authStatusRes.data.authEnabled

        if (authEnabled.value) {
          const savedToken = settingStore.getToken()
          isLoggedIn.value = Boolean(savedToken)
        } else {
          isLoggedIn.value = true
        }
      } catch (error: any) {
        console.error('Failed to check auth status:', error)
        if (error?.code === 401 || error?.response?.status === 401) {
          authEnabled.value = true
          isLoggedIn.value = false
        } else {
          const hasSavedToken = Boolean(settingStore.getToken())
          authEnabled.value = hasSavedToken
          isLoggedIn.value = hasSavedToken
        }
      } finally {
        checkingAuth.value = false
      }
    })()
    return initPromise
  }

  const waitForInit = async () => {
    await (initPromise ?? initAuth())
  }

  // 登录
  const login = async (loginUsername: string, password: string) => {
    loginLoading.value = true
    try {
      const res = await authApi.login(loginUsername, password)
      if (res.code !== 0) {
        return { success: false, message: res.msg || '登录失败' }
      }

      // 检查是否需要二次验证
      if (res.data?.needTwoFA) {
        settingStore.setToken('')
        twoFARequired.value = true
        twoFASetupRequired.value = !res.data.isSetup
        twoFAAccountSetup.value = Boolean(res.data.isAccountSetup)
        tempToken.value = res.data.tempToken || ''
        twoFAMethod.value = res.data.method || ''
        // 保存临时 token 到 setting store
        settingStore.setTmpToken(res.data.tempToken || '')
        settingStore.setCurrentUsername(res.data.username || loginUsername)

        return {
          success: true,
          needTwoFA: true,
          isSetup: res.data.isSetup,
          isAccountSetup: res.data.isAccountSetup,
          method: res.data.method,
        }
      }

      // 不需要二次验证，直接登录
      if (res.data?.token) {
        settingStore.setTmpToken('')
        settingStore.setToken(res.data.token)
        settingStore.setCurrentUsername(res.data.username || loginUsername)
        isLoggedIn.value = true
        twoFARequired.value = false
        twoFASetupRequired.value = false
        twoFAAccountSetup.value = false
        void settingStore.fetchSystemInfo(true)
        return { success: true }
      } else {
        return { success: false, message: res.msg || '登录失败' }
      }
    } catch (error: any) {
      console.error('Login failed:', error)
      const message = error.response?.data?.msg || error.msg || error.message || '登录失败'
      return { success: false, message }
    } finally {
      loginLoading.value = false
    }
  }

  // 登出
  const logout = async () => {
    try {
      if (isLoggedIn.value) {
        await authApi.logout()
      }
    } catch (error) {
      console.error('Logout API failed:', error)
    } finally {
      console.debug('logout')
      // 无论API调用是否成功，都清除本地状态
      settingStore.clearToken()
      settingStore.clearCurrentUsername()
      isLoggedIn.value = false
      twoFARequired.value = false
      twoFASetupRequired.value = false
      twoFAAccountSetup.value = false
      tempToken.value = ''
      twoFAMethod.value = ''
      navigateTo('/login')
    }
  }

  // 强制登出（用于token过期等情况）
  const forceLogout = () => {
    console.debug('forceLogout')
    settingStore.clearToken()
    settingStore.clearCurrentUsername()
    isLoggedIn.value = false
    twoFARequired.value = false
    twoFASetupRequired.value = false
    twoFAAccountSetup.value = false
    tempToken.value = ''
    twoFAMethod.value = ''
    if (authEnabled.value) {
      navigateTo('/login')
    }
  }

  // 完成二次验证后设置完整 token，并清空临时 token
  const completeTwoFA = (fullToken: string) => {
    settingStore.setTmpToken('')
    settingStore.setToken(fullToken)
    isLoggedIn.value = true
    twoFARequired.value = false
    twoFASetupRequired.value = false
    twoFAAccountSetup.value = false
    tempToken.value = ''
    twoFAMethod.value = ''
    void settingStore.fetchSystemInfo(true)
  }

  // 取消二次验证流程，返回用户名密码登录界面
  const cancelTwoFA = () => {
    twoFARequired.value = false
    twoFASetupRequired.value = false
    twoFAAccountSetup.value = false
    tempToken.value = ''
    twoFAMethod.value = ''
    settingStore.setTmpToken('')
  }

  return {
    // 状态
    isLoggedIn,
    username,
    authEnabled,
    loginLoading,
    checkingAuth,
    twoFARequired,
    twoFASetupRequired,
    twoFAAccountSetup,
    tempToken,
    twoFAMethod,

    // 计算属性
    requiresAuth,

    // 方法
    initAuth,
    waitForInit,
    setAuthStatus,
    login,
    logout,
    forceLogout,
    completeTwoFA,
    cancelTwoFA,
  }
})

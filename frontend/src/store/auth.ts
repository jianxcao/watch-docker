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
  const tempToken = ref('')
  const twoFAMethod = ref('')

  // 登录加载状态
  const loginLoading = ref(false)

  // 检查认证状态加载中
  const checkingAuth = ref(false)

  // 计算属性：是否需要登录
  const requiresAuth = computed(() => {
    return authEnabled.value && !isLoggedIn.value
  })

  // 计算属性：当前用户名
  const username = computed(() => {
    return settingStore.currentUsername
  })

  // 初始化：从setting store恢复token
  const initAuth = async () => {
    checkingAuth.value = true
    try {
      // 检查是否启用身份验证
      const authStatusRes = await authApi.checkAuthStatus()
      authEnabled.value = authStatusRes.data.authEnabled

      // 如果启用了身份验证，检查当前token状态
      if (authEnabled.value) {
        const savedToken = settingStore.getToken()
        if (savedToken) {
          isLoggedIn.value = true
          // 这里可以添加验证token有效性的逻辑
        }
      } else {
        // 如果未启用身份验证，自动设为已登录状态
        isLoggedIn.value = true
      }
    } catch (error) {
      console.error('Failed to check auth status:', error)
      // 如果无法检查状态，默认不启用认证
      authEnabled.value = false
      isLoggedIn.value = true
    } finally {
      checkingAuth.value = false
    }
  }

  // 登录
  const login = async (loginUsername: string, password: string) => {
    loginLoading.value = true
    try {
      const res = await authApi.login(loginUsername, password)

      // 检查是否需要二次验证
      if (res.data?.needTwoFA) {
        twoFARequired.value = true
        twoFASetupRequired.value = !res.data.isSetup
        tempToken.value = res.data.tempToken || ''
        twoFAMethod.value = res.data.method || ''
        // 保存临时 token 到 setting store
        settingStore.setTmpToken(res.data.tempToken || '')
        settingStore.setCurrentUsername(res.data.username || loginUsername)

        return {
          success: true,
          needTwoFA: true,
          isSetup: res.data.isSetup,
          method: res.data.method,
        }
      }

      // 不需要二次验证，直接登录
      if (res.data?.token) {
        settingStore.setToken(res.data.token)
        settingStore.setCurrentUsername(res.data.username || loginUsername)
        isLoggedIn.value = true
        twoFARequired.value = false
        return { success: true }
      } else {
        return { success: false, message: '登录失败' }
      }
    } catch (error: any) {
      console.error('Login failed:', error)
      const message = error.response?.data?.msg || error.message || '登录失败'
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
    tempToken.value = ''
    twoFAMethod.value = ''
    if (authEnabled.value) {
      navigateTo('/login')
    }
  }

  // 完成二次验证后设置完整 token
  const completeTwoFA = (fullToken: string) => {
    settingStore.setToken(fullToken)
    isLoggedIn.value = true
    twoFARequired.value = false
    twoFASetupRequired.value = false
    tempToken.value = ''
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
    tempToken,
    twoFAMethod,

    // 计算属性
    requiresAuth,

    // 方法
    initAuth,
    login,
    logout,
    forceLogout,
    completeTwoFA,
  }
})

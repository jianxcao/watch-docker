import { createApp } from 'vue'
import 'virtual:uno.css'
import './style.css'
import 'dayjs/locale/zh-cn' // import locale
import App from './App.vue'
import { createPinia } from 'pinia'
import router from '@/router'
import { useAuthStore } from '@/store/auth'
import { useSettingStore } from '@/store/setting'
import { initTokenStore } from '@/common/axiosConfig'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

// 初始化 token store
const settingStore = useSettingStore()
initTokenStore(() => settingStore.getToken())

// 初始化身份验证状态
const authStore = useAuthStore()
authStore.initAuth()

app.mount('#app')

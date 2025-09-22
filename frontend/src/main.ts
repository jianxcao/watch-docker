import { createApp } from 'vue'
import 'virtual:uno.css'
import 'dayjs/locale/zh-cn' // import locale
import App from './App.vue'
import { createPinia } from 'pinia'
import router from '@/router'

const app = createApp(App)
app.use(createPinia())
app.use(router)

app.mount('#app')

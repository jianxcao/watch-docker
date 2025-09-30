import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/pages/LoginView.vue'),
      meta: {
        title: '登录',
        requiresAuth: false,
        hideInMenu: true,
      },
    },
    {
      path: '/',
      component: () => import('@/components/LayoutView.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          redirect: '/home',
        },
        {
          path: '/home',
          component: () => import('@/pages/HomeView.vue'),
          meta: { title: '首页', requiresAuth: true },
        },
        {
          path: '/containers',
          component: () => import('@/pages/ContainersView.vue'),
          meta: { title: '容器管理', requiresAuth: true },
        },
        {
          path: '/images',
          component: () => import('@/pages/ImagesView.vue'),
          meta: { title: '镜像管理', requiresAuth: true },
        },
        {
          path: '/settings',
          component: () => import('@/pages/SettingsView.vue'),
          meta: { title: '系统设置', requiresAuth: true },
        },
        {
          path: '/logs',
          component: () => import('@/pages/LogsPageView.vue'),
          meta: { title: '日志', requiresAuth: true },
        },
      ],
    },

    {
      // 404 页面
      path: '/:pathMatch(.*)*',
      redirect: '/home',
    },
  ],
})

// 路由守卫
router.beforeEach(async (to, _from, next) => {
  const authStore = useAuthStore()

  // 如果还在检查认证状态，等待完成
  if (authStore.checkingAuth) {
    // 等待认证检查完成
    while (authStore.checkingAuth) {
      await new Promise((resolve) => setTimeout(resolve, 50))
    }
  }

  // 如果是登录页面
  if (to.path === '/login') {
    // 如果已经登录且不需要认证，跳转到首页
    if (authStore.isLoggedIn || !authStore.authEnabled) {
      next('/')
      return
    }
    // 否则正常进入登录页
    next()
    return
  }

  // 如果启用了认证且未登录，跳转到登录页
  if (authStore.authEnabled && !authStore.isLoggedIn) {
    if (to.path !== '/login') {
      next('/login')
      return
    }
  }

  // 其他情况正常跳转
  next()
})

export default router

export function navigateTo(path: string) {
  router.push(path)
}

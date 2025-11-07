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
          meta: { title: '首页', requiresAuth: true, layoutClass: 'layout-home' },
        },
        {
          path: '/containers',
          component: () => import('@/pages/ContainersView.vue'),
          meta: { title: '容器管理', requiresAuth: true, layoutClass: 'layout-containers' },
        },
        {
          path: '/containers/create',
          name: 'container-create',
          component: () => import('@/pages/CreateContainer/ContainerCreateView.vue'),
          meta: {
            title: '创建容器',
            requiresAuth: true,
            layoutClass: 'layout-container-create',
          },
        },
        {
          path: '/containers/:id',
          name: 'container-detail',
          component: () => import('@/pages/ContainerDetail/ContainerDetailView.vue'),
          meta: {
            title: '容器详情',
            requiresAuth: true,
            layoutClass: 'layout-container-detail',
          },
        },
        {
          path: '/images',
          component: () => import('@/pages/ImagesView.vue'),
          meta: { title: '镜像管理', requiresAuth: true, layoutClass: 'layout-images' },
        },
        {
          path: '/compose',
          name: 'compose',
          component: () => import('@/pages/ComposeView.vue'),
          meta: { title: 'Compose 项目', requiresAuth: true },
        },
        {
          path: '/compose/create',
          name: 'compose-create',
          component: () => import('@/pages/ComposeCreateView.vue'),
          meta: {
            title: '创建 Compose 项目',
            requiresAuth: true,
            layoutClass: 'layout-compose-create',
          },
        },
        {
          path: '/compose/:projectName/detail',
          name: 'compose-detail',
          component: () => import('@/pages/ComposeDetailView.vue'),
          meta: {
            title: 'Compose 项目详情',
            requiresAuth: true,
            layoutClass: 'layout-compose-detail',
          },
        },
        {
          path: '/volumes',
          name: 'volumes',
          component: () => import('@/pages/VolumesView.vue'),
          meta: { title: 'Volume 管理', requiresAuth: true, layoutClass: 'layout-volumes' },
        },
        {
          path: '/volumes/:name',
          name: 'volume-detail',
          component: () => import('@/pages/VolumeDetailView.vue'),
          meta: {
            title: 'Volume 详情',
            requiresAuth: true,
            layoutClass: 'layout-volume-detail',
          },
        },
        {
          path: '/networks',
          name: 'networks',
          component: () => import('@/pages/NetworksView.vue'),
          meta: { title: '网络管理', requiresAuth: true, layoutClass: 'layout-networks' },
        },
        {
          path: '/networks/:id',
          name: 'network-detail',
          component: () => import('@/pages/NetworkDetailView.vue'),
          meta: {
            title: '网络详情',
            requiresAuth: true,
            layoutClass: 'layout-network-detail',
          },
        },
        {
          path: '/settings',
          component: () => import('@/pages/SettingsView.vue'),
          meta: { title: '系统设置', requiresAuth: true, layoutClass: 'layout-settings' },
        },
        {
          path: '/logs',
          component: () => import('@/pages/LogsPageView.vue'),
          meta: { title: '日志', requiresAuth: true, layoutClass: 'layout-logs' },
        },
        {
          path: '/terminal',
          component: () => import('@/pages/TerminalView.vue'),
          meta: { title: '终端', requiresAuth: true, layoutClass: 'layout-terminal' },
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

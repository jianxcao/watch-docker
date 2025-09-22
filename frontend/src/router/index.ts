import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: () => import('@/components/Layout.vue'),
      children: [
        {
          path: '',
          redirect: '/home',
        },
        {
          path: '/home',
          component: () => import('@/pages/Home.vue'),
          meta: { title: '首页' },
        },
        {
          path: '/containers',
          component: () => import('@/pages/Containers.vue'),
          meta: { title: '容器管理' },
        },
        {
          path: '/images',
          component: () => import('@/pages/Images.vue'),
          meta: { title: '镜像管理' },
        },
        {
          path: '/settings',
          component: () => import('@/pages/Settings.vue'),
          meta: { title: '系统设置' },
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

export default router

export function navigateTo(path: string) {
  router.push(path)
}

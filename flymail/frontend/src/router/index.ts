import { createRouter, createWebHashHistory } from 'vue-router'
import type { RouteRecordRaw, NavigationGuardNext, RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { authService } from '@/api'

// 声明路由元信息类型
declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    title?: string
  }
}

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: {
      requiresAuth: false,
      title: '登录'
    }
  },
  {
    path: '/main',
    name: 'main',
    component: () => import('@/views/MailView.vue'),
    meta: {
      requiresAuth: true,
      title: '邮箱'
    }
  },
  {
    path: '/view',
    name: 'mailView',
    component: () => import('@/views/MailViewPage.vue'),
    meta: {
      requiresAuth: true,
      title: '邮件详情'
    }
  },
  {
    path: '/compose',
    name: 'compose',
    component: () => import('@/views/ComposeView.vue'),
    meta: {
      requiresAuth: true,
      title: '撰写邮件'
    }
  },
  // 根路径重定向到主页面
  {
    path: '/',
    redirect: '/main?a=0&f=all-inbox'
  },
  // 404 页面
  {
    path: '/:pathMatch(.*)*',
    name: 'notFound',
    redirect: '/main?a=0&f=all-inbox'
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

// Navigation guard
router.beforeEach(async (to: RouteLocationNormalized, _from: RouteLocationNormalized, next: NavigationGuardNext) => {
  console.log('🚏 [Router] 导航到:', to.path, to.query)

  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth !== false)

  if (requiresAuth) {
    // Check if we have a token
    const token = authService.getAccessToken()

    if (!token) {
      // No token, redirect to login
      console.log('🔐 [Router] 无令牌，重定向到登录页')
      next({
        path: '/login',
        query: { returnUrl: to.fullPath }
      })
      return
    }

    // Have token but no user info, try to fetch it
    if (!authStore.isAuthenticated) {
      try {
        console.log('👤 [Router] 获取用户信息')
        await authService.getCurrentUser()
        next()
      } catch (error) {
        // Token is invalid
        console.log('❌ [Router] 令牌无效，清除认证信息')
        authStore.clearAuth()
        next({
          path: '/login',
          query: { returnUrl: to.fullPath }
        })
      }
    } else {
      next()
    }
  } else {
    // Page doesn't require auth
    next()
  }
})

// 路由后置钩子，用于设置页面标题
router.afterEach((to) => {
  const title = to.meta.title
  if (title) {
    document.title = `${title} - FlyMail+`
  }
})

export default router
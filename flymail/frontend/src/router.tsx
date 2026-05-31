import type { ReactNode } from 'react'
import { createBrowserRouter, Navigate } from 'react-router'
import { auth } from '@/lib/auth'
import { LoginPage } from '@/pages/Login'
import { ShellPage } from '@/pages/Shell'

// 路由守卫：未登录则重定向到 /login。
function RequireAuth({ children }: { children: ReactNode }) {
  if (!auth.isAuthenticated()) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

export const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: (
      <RequireAuth>
        <ShellPage />
      </RequireAuth>
    ),
  },
  {
    path: '*',
    element: <Navigate to="/" replace />,
  },
])

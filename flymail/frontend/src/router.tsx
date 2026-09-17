import type { ReactNode } from 'react'
import { createBrowserRouter, Navigate, useLocation } from 'react-router'
import { auth } from '@/lib/auth'
import { encodeNext } from '@/lib/next-path'
import { LoginPage } from '@/pages/Login'
import { ShellPage } from '@/pages/Shell'

// 路由守卫：未登录则重定向到 /login，并把来路带过去。
//
// ⚠ 带来路不是锦上添花。通知里的「打开邮件」链接多半在**另一台设备**上点开
// （飞书提醒 → 手机浏览器），那里没有登录态，这一跳是必经之路。不带来路的话
// URL 上的 account/folder/message 在这里就丢光了，登录完落在默认收件箱——
// 用户点了链接却没打开那封邮件，而且没有任何提示说明发生了什么。
function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation()
  if (!auth.isAuthenticated()) {
    const next = encodeNext(location.pathname, location.search)
    return <Navigate to={`/login?next=${next}`} replace />
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

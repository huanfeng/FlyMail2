import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { router } from './router'
import './index.css'
import '@/lib/i18n'
import { initTheme } from '@/lib/theme'
import { ToastProvider } from '@/components/ui/Toast'
import { ErrorBoundary } from '@/components/ui/ErrorBoundary'

initTheme()

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {/* 最外层：连 provider 自身的渲染异常也兜住。没有它，任何一个组件抛异常
        都会让 React 卸载整棵树，用户只剩一片白屏与 F5。 */}
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        {/* ToastProvider 包裹路由根，使所有子组件均可调用 useToast() */}
        <ToastProvider>
          <RouterProvider router={router} />
        </ToastProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  </StrictMode>,
)

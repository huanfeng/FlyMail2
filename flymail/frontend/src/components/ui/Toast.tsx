import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from 'react'

// ────────────────────────────────────────────────────────────────────────────
// Context 类型
// ────────────────────────────────────────────────────────────────────────────

interface ToastContextValue {
  /** 显示一条 toast 消息，2.5s 后自动消失 */
  toast: (message: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

// ────────────────────────────────────────────────────────────────────────────
// Provider
// ────────────────────────────────────────────────────────────────────────────

const TOAST_DURATION = 2500

export function ToastProvider({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState<string | null>(null)
  // 用 ref 持有定时器，避免多次触发时累积
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const toast = useCallback((msg: string) => {
    // 清除上一次定时器，实现单条替换
    if (timerRef.current != null) {
      clearTimeout(timerRef.current)
    }
    setMessage(msg)
    timerRef.current = setTimeout(() => {
      setMessage(null)
      timerRef.current = null
    }, TOAST_DURATION)
  }, [])

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      {/* 渲染 .toast 条；CSS 令牌与 toastIn 动画已在 index.css 就绪 */}
      {message != null && (
        <div className="toast" role="status" aria-live="polite">
          {message}
        </div>
      )}
    </ToastContext.Provider>
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hook
// ────────────────────────────────────────────────────────────────────────────

/** 在任意子组件中调用 toast(msg) 显示提示条 */
export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (ctx == null) {
    throw new Error('useToast 必须在 ToastProvider 内部使用')
  }
  return ctx
}

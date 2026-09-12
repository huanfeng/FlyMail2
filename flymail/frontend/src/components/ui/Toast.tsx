import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'

// ────────────────────────────────────────────────────────────────────────────
// Context 类型
// ────────────────────────────────────────────────────────────────────────────

export interface ToastOptions {
  /** 动作按钮文案（如「撤销」）。省略则只显示消息。 */
  actionLabel?: string
  /** 点击动作按钮时调用，调用后 toast 立即关闭。 */
  onAction?: () => void
  /** 停留时长（毫秒）。带动作的 toast 需要更长，默认见 TOAST_DURATION。 */
  duration?: number
  /**
   * toast 自然消失（未点动作）时调用。
   *
   * 延迟提交的操作用它来落地：toast 消失即撤销窗口关闭。
   */
  onExpire?: () => void
}

interface ToastContextValue {
  /** 显示一条 toast 消息，默认 2.5s 后自动消失 */
  toast: (message: string, options?: ToastOptions) => void
  /**
   * 立刻收起当前 toast，**不**触发它的 onExpire。
   *
   * 给延迟提交用：操作已经由别处强制落地时，撤销入口必须同时消失，
   * 否则按钮还在、点下去却已经无事可撤（见 useUndoable 的注释）。
   */
  dismiss: () => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

// ────────────────────────────────────────────────────────────────────────────
// Provider
// ────────────────────────────────────────────────────────────────────────────

const TOAST_DURATION = 2500

interface ToastState {
  message: string
  actionLabel?: string
  onAction?: () => void
  onExpire?: () => void
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ToastState | null>(null)
  // 用 ref 持有定时器，避免多次触发时累积
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // 被替换/卸载时要触发上一条的 onExpire，但那发生在 setState 之后，
  // 届时 state 已经是新的了，所以额外用 ref 记住「当前这条的收尾回调」。
  const expireRef = useRef<(() => void) | null>(null)

  /** 结束当前 toast。fire 为真时触发它的 onExpire（自然到期），否则只是清场。 */
  const dismiss = useCallback((fire: boolean) => {
    if (timerRef.current != null) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    const expire = expireRef.current
    expireRef.current = null
    setState(null)
    if (fire) expire?.()
  }, [])

  /** 对外暴露的收起：只清场，挂起的操作由调用方自己负责落地。 */
  const dismissOnly = useCallback(() => dismiss(false), [dismiss])

  const toast = useCallback(
    (msg: string, options?: ToastOptions) => {
      // 单条替换：新 toast 到来时，上一条的挂起操作必须立刻落地，
      // 否则它的撤销入口消失了，操作却还悬在半空。
      dismiss(true)

      expireRef.current = options?.onExpire ?? null
      setState({
        message: msg,
        actionLabel: options?.actionLabel,
        onAction: options?.onAction,
        onExpire: options?.onExpire,
      })
      timerRef.current = setTimeout(() => {
        timerRef.current = null
        dismiss(true)
      }, options?.duration ?? TOAST_DURATION)
    },
    [dismiss],
  )

  // 卸载时把挂起的操作落地，不能让它随组件一起消失。
  useEffect(() => {
    return () => {
      if (timerRef.current != null) clearTimeout(timerRef.current)
      expireRef.current?.()
      expireRef.current = null
    }
  }, [])

  function handleAction() {
    const act = state?.onAction
    // 撤销掉的操作不该再落地，所以这里不触发 onExpire。
    expireRef.current = null
    dismiss(false)
    act?.()
  }

  return (
    <ToastContext.Provider value={{ toast, dismiss: dismissOnly }}>
      {children}

      {/*
        播报区常驻。
        aria-live 的语义是「这个区域的内容变化时播报」——区域本身和内容一起插入 DOM 时，
        多数读屏不播报。撤销条是删除/归档/移动的唯一反馈（5 秒后静默提交），
        漏播等于读屏用户做完危险操作收不到任何确认，也不知道有撤销可按。
        所以这里始终渲染容器，只让里面的文本变化。
      */}
      <div className="sr-only" role="status" aria-live="polite">
        {state?.message ?? ''}
      </div>

      {/* 渲染 .toast 条；CSS 令牌与 toastIn 动画已在 index.css 就绪 */}
      {state != null && (
        <div className="toast">
          <span>{state.message}</span>
          {state.actionLabel && (
            <button type="button" className="toast-action" onClick={handleAction}>
              {state.actionLabel}
            </button>
          )}
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

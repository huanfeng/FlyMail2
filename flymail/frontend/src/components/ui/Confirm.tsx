import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'

// ────────────────────────────────────────────────────────────────────────────
// 为什么要这个组件
// ────────────────────────────────────────────────────────────────────────────
//
// 原先删账户 / 别名 / 黑名单 / 通知渠道 / 规则 / 信任发件人 / 会话内单封，
// 八处全是 `window.confirm`。原生 confirm 有三个问题：
//
// 1. 在 Wails 桌面壳里它是系统弹窗，外观完全脱离应用，也不跟随主题；
// 2. 它**阻塞整个 JS 线程**——弹着的时候定时器、SSE 事件、React 更新全停；
// 3. 文案只能是一整段纯文本，没法把「这个操作会怎样」和「删的是哪一个」分开。
//
// API 刻意做成返回 Promise 的函数，而不是「传 onConfirm 回调」：
// 调用点因此只需加一个 `await`，控制流的形状原样保留——
//
//     if (!(await confirm({ ... }))) return
//
// 与 `if (!window.confirm(...)) return` 逐字对应，八处的改动都是机械的。

export interface ConfirmOptions {
  /** 标题。一句话说清要做什么 */
  title: string
  /** 补充说明。可省略 */
  body?: string
  /** 确认按钮文案。默认「删除」那一档由调用方给 */
  confirmLabel?: string
  /** 取消按钮文案。默认取 common.cancel */
  cancelLabel?: string
  /** 危险操作（不可逆）：确认键用 danger 语义色 */
  danger?: boolean
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>

const ConfirmContext = createContext<ConfirmFn | null>(null)

/**
 * 取确认函数。返回 true = 用户确认，false = 取消 / 关闭 / Esc。
 *
 * 没有 Provider 时**不**回退到 window.confirm：那会让「忘了挂 Provider」
 * 这件事在界面上完全看不出来（弹的还是个确认框，只是长得不一样），
 * 而这正是这次要消灭的东西。直接抛，在开发期就暴露。
 */
export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext)
  if (fn == null) throw new Error('useConfirm 必须在 ConfirmProvider 内使用')
  return fn
}

interface PendingConfirm extends ConfirmOptions {
  resolve: (ok: boolean) => void
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<PendingConfirm | null>(null)
  // 关闭动画期间 pending 已清空，但 resolve 还没调用过时要兜住
  const resolveRef = useRef<((ok: boolean) => void) | null>(null)

  const confirm = useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      // 前一个还开着就直接当作取消——两个确认框叠在一起时用户分不清在答哪一个，
      // 而未兑现的 Promise 会让调用方永远卡在 await 上。
      resolveRef.current?.(false)
      resolveRef.current = resolve
      setPending({ ...opts, resolve })
    })
  }, [])

  const settle = useCallback((ok: boolean) => {
    resolveRef.current?.(ok)
    resolveRef.current = null
    setPending(null)
  }, [])

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog.Root
        open={pending != null}
        onOpenChange={(o) => {
          // Esc、点遮罩、点关闭都走这里，一律算取消
          if (!o) settle(false)
        }}
      >
        <Dialog.Portal>
          <Dialog.Overlay className="confirm-backdrop" />
          <Dialog.Content className="confirm-dialog" aria-describedby={undefined}>
            <Dialog.Title className="confirm-title">{pending?.title ?? ''}</Dialog.Title>
            {pending?.body != null && <p className="confirm-body">{pending.body}</p>}
            <div className="confirm-actions">
              {/* 取消排在前面：确认框多半用于不可逆操作，默认落点应当是
                  「什么都不做」——连按两下回车不该删掉东西。
                  autoFocus 在今天是冗余的（radix 本来就聚焦内容里第一个可聚焦元素，
                  实测去掉它测试照样绿），留着是因为这个属性表达的是意图，
                  而不是对第三方默认行为的依赖。 */}
              <button
                type="button"
                className="pill-btn"
                onClick={() => settle(false)}
                autoFocus
              >
                {pending?.cancelLabel ?? t('common.cancel')}
              </button>
              <button
                type="button"
                className={'pill-btn ' + (pending?.danger ? 'danger' : 'primary')}
                onClick={() => settle(true)}
              >
                {pending?.confirmLabel ?? t('common.confirm')}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </ConfirmContext.Provider>
  )
}

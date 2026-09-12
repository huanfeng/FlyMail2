import { Component } from 'react'
import type { ErrorInfo, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { errorText } from '@/lib/format'

/**
 * 渲染期异常的兜底。
 *
 * 没有它的话，任何一个组件抛出异常都会让 React 卸载整棵树——浏览器里只剩一片
 * 白屏，用户既看不到发生了什么，也没有除了自己按 F5 之外的出路，
 * 而"白屏"这个表现与"网络断了""服务挂了"完全无法区分。
 *
 * 只兜渲染期异常：事件处理器与 async 里的异常 React 不会传到这里，
 * 那些路径各自用 toast / 错误态表达（见 MailList 的 error 分支）。
 */

interface Props {
  children: ReactNode
}

interface State {
  /** 独立于 error 的开关：`throw null` / `throw undefined` 也是合法的抛出，
   *  拿 `error != null` 当判据时那种情况会漏掉，回退界面不出现而子树继续抛。 */
  hasError: boolean
  error: unknown
}

/** 崩溃回退界面。拆成函数组件是为了能用 useTranslation——class 组件里没有 hook。 */
function CrashScreen({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation()
  const detail = errorText(error)
  return (
    <div
      className="list-error"
      role="alert"
      style={{ display: 'grid', placeContent: 'center', minHeight: '100vh' }}
    >
      <div className="list-error-title">{t('app.crashTitle')}</div>
      <div>{t('app.crashHint')}</div>
      {detail && <div className="list-error-detail">{detail}</div>}
      <div style={{ display: 'flex', gap: 8, justifyContent: 'center', marginTop: 14 }}>
        {/* 重试 = 清掉错误重新挂载。瞬时故障（比如一次取数返回了意外形状）
            靠它就能恢复，不必丢掉整个页面状态。 */}
        <button type="button" className="pill-btn" onClick={onRetry}>
          {t('app.retry')}
        </button>
        <button type="button" className="pill-btn primary" onClick={() => window.location.reload()}>
          {t('app.crashReload')}
        </button>
      </div>
    </div>
  )
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: unknown): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // 组件栈只在这里拿得到，控制台是排查这类问题唯一的线索来源
    console.error('[ErrorBoundary]', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <CrashScreen
          error={this.state.error}
          onRetry={() => this.setState({ hasError: false, error: null })}
        />
      )
    }
    return this.props.children
  }
}

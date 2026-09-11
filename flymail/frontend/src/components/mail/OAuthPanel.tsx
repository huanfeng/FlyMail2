import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/Icon'
import { openExternal } from '@/lib/platform'
import {
  useCancelOAuthFlow,
  useCompleteOAuth,
  useOAuthFlowStatus,
  useStartOAuth,
} from '@/lib/queries'
import type { Account, OAuthMode, OAuthProviderInfo, OAuthStartResponse } from '@/lib/types'

export interface OAuthPanelProps {
  provider: OAuthProviderInfo
  /** 授权方式；设备码用于浏览器回调走不通的场景 */
  mode?: OAuthMode
  /** 非空表示重新授权既有账户，而不是新建 */
  accountId?: number
  /** 作为 login_hint，帮用户在账号选择页定位 */
  hintEmail?: string
  /** 新建时的显示名，留空由后端取邮箱本地部分 */
  name?: string
  onDone: (account: Account) => void
  onCancel: () => void
}

/**
 * 一次 OAuth 授权的引导面板。
 *
 * 三段式：发起 → 用户在浏览器/另一设备上完成 → 轮询到结果后落库。
 * 中间那段发生在 FlyMail 之外，前端唯一能做的就是轮询，因此这里的重点是把
 * 「现在该做什么」始终显式摆在用户面前，避免出现无反馈的等待。
 */
export function OAuthPanel({
  provider,
  mode,
  accountId,
  hintEmail,
  name,
  onDone,
  onCancel,
}: OAuthPanelProps) {
  const { t } = useTranslation()
  const [flow, setFlow] = React.useState<OAuthStartResponse | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  const startOAuth = useStartOAuth()
  const completeOAuth = useCompleteOAuth()
  const cancelFlow = useCancelOAuthFlow()
  const { data: status } = useOAuthFlowStatus(flow?.flow_id ?? null)

  // 面板挂载即发起授权：用户点的是「用 Google 登录」，不该再多一次确认。
  // StrictMode 下 effect 会跑两次，用 ref 兜住，避免起两条流程白占一个回调端口。
  const started = React.useRef(false)
  React.useEffect(() => {
    if (started.current) return
    started.current = true
    startOAuth.mutate(
      { provider: provider.id, mode, email: hintEmail, account_id: accountId },
      {
        onSuccess: (res) => setFlow(res),
        onError: (e) => setError(errorMessage(e, t('account.oauthStartFailed'))),
      },
    )
    // 依赖刻意留空：这是一次性的挂载副作用，参数在面板生命周期内不变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 授权完成后落库。completed 保证只提交一次——轮询可能在 mutation 返回前再触发一轮。
  const completed = React.useRef(false)
  React.useEffect(() => {
    if (status?.status !== 'success' || !flow || completed.current) return
    completed.current = true
    completeOAuth.mutate(
      { flow_id: flow.flow_id, name, email: status.email },
      {
        onSuccess: onDone,
        onError: (e) => setError(errorMessage(e, t('account.oauthCompleteFailed'))),
      },
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status?.status, flow])

  // 用户中途关闭面板时释放后端占用的回调端口，不必等它自己超时。
  function handleCancel() {
    if (flow && status?.status === 'pending') {
      cancelFlow.mutate(flow.flow_id)
    }
    onCancel()
  }

  // loopback 回调打的是「用户这台机器」的 127.0.0.1。若 FlyMail 本身是远程访问的，
  // 那个端口在用户机上并不存在，授权完会跳到一个打不开的页面、流程静默停在 pending。
  // 这是部署方漏配 oauth.redirect_base_url 的典型症状，直接点破比让人对着转圈排查强。
  const remoteLoopback =
    flow?.loopback === true &&
    typeof window !== 'undefined' &&
    !['localhost', '127.0.0.1', '[::1]'].includes(window.location.hostname)

  const failed = status?.status === 'failed'
  const shownError = error ?? (failed ? (status?.error ?? t('account.oauthFailed')) : null)
  const busy = startOAuth.isPending || completeOAuth.isPending

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2 text-sm font-medium" style={{ color: 'var(--ink)' }}>
        <Icon name="shield" size={16} />
        <span>{t('account.oauthTitle', { provider: provider.name })}</span>
      </div>

      {shownError && (
        <div
          className="rounded-md px-3 py-2 text-sm"
          style={{ background: 'var(--danger-wash, #fdecec)', color: 'var(--danger-ink, #a12622)' }}
          role="alert"
        >
          {shownError}
        </div>
      )}

      {busy && !shownError && (
        <div className="text-sm" style={{ color: 'var(--ink-2)' }}>
          {completeOAuth.isPending ? t('account.oauthSaving') : t('account.oauthStarting')}
        </div>
      )}

      {remoteLoopback && (
        <div
          className="rounded-md px-3 py-2 text-sm"
          style={{ background: 'var(--accent-wash)', color: 'var(--accent-ink)' }}
        >
          {t('account.oauthLoopbackWarning')}
        </div>
      )}

      {/* 授权码流程：把用户送到服务商的授权页 */}
      {flow?.auth_url && !shownError && (
        <div className="flex flex-col gap-3">
          <p className="text-sm" style={{ color: 'var(--ink-2)', margin: 0 }}>
            {t('account.oauthCodeHint')}
          </p>
          <Button type="button" onClick={() => openExternal(flow.auth_url!)}>
            {t('account.oauthOpen')}
          </Button>
          {/* 链接兜底：桌面端的系统浏览器调用可能被拦，或用户想换个浏览器登录 */}
          <details>
            <summary className="cursor-pointer text-xs" style={{ color: 'var(--ink-3)' }}>
              {t('account.oauthCopyHint')}
            </summary>
            <textarea
              readOnly
              value={flow.auth_url}
              onFocus={(e) => e.currentTarget.select()}
              className="mt-2 w-full rounded-md border border-input bg-transparent p-2 text-xs"
              rows={3}
              style={{ color: 'var(--ink-3)' }}
            />
          </details>
        </div>
      )}

      {/* 设备码流程：用户在另一台设备上输码 */}
      {flow?.user_code && !shownError && (
        <div className="flex flex-col gap-3">
          <p className="text-sm" style={{ color: 'var(--ink-2)', margin: 0 }}>
            {t('account.oauthDeviceHint')}
          </p>
          <div
            className="select-all rounded-md px-3 py-3 text-center text-lg font-semibold tracking-widest"
            style={{ background: 'var(--bg-alt)', color: 'var(--ink)' }}
          >
            {flow.user_code}
          </div>
          {flow.verification_uri && (
            <Button type="button" variant="outline" onClick={() => openExternal(flow.verification_uri!)}>
              {flow.verification_uri}
            </Button>
          )}
        </div>
      )}

      {status?.status === 'pending' && (
        <div className="text-sm" style={{ color: 'var(--ink-3)' }}>
          {t('account.oauthWaiting')}
        </div>
      )}

      <div className="flex justify-end">
        <Button type="button" variant="ghost" onClick={handleCancel}>
          {t('account.cancel')}
        </Button>
      </div>
    </div>
  )
}

/** 从 axios 错误里取出后端给的中文原因，取不到时回退到通用文案。 */
function errorMessage(e: unknown, fallback: string): string {
  const detail = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
  return detail || fallback
}

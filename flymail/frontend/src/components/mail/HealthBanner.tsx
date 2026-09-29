import { useState, useEffect, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useAccounts } from '@/lib/queries'
import { healthKind, loadDismissed, saveDismissed, pruneDismissed } from '@/lib/account-health'
import { requestOpenSettings } from '@/lib/settings-nav'

export function HealthBanner() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const [dismissed, setDismissed] = useState<Set<number>>(() => loadDismissed())
  const faulty = accounts.filter((a) => healthKind(a) !== null)
  const faultyIds = useMemo(() => new Set(faulty.map((a) => a.id)), [accounts])
  const visible = faulty.filter((a) => !dismissed.has(a.id))

  // 恢复即清记忆：faultyIds 缩小后把已恢复账户从 dismissed 移除，
  // 同会话「恢复→再故障」才会重现横幅。没有故障时 faultyIds 为空集，
  // prune 把记忆全清——这正是期望行为。
  useEffect(() => {
    if (dismissed.size === 0) return
    const next = pruneDismissed(dismissed, faultyIds)
    if (next.size === dismissed.size) return
    setDismissed(next)
    saveDismissed(next)
  }, [dismissed, faultyIds])

  // dismissAll 必须在 early return 之前定义（Hook 数量不能随渲染分支变化：
  // 点击关闭后 visible 变空，若定义在其后 React 会因 Hook 数量变化崩溃）。
  const dismissAll = useCallback(() => {
    const next = new Set(dismissed)
    visible.forEach((a) => next.add(a.id))
    setDismissed(next)
    saveDismissed(next)
  }, [dismissed, visible])

  if (visible.length === 0) return null
  const named = visible.slice(0, 2)
  const more = visible.length - named.length

  return (
    <div className="health-banner" role="alert">
      <span className="health-banner-text">
        {t('account.healthBanner.title')}
        {named.map((a) => a.name).join('、')}
        {more > 0 && t('account.healthBanner.more').replace('{n}', String(more))}
      </span>
      <div className="health-banner-actions">
        <button
          type="button"
          className="health-banner-btn"
          onClick={() => requestOpenSettings('monitoring')}
        >
          {t('account.healthBanner.openSettings')}
        </button>
        <button
          type="button"
          className="health-banner-btn"
          onClick={dismissAll}
        >
          {t('account.healthBanner.dismiss')}
        </button>
      </div>
    </div>
  )
}

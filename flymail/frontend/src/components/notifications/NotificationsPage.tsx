// 通知中心（站内 feed）— 渲染进第三栏 reader 区
// 数据来自后端 /notifications；支持按类型/未读筛选、按日分组、单条/全部已读、清空、加载更多。

import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import type { IconName } from '@/components/ui/Icon'
import {
  useNotifications,
  useMarkNotificationRead,
  useMarkAllNotificationsRead,
  useClearNotifications,
} from '@/lib/queries'
import type { Notification } from '@/lib/types'

interface NotificationsPageProps {
  onBack: () => void
}

type Tab = 'all' | 'unread' | 'mail_new' | 'sync_failed' | 'account_status'

// 事件类型 → 图标 + kind 配色类
const TYPE_META: Record<string, { icon: IconName; kind: string }> = {
  mail_new: { icon: 'inbox', kind: 'kind-mail' },
  sync_failed: { icon: 'circle-dot', kind: 'kind-cal' },
  account_status: { icon: 'tag', kind: 'kind-acct' },
}

export function NotificationsPage({ onBack }: NotificationsPageProps) {
  const { t, i18n } = useTranslation()
  const isZh = i18n.language.startsWith('zh')
  const [tab, setTab] = useState<Tab>('all')

  const { data, hasNextPage, isFetchingNextPage, fetchNextPage, isLoading } = useNotifications()
  const markRead = useMarkNotificationRead()
  const markAll = useMarkAllNotificationsRead()
  const clearAll = useClearNotifications()

  const all = useMemo(() => data?.pages.flatMap((p) => p.notifications) ?? [], [data])
  const unreadCount = data?.pages[0]?.unread_count ?? 0

  const filtered = all.filter((n) => {
    if (tab === 'all') return true
    if (tab === 'unread') return !n.read
    return n.type === tab
  })

  // 相对时间
  function fmtTime(iso: string): string {
    const ms = new Date(iso).getTime()
    if (Number.isNaN(ms)) return ''
    const diff = Date.now() - ms
    if (diff < 60_000) return isZh ? '刚刚' : 'now'
    if (diff < 3_600_000) return Math.floor(diff / 60_000) + (isZh ? ' 分钟前' : 'm')
    if (diff < 86_400_000) return Math.floor(diff / 3_600_000) + (isZh ? ' 小时前' : 'h')
    return Math.floor(diff / 86_400_000) + (isZh ? ' 天前' : 'd')
  }

  // 按日分组
  function dayLabel(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime()
    if (diff < 86_400_000) return t('notif.today')
    if (diff < 2 * 86_400_000) return t('notif.yesterday')
    if (diff < 7 * 86_400_000) return t('notif.earlier')
    return t('notif.older')
  }
  const groups: { label: string; items: Notification[] }[] = []
  for (const n of filtered) {
    const label = dayLabel(n.created_at)
    let g = groups.find((x) => x.label === label)
    if (!g) { g = { label, items: [] }; groups.push(g) }
    g.items.push(n)
  }

  const tabs: { id: Tab; labelKey: string }[] = [
    { id: 'all', labelKey: 'notif.tabAll' },
    { id: 'unread', labelKey: 'notif.tabUnread' },
    { id: 'mail_new', labelKey: 'notif.tabMail' },
    { id: 'sync_failed', labelKey: 'notif.tabSync' },
    { id: 'account_status', labelKey: 'notif.tabAccount' },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: 'var(--bg)' }}>
      {/* 顶部操作栏 */}
      <div className="fp-tabs">
        <button type="button" className="fp-back" onClick={onBack}>
          <span style={{ transform: 'scaleX(-1)', display: 'inline-block' }}>
            <Icon name="chevron-right" size={14} />
          </span>
          {t('notif.backToInbox')}
        </button>
        <div className="spacer" style={{ flex: 1 }} />
        <button
          type="button"
          className="tb-btn"
          onClick={() => markAll.mutate()}
          disabled={unreadCount === 0 || markAll.isPending}
        >
          <Icon name="check" size={13} /> {t('notif.markAllRead')}
        </button>
        <button
          type="button"
          className="tb-btn"
          onClick={() => clearAll.mutate()}
          disabled={all.length === 0 || clearAll.isPending}
        >
          <Icon name="trash" size={13} /> {t('notif.clear')}
        </button>
      </div>

      <div className="fullpage">
        <div className="fp-head">
          <div style={{ flex: 1 }}>
            <div className="fp-title">{t('notif.title')}</div>
            <div className="fp-sub">
              {isZh
                ? `${unreadCount} 条未读 · 共 ${all.length} 条`
                : `${unreadCount} unread · ${all.length} total`}
            </div>
          </div>
        </div>

        {/* tab 行 */}
        <div className="notif-tabs">
          {tabs.map((x) => (
            <button
              key={x.id}
              type="button"
              className={'notif-tab' + (tab === x.id ? ' active' : '')}
              onClick={() => setTab(x.id)}
            >
              <span>{t(x.labelKey)}</span>
            </button>
          ))}
        </div>

        <div className="fp-body">
          {!isLoading && filtered.length === 0 && (
            <div className="notif-empty">
              <div style={{ fontFamily: 'var(--font-display)', fontSize: 22, color: 'var(--ink-2)', marginBottom: 6 }}>
                {t('notif.emptyTitle')}
              </div>
              <div style={{ fontSize: 14, color: 'var(--ink-3)' }}>{t('notif.emptyHint')}</div>
            </div>
          )}

          {groups.map((g) => (
            <div key={g.label}>
              <div className="notif-day-label">{g.label}</div>
              {g.items.map((n) => {
                const meta = TYPE_META[n.type] ?? { icon: 'bell' as IconName, kind: '' }
                return (
                  <div
                    key={n.id}
                    className={'notif-card' + (n.read ? '' : ' unread')}
                    onClick={() => { if (!n.read) markRead.mutate(n.id) }}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => { if ((e.key === 'Enter' || e.key === ' ') && !n.read) markRead.mutate(n.id) }}
                  >
                    <div className={'nf-icon ' + meta.kind}>
                      <Icon name={meta.icon} size={16} />
                    </div>
                    <div style={{ minWidth: 0 }}>
                      <div className="nf-title">{n.title}</div>
                      {n.body && <div className="nf-body">{n.body}</div>}
                    </div>
                    <div className="nf-time">{fmtTime(n.created_at)}</div>
                  </div>
                )
              })}
            </div>
          ))}

          {hasNextPage && (
            <div style={{ padding: '16px 0', textAlign: 'center' }}>
              <button
                type="button"
                className="pill-btn"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
              >
                {isFetchingNextPage ? t('list.loadingMore') : t('list.loadMore')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

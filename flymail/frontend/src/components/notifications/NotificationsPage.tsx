// 通知中心（站内 feed）— 居中浮层
import { useFocusTrap } from '@/hooks/useFocusTrap'
//
// 归属：与「设置」一致做成浮层，而不是占据第三栏。
// 侧栏那两个辅助入口（铃铛 / 齿轮）行为因此统一，主区域始终是邮件；
// 更重要的是消除了与阅读区抢第三栏导致的互斥——通知开着时点邮件列表，
// 曾经因为 Reader 根本没挂载而表现为「点击无反应」。
//
// 数据来自后端 /notifications；支持按类型/未读筛选、按日分组、单条/全部已读、清空、加载更多。

import { useEffect, useMemo, useState } from 'react'
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
import { modalLayerOpen } from '@/lib/overlay-layers'

interface NotificationsPageProps {
  /** 关闭浮层 */
  onClose: () => void
  /**
   * 点击通知卡片：跳转到关联邮件/账户（由 Shell 实现导航）。
   * 跳转后浮层自动关闭——用户的意图已经从「看通知」转成「看那封邮件」。
   */
  onOpen?: (n: Notification) => void
}

type Tab = 'all' | 'unread' | 'mail_new' | 'mail_rule' | 'sync_failed' | 'account_status'

// 事件类型 → 图标 + kind 配色类
const TYPE_META: Record<string, { icon: IconName; kind: string }> = {
  mail_new: { icon: 'inbox', kind: 'kind-mail' },
  sync_failed: { icon: 'circle-dot', kind: 'kind-cal' },
  account_status: { icon: 'tag', kind: 'kind-acct' },
  // 规则命中（M11）：与「新邮件」区分开，用漏斗图标
  mail_rule: { icon: 'filter', kind: 'kind-mail' },
}

export function NotificationsPage({ onClose, onOpen }: NotificationsPageProps) {
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
    const ms = new Date(iso).getTime()
    // ⚠ 这一句**不改变任何行为**，别把它当成修了一个 bug：NaN 让下面三个比较
    // 全为 false，本来就会落到最后那个 return，结果同样是「更早」。
    // 留着是因为「靠三次比较都失败来得到正确答案」是偶然对的——
    // 谁把顺序改成从大到小判，非法时间戳就会跳进「今天」。
    // （同文件的 fmtTime 与 MailList 的 relTime 守 NaN 则是真的有区别。）
    if (Number.isNaN(ms)) return t('notif.older')
    const diff = Date.now() - ms
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

  // 焦点关在浮层里（与设置浮层一致）
  const trapRef = useFocusTrap<HTMLDivElement>(true)

  // Esc 关闭（与设置浮层一致，含「上面开着 radix 浮层时让位」那条判断）
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      // 两个判据各管一头：defaultPrevented 认「那一层已经消费了这次按键」，
      // modalLayerOpen() 认「那一层还开着」。实测任一个单独都够用，
      // 留着两个是因为它们失效的方式不同（浮层不 preventDefault / 浮层不在 DOM 上留痕）。
      if (e.defaultPrevented || modalLayerOpen()) return
      onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  /** 点击通知卡片：标记已读 + 跳转 + 关闭浮层 */
  function handleOpen(n: Notification) {
    if (!n.read) markRead.mutate(n.id)
    onOpen?.(n)
    onClose()
  }

  const tabs: { id: Tab; labelKey: string }[] = [
    { id: 'all', labelKey: 'notif.tabAll' },
    { id: 'unread', labelKey: 'notif.tabUnread' },
    { id: 'mail_new', labelKey: 'notif.tabMail' },
    { id: 'mail_rule', labelKey: 'notif.tabRule' },
    { id: 'sync_failed', labelKey: 'notif.tabSync' },
    { id: 'account_status', labelKey: 'notif.tabAccount' },
  ]

  return (
    // 遮罩层复用设置浮层的那一套（点空白关闭 + 暗色加深 + 淡入）
    <div
      className="settings-backdrop"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div
        ref={trapRef}
        className="notif-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={t('notif.title')}
        onMouseDown={(e) => e.stopPropagation()}
      >
      {/* 顶栏：标题与操作合并在同一行。
          原来标题是独占一行的 28px 大字（全屏页面的排版），搬进浮层后
          光标题区就吃掉近 90px 高，留给通知条目的空间反而不够。 */}
      <div className="fp-tabs">
        <div className="nd-head">
          <span className="nd-title">{t('notif.title')}</span>
          <span className="nd-sub">
            {isZh
              ? `${unreadCount} 条未读 · 共 ${all.length} 条`
              : `${unreadCount} unread · ${all.length} total`}
          </span>
        </div>
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
        <button
          type="button"
          className="icon-btn"
          onClick={onClose}
          title={t('reader.close')}
          aria-label={t('reader.close')}
        >
          <Icon name="close" size={14} />
        </button>
      </div>

      <div className="fullpage">
        {/* tab 行 */}
        {/* 一组互斥的筛选器，语义上是 tablist 而不是六个孤立按钮：
            没有 role 的话读屏既不报「第 2 项，共 6 项」也不报哪个是选中的，
            而且 Tab 要按六次才能走完。加上 role 之后按 ARIA 惯例改成
            roving tabindex——只有选中项进 Tab 序列，组内用方向键移动。 */}
        <div className="notif-tabs" role="tablist" aria-label={t('notif.filterLabel')}>
          {tabs.map((x) => (
            <button
              key={x.id}
              id={`notif-tab-${x.id}`}
              type="button"
              role="tab"
              aria-selected={tab === x.id}
              aria-controls="notif-tabpanel"
              tabIndex={tab === x.id ? 0 : -1}
              className={'notif-tab' + (tab === x.id ? ' active' : '')}
              onClick={() => setTab(x.id)}
              onKeyDown={(e) => {
                // 带修饰键的方向键让给系统/浏览器：Ctrl+← / ⌥+→ 是很多人的
                // 词间移动习惯，吞掉它会变成"想移动光标结果换了筛选器"
                if (e.ctrlKey || e.metaKey || e.altKey) return
                const i = tabs.findIndex((y) => y.id === tab)
                let target: number | null = null
                if (e.key === 'ArrowRight') target = (i + 1) % tabs.length
                else if (e.key === 'ArrowLeft') target = (i - 1 + tabs.length) % tabs.length
                // Home / End 是 ARIA 对 tablist 的选配键位，键盘用户按惯例会试
                else if (e.key === 'Home') target = 0
                else if (e.key === 'End') target = tabs.length - 1
                if (target == null) return
                e.preventDefault()
                setTab(tabs[target].id)
                // 焦点跟着走，否则下一次方向键还是从原处算起
                const all = e.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>(
                  '[role="tab"]',
                )
                all?.[target]?.focus()
              }}
            >
              <span>{t(x.labelKey)}</span>
            </button>
          ))}
        </div>

        {/* tab 对应的面板。只做一半的 tablist 语义会让读屏报完「标签，已选中，
            第 2 项共 6 项」之后，用户按惯例去找面板却找不到。
            tabIndex=0：面板本身可滚动，键盘用户要能把焦点放进来翻页。 */}
        <div
          className="fp-body"
          id="notif-tabpanel"
          role="tabpanel"
          aria-labelledby={`notif-tab-${tab}`}
          tabIndex={0}
        >
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
                    onClick={() => handleOpen(n)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') handleOpen(n)
                    }}
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
    </div>
  )
}

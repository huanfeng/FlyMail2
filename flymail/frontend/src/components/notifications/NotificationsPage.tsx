// 通知屏组件（特殊模式：占据第三栏 reader 区，左侧 sidebar + 列表仍可见）
// 参考蓝本：.dev/mailmaster/src_extracted/06_87910dfb.js (NotificationsScreen)
// FlyMail 后端暂无通知数据，渲染屏框架 + tab + 空态降级，不放假数据、不报错。
// 所有颜色严格使用 CSS 令牌，不写死任何颜色值。

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'

// ── 通知 tab 类型 ────────────────────────────────────────
type NotifTab = 'all' | 'unread' | 'mail' | 'security'

// ── Props ────────────────────────────────────────────────
interface NotificationsPageProps {
  /** 返回邮件视图的回调 */
  onBack: () => void
}

// ── 主组件 ───────────────────────────────────────────────
export function NotificationsPage({ onBack }: NotificationsPageProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<NotifTab>('all')

  // 后端暂无通知数据，各 tab 计数恒为 0
  const tabs: { id: NotifTab; labelKey: string }[] = [
    { id: 'all', labelKey: 'notif.tabAll' },
    { id: 'unread', labelKey: 'notif.tabUnread' },
    { id: 'mail', labelKey: 'notif.tabMail' },
    { id: 'security', labelKey: 'notif.tabSecurity' },
  ]

  return (
    // 第三栏容器（外层 .col.reader 由 AppLayout 提供）
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: 'var(--bg)' }}>

      {/* 顶部操作栏 .fp-tabs */}
      <div className="fp-tabs">
        {/* 返回按钮 */}
        <button type="button" className="fp-back" onClick={onBack}>
          <span style={{ transform: 'scaleX(-1)', display: 'inline-block' }}>
            <Icon name="chevron-right" size={14} />
          </span>
          {t('notif.backToInbox')}
        </button>
      </div>

      {/* 内容 .fullpage */}
      <div className="fullpage">
        {/* 页头 .fp-head */}
        <div className="fp-head">
          <div style={{ flex: 1 }}>
            <div className="fp-title">{t('notif.title')}</div>
            <div className="fp-sub">{t('notif.sub')}</div>
          </div>
        </div>

        {/* tab 行 .notif-tabs */}
        <div className="notif-tabs">
          {tabs.map((x) => (
            <button
              key={x.id}
              type="button"
              className={'notif-tab' + (tab === x.id ? ' active' : '')}
              onClick={() => setTab(x.id)}
            >
              <span>{t(x.labelKey)}</span>
              <span className="badge">0</span>
            </button>
          ))}
        </div>

        {/* 正文 .fp-body — 空态 */}
        <div className="fp-body">
          <div className="notif-empty">
            <div
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: 22,
                color: 'var(--ink-2)',
                marginBottom: 6,
              }}
            >
              {t('notif.emptyTitle')}
            </div>
            <div style={{ fontSize: 14, color: 'var(--ink-3)' }}>
              {t('notif.emptyHint')}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

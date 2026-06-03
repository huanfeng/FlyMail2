// 侧边栏组件 — 复刻 MailMaster Sidebar 结构
// 参考 .dev/mailmaster/src_extracted/03_f2308e64.js + app.css
// 所有颜色严格使用 CSS 设计令牌，不写死任何颜色值

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import type { IconName } from '@/components/ui/Icon'
import { useNotificationUnread, useMe } from '@/lib/queries'
import type { AggregateView } from '@/lib/queries'
import type { Account, Folder } from '@/lib/types'
import { auth } from '@/lib/auth'

/** 从名称取首字母（最多 2 个），用于头像占位 */
function nameInitials(name: string): string {
  const s = name.trim()
  if (!s) return '?'
  return s
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0] ?? '')
    .join('')
    .toUpperCase()
}

// ── 文件夹类型 → 图标名称映射 ─────────────────────────────
const FOLDER_ICON: Record<string, IconName> = {
  inbox: 'inbox',
  sent: 'send',
  drafts: 'draft',
  trash: 'trash',
  junk: 'tag',
  archive: 'archive',
  custom: 'folder',
}

/** 应用级视图（第三栏模式）：邮件 / 通知 */
export type AppView = 'mail' | 'notif'

// ── Props ────────────────────────────────────────────────
interface Props {
  accounts: Account[]
  folders: Folder[]
  activeAccountId: number | null
  activeFolderId: number | null
  syncing: boolean
  /** 当前第三栏视图，用于高亮铃铛 */
  activeView: AppView
  /** 设置浮层是否打开，用于高亮齿轮 */
  settingsOpen: boolean
  /** 当前激活的聚合入口（null 表示未选中聚合） */
  activeAgg: AggregateView | null
  /** 聚合入口徽标计数 */
  aggCounts: Record<AggregateView, number>
  onSelectAccount: (id: number) => void
  onSelectFolder: (id: number) => void
  onSelectAggregate: (view: AggregateView) => void
  onSync: (accountId: number) => void
  onAddAccount: () => void
  /** 切换第三栏视图（mail / notif） */
  onSetView: (view: AppView) => void
  /** 切换设置浮层 */
  onToggleSettings: () => void
  onCompose: () => void
  onOpenDrafts: (accountId: number) => void
}

// ── 文件夹行 ─────────────────────────────────────────────

interface FolderRowProps {
  iconName: IconName
  label: string
  active: boolean
  count?: number
  onClick: () => void
}

function FolderRow({ iconName, label, active, count, onClick }: FolderRowProps) {
  return (
    <button
      type="button"
      className={'folder-row' + (active ? ' active' : '')}
      onClick={onClick}
    >
      {/* 文件夹图标 */}
      <span className="f-icon">
        <Icon name={iconName} size={13} />
      </span>
      {/* 文件夹名 */}
      <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', textAlign: 'left' }}>
        {label}
      </span>
      {/* 未读/数量 badge */}
      {count !== undefined && count > 0 && (
        <span className="f-count">{count > 99 ? '99+' : count}</span>
      )}
    </button>
  )
}

// ── 账户行（可展开，含文件夹列表）────────────────────────

interface AccountBlockProps {
  acc: Account
  expanded: boolean
  active: boolean
  folders: Folder[]
  activeFolderId: number | null
  syncing: boolean
  onToggleExpand: () => void
  onSync: () => void
  onSelectFolder: (id: number) => void
  onOpenDrafts: () => void
}

function AccountBlock({
  acc,
  expanded,
  active,
  folders,
  activeFolderId,
  syncing,
  onToggleExpand,
  onSync,
  onSelectFolder,
  onOpenDrafts,
}: AccountBlockProps) {
  const { t } = useTranslation()

  // 计算账户下全部未读数
  const totalUnread = folders.reduce((sum, f) => sum + (f.unread_count ?? 0), 0)

  return (
    <div>
      {/* 账户标题行 — 点击展开/收起 */}
      <div className="group" style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
        <button
          type="button"
          className={'account-row' + (active ? ' active' : '')}
          onClick={onToggleExpand}
          style={{ flex: 1 }}
        >
          {/* 展开/收起 caret */}
          <span className={'caret' + (expanded ? ' open' : '')}>
            <Icon name="chevron-right" size={12} stroke={2} />
          </span>
          {/* 账户色点（用 accent 色代替自定义颜色，FlyMail 账户无 color 字段） */}
          <span
            className="acct-dot"
            style={{ background: 'var(--accent)' }}
          />
          {/* 账户名（优先显示 name，回落到 email） */}
          <span className="acct-name">{acc.name || acc.email}</span>
          {/* 未读总数 */}
          {totalUnread > 0 && (
            <span className="acct-unread">{totalUnread > 99 ? '99+' : totalUnread}</span>
          )}
        </button>

        {/* hover 显示：同步按钮（账户管理移至设置弹框，此处不再放编辑/删除）*/}
        <div
          className="account-row-actions"
          style={{ display: 'flex', alignItems: 'center', flexShrink: 0, paddingRight: 6 }}
        >
          {/* 同步（进行中时旋转） */}
          <button
            type="button"
            className="icon-btn"
            title={t('sync.trigger')}
            aria-label={t('sync.trigger')}
            onClick={(e) => { e.stopPropagation(); onSync() }}
            style={{ width: 22, height: 22 }}
          >
            <Icon
              name="circle-dot"
              size={11}
              className={syncing && active ? 'spin-anim' : undefined}
            />
          </button>
        </div>
      </div>

      {/* 展开的文件夹列表 */}
      {expanded && (
        <div className="folder-list">
          {folders
            .filter((f) => f.selectable)
            .map((f) => {
              const iconName: IconName = FOLDER_ICON[f.type] ?? 'folder'
              // 自定义文件夹用 display_name，系统文件夹走 i18n
              const label = f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
              return (
                <FolderRow
                  key={f.id}
                  iconName={iconName}
                  label={label}
                  active={f.id === activeFolderId}
                  count={f.type === 'inbox' || f.type === 'junk' ? f.unread_count : undefined}
                  onClick={() => onSelectFolder(f.id)}
                />
              )
            })}

          {/* 草稿箱（本地）入口 — 独立于 IMAP 文件夹 */}
          <FolderRow
            iconName="draft"
            label={t('compose.draftsBox')}
            active={false}
            onClick={onOpenDrafts}
          />
        </div>
      )}
    </div>
  )
}

// ── 主组件 ───────────────────────────────────────────────

export function AccountSidebar({
  accounts,
  folders,
  activeAccountId,
  activeFolderId,
  syncing,
  activeView,
  settingsOpen,
  activeAgg,
  aggCounts,
  onSelectAccount,
  onSelectFolder,
  onSelectAggregate,
  onSync,
  onAddAccount,
  onSetView,
  onToggleSettings,
  onCompose,
  onOpenDrafts,
}: Props) {
  const { t } = useTranslation()
  // 站内未读通知数（铃铛角标）
  const { data: unreadNotifs = 0 } = useNotificationUnread()
  // 当前管理员资料（底部用户卡）
  const { data: me } = useMe()

  function handleLogout() {
    auth.clear()
    window.location.href = '/login'
  }

  // 各账户展开状态（默认展开前两个）
  const [expanded, setExpanded] = useState<Record<number, boolean>>(() => {
    const init: Record<number, boolean> = {}
    accounts.forEach((a, i) => { init[a.id] = i < 2 })
    return init
  })

  function toggleExpand(id: number) {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  // 聚合入口在邮件视图下才可能高亮
  const inMail = activeView === 'mail' && !settingsOpen

  // 聚合入口配置
  const aggItems: { id: AggregateView; icon: IconName; labelKey: string }[] = [
    { id: 'inbox', icon: 'inbox', labelKey: 'sidebar.allInboxes' },
    { id: 'unread', icon: 'circle-dot', labelKey: 'sidebar.allUnread' },
    { id: 'starred', icon: 'star', labelKey: 'sidebar.starred' },
  ]

  return (
    // .col.sidebar 由 AppLayout 提供外层容器，这里只填充内容
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>

      {/* ── 顶部品牌栏 .sidebar-head ───────────────────────── */}
      <div className="sidebar-head">
        {/* 品牌方块 logo，点击回到邮件视图 */}
        <button
          type="button"
          className="brand-mark"
          style={{ cursor: 'pointer' }}
          onClick={() => onSetView('mail')}
          aria-label={t('app.name')}
        >
          F
        </button>
        {/* 品牌名 */}
        <div className="brand-name">{t('app.name')}</div>
        {/* brand-dot：视觉装饰 */}
        <div className="brand-dot" />
        {/* 铃铛按钮：切换到通知视图（第三栏特殊模式）*/}
        <button
          type="button"
          className={'icon-btn bell-wrap' + (activeView === 'notif' ? ' active' : '')}
          title={t('notif.title')}
          aria-label={t('notif.title')}
          onClick={() => onSetView(activeView === 'notif' ? 'mail' : 'notif')}
        >
          <Icon name="bell" size={14} />
          {unreadNotifs > 0 && <span className="bell-badge" />}
        </button>
      </div>

      {/* ── 写邮件按钮 .compose-btn ────────────────────────── */}
      <button
        type="button"
        className="compose-btn"
        onClick={onCompose}
      >
        <Icon name="compose" size={14} />
        <span>{t('sidebar.compose')}</span>
        {/* 快捷键提示 */}
        <span className="kbd">C</span>
      </button>

      {/* ── 滚动区 .sidebar-scroll ─────────────────────────── */}
      <div className="sidebar-scroll">

        {/* 聚合入口：所有收件箱 / 所有未读 / 星标（跨所有账户）*/}
        {aggItems.map((it) => (
          <button
            key={it.id}
            type="button"
            className={'side-btn' + (inMail && activeAgg === it.id ? ' active' : '')}
            onClick={() => onSelectAggregate(it.id)}
          >
            <Icon name={it.icon} size={14} />
            <span style={{ flex: 1, textAlign: 'left' }}>{t(it.labelKey)}</span>
            {aggCounts[it.id] > 0 && (
              <span className="count">{aggCounts[it.id] > 99 ? '99+' : aggCounts[it.id]}</span>
            )}
          </button>
        ))}

        {/* 账户区 section label */}
        <div className="side-section-label">
          <span>{t('sidebar.accounts')}</span>
          {/* 添加账户按钮 */}
          <button
            type="button"
            className="add"
            title={t('account.add')}
            aria-label={t('account.add')}
            onClick={onAddAccount}
          >
            <Icon name="plus" size={12} />
          </button>
        </div>

        {/* 账户列表 */}
        {accounts.map((acc) => (
          <AccountBlock
            key={acc.id}
            acc={acc}
            expanded={!!expanded[acc.id]}
            active={inMail && activeAgg == null && acc.id === activeAccountId}
            // 只有激活账户才传入文件夹，其余传空数组节省渲染
            folders={acc.id === activeAccountId ? folders : []}
            activeFolderId={activeFolderId}
            syncing={syncing}
            onToggleExpand={() => {
              // 展开时同时切换账户选中（若点击非激活账户）
              if (acc.id !== activeAccountId) onSelectAccount(acc.id)
              toggleExpand(acc.id)
            }}
            onSync={() => onSync(acc.id)}
            onSelectFolder={onSelectFolder}
            onOpenDrafts={() => onOpenDrafts(acc.id)}
          />
        ))}

        {/* labels 标签区：FlyMail 暂无标签功能，本阶段完全隐藏（不放静态假数据） */}

      </div>

      {/* ── 底部 .sidebar-foot ─────────────────────────────── */}
      <div className="sidebar-foot">
        {/* 当前管理员信息：点击打开设置（资料分区） */}
        <button
          type="button"
          className="me"
          style={{ cursor: 'pointer', background: 'transparent', border: 0, textAlign: 'left' }}
          title={t('settings.navProfile')}
          onClick={onToggleSettings}
        >
          {/* 头像方块（取展示名/用户名首字母） */}
          <div
            className="avatar-sq"
            style={{
              width: 28,
              height: 28,
              borderRadius: 6,
              background: 'var(--accent)',
              display: 'grid',
              placeItems: 'center',
              color: 'white',
              fontSize: 11,
              fontWeight: 600,
              flexShrink: 0,
            }}
          >
            {nameInitials(me?.display_name || me?.username || 'FM')}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="me-name" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {me?.display_name || me?.username || t('app.name')}
            </div>
            <div className="me-mail" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {me?.email
                || (accounts.length > 0
                  ? t('sidebar.accountCount', { count: accounts.length })
                  : t('sidebar.noAccounts'))}
            </div>
          </div>
        </button>

        {/* 设置入口 → 切换设置浮层 */}
        <button
          type="button"
          className={'icon-btn' + (settingsOpen ? ' active' : '')}
          title={t('settings.title')}
          aria-label={t('settings.title')}
          onClick={onToggleSettings}
        >
          <Icon name="settings" size={15} />
        </button>

        {/* 登出 */}
        <button
          type="button"
          className="icon-btn"
          title={t('sidebar.logout')}
          aria-label={t('sidebar.logout')}
          onClick={handleLogout}
        >
          <Icon name="logout" size={15} />
        </button>
      </div>

    </div>
  )
}

// 侧边栏组件 — 复刻 MailMaster Sidebar 结构
// 参考 .dev/mailmaster/src_extracted/03_f2308e64.js + app.css
// 所有颜色严格使用 CSS 设计令牌，不写死任何颜色值

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import type { IconName } from '@/components/ui/Icon'
import { CtxMenu, type CtxMenuItem } from '@/components/ui/ContextMenu'
import { AccountDialog } from '@/components/mail/AccountDialog'
import {
  useNotificationUnread,
  useAccountUnread,
  useMe,
  useSetAccountEnabled,
  useDeleteAccount,
} from '@/lib/queries'
import type { AggregateView } from '@/lib/queries'
import type { Account, Folder, SyncStatus } from '@/lib/types'
import { auth } from '@/lib/auth'

/**
 * 同步进度行。
 *
 * 三种表达，按后端给得出什么来分：
 * - 知道总数（messages 阶段）→ 确定进度条 + 「已处理 / 总数」
 * - 还不知道总数（queued / folders 阶段）→ 不确定进度条 + 阶段名
 * - 后端没返回状态（刚触发、状态还没建立）→ 也走不确定那一支
 *
 * 刻意**不**显示百分比数字：total 是「这一轮要处理的邮件数」，
 * 各文件夹是边发现边累加的，百分比会往回跳。计数不会有这个问题。
 */
export function SyncProgress({ status }: { status: SyncStatus | null }) {
  const { t } = useTranslation()
  const total = status?.total ?? 0
  const processed = status?.processed ?? 0
  const determinate = total > 0

  // 复用已有的 sync.* 文案，不另起一层 sync.phase.*——同一件事两套键是下一个漂移源
  const phaseLabel =
    status?.phase === 'queued'
      ? t('sync.queued')
      : status?.phase === 'folders'
        ? t('sync.folders')
        : t('sync.messages')

  return (
    // ⚠ 整块**不能**是 live region。计数每秒变一次（轮询间隔 1s），而首次导入
    // 是分钟级的——读屏用户会连续几分钟每秒听一句「正在同步邮件 37 / 2000」，
    // 新邮件提醒、操作结果、Shell 那个 announce 全被挤掉。
    // 进度交给 progressbar（读屏按用户自己的节奏查询），只把**阶段变化**
    // 播报出去：一次同步最多三次（queued → folders → messages）。
    <div className="sync-progress">
      <div
        className={'sync-bar' + (determinate ? '' : ' indeterminate')}
        role="progressbar"
        aria-valuemin={0}
        {...(determinate
          ? {
              'aria-valuemax': total,
              'aria-valuenow': processed,
              'aria-valuetext': `${processed} / ${total}`,
            }
          : {})}
        aria-label={phaseLabel}
      >
        {determinate && (
          <span
            className="sync-bar-fill"
            style={{ width: `${Math.min(100, (processed / total) * 100)}%` }}
          />
        )}
      </div>
      {/* 可见文本已被上面的 progressbar 完整表达，对读屏是重复的 */}
      <span className="sync-progress-text" aria-hidden="true">
        {determinate ? `${phaseLabel} ${processed} / ${total}` : phaseLabel}
      </span>
      <span className="sr-only" role="status" aria-live="polite">
        {phaseLabel}
      </span>
    </div>
  )
}

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


// ── 侧栏错误行 ───────────────────────────────────────────
//
// 侧栏的两条链路失败时 data 都回落成空数组，界面与「一个账户都没有」
// 「这个账户没有文件夹」完全同形。没有这一行，一次 500 就表现为一个空侧栏：
// 既看不出是故障，也没有重试的入口。
function SideError({ text, onRetry }: { text: string; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="side-error" role="status">
      <span>{text}</span>
      {onRetry && (
        <button type="button" className="side-error-retry" onClick={onRetry}>
          {t('app.retry')}
        </button>
      )}
    </div>
  )
}

// ── Props ────────────────────────────────────────────────
interface Props {
  accounts: Account[]
  /** 账户列表加载失败（与「一个账户都没有」是两回事，必须分开表达） */
  accountsError?: unknown
  onRetryAccounts?: () => void
  folders: Folder[]
  /** 当前账户的文件夹加载失败 */
  foldersError?: unknown
  onRetryFolders?: () => void
  activeAccountId: number | null
  activeFolderId: number | null
  syncing: boolean
  /** 正在同步的账户 id（null = 无）。转圈与进度只给这一个账户，
      而不是「有同步在跑就让当前账户转」。 */
  syncingAccountId: number | null
  /** 那个账户的同步进度 */
  syncStatus: SyncStatus | null
  /** 通知浮层是否打开，用于高亮铃铛 */
  notifOpen: boolean
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
  /** 切换通知浮层 */
  onToggleNotif: () => void
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
  /** 右键菜单项（可选） */
  ctxItems?: CtxMenuItem[]
}

function FolderRow({ iconName, label, active, count, onClick, ctxItems }: FolderRowProps) {
  const btn = (
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
  return ctxItems && ctxItems.length > 0 ? <CtxMenu trigger={btn} items={ctxItems} /> : btn
}

// ── 账户行（可展开，含文件夹列表）────────────────────────

interface AccountBlockProps {
  acc: Account
  expanded: boolean
  active: boolean
  folders: Folder[]
  /** 文件夹加载失败（仅激活账户会传：非激活账户压根没发这个请求） */
  foldersError?: unknown
  onRetryFolders?: () => void
  activeFolderId: number | null
  /** 这个账户此刻是否在同步（不是「有账户在同步」——原先传的是全局布尔，
      于是同步账户 B 时转圈出现在 A 上） */
  syncing: boolean
  /** 同步进度。仅正在同步的那个账户会拿到，其余为 null */
  syncStatus: SyncStatus | null
  /** 账户级未读数（后端去重口径，见 useAccountUnread） */
  unread: number
  onToggleExpand: () => void
  onSync: () => void
  onSelectFolder: (id: number) => void
  onOpenDrafts: () => void
  // ── 右键菜单动作 ──
  onEdit: () => void
  onToggleEnabled: () => void
  onDelete: () => void
}

function AccountBlock({
  acc,
  expanded,
  active,
  folders,
  foldersError,
  onRetryFolders,
  activeFolderId,
  syncing,
  syncStatus,
  unread,
  onToggleExpand,
  onSync,
  onSelectFolder,
  onOpenDrafts,
  onEdit,
  onToggleEnabled,
  onDelete,
}: AccountBlockProps) {
  const { t } = useTranslation()

  // 账户右键菜单：同步 / 编辑 / 启停 / 删除
  const accountCtxItems: CtxMenuItem[] = [
    { key: 'sync', label: t('ctx.syncNow'), icon: 'circle-dot', onSelect: onSync, disabled: !acc.enabled },
    { key: 'edit', label: t('ctx.editAccount'), icon: 'compose', onSelect: onEdit },
    {
      key: 'enabled',
      label: acc.enabled ? t('ctx.disableAccount') : t('ctx.enableAccount'),
      icon: 'circle-dot',
      onSelect: onToggleEnabled,
    },
    { key: 'sep', separator: true },
    { key: 'del', label: t('ctx.deleteAccount'), icon: 'trash', destructive: true, onSelect: onDelete },
  ]
  // 文件夹右键菜单：立即同步该账户
  const folderCtxItems: CtxMenuItem[] = [
    { key: 'sync', label: t('ctx.syncNow'), icon: 'circle-dot', onSelect: onSync, disabled: !acc.enabled },
  ]

  return (
    <div>
      {/* 账户标题行 — 点击展开/收起，右键弹操作菜单 */}
      <CtxMenu
        items={accountCtxItems}
        trigger={
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
          {/* 未读总数（后端去重口径：收件箱+自定义文件夹，同一封跨标签只计一次） */}
          {unread > 0 && (
            <span className="acct-unread">{unread > 99 ? '99+' : unread}</span>
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
            className="icon-btn compact"
            title={t('sync.trigger')}
            aria-label={t('sync.trigger')}
            // 停用的账户后端会直接拒（500），同步中再点会 409——两种都只换来一条
            // 错误提示。右键菜单里那一项本来就判了 acc.enabled，这个按钮漏了。
            disabled={syncing || !acc.enabled}
            onClick={(e) => { e.stopPropagation(); onSync() }}
          >
            {/* 判据只看「这个账户在不在同步」。原先是 `syncing && active`——
                那个 active 让同步非当前账户时屏幕上完全没有变化。 */}
            <Icon
              name="circle-dot"
              size={11}
              className={syncing ? 'spin-anim' : undefined}
            />
          </button>
        </div>
          </div>
        }
      />

      {/* 同步进度。首次导入几千封是分钟级操作，此前全部反馈只有上面那个 11px 的
          圆点在转——用户无从判断是在动、卡住了、还是快好了。
          后端的 Status 一直带着 phase/total/processed，前端一行都没用过。 */}
      {syncing && <SyncProgress status={syncStatus} />}

      {/* 展开的文件夹列表 */}
      {expanded && (
        <div className="folder-list">
          {foldersError != null && (
            <SideError text={t('sidebar.foldersError')} onRetry={onRetryFolders} />
          )}
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
                  count={
                    // 文件夹行徽标：收件箱/垃圾邮件/自定义显示各自未读（主流客户端行为），
                    // archive（Gmail 所有邮件）/回收站/已发送/草稿不显示
                    f.type === 'inbox' || f.type === 'junk' || f.type === 'custom'
                      ? f.unread_count
                      : undefined
                  }
                  onClick={() => onSelectFolder(f.id)}
                  ctxItems={folderCtxItems}
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
  accountsError,
  onRetryAccounts,
  folders,
  foldersError,
  onRetryFolders,
  activeAccountId,
  activeFolderId,
  syncing,
  syncingAccountId,
  syncStatus,
  notifOpen,
  settingsOpen,
  activeAgg,
  aggCounts,
  onSelectAccount,
  onSelectFolder,
  onSelectAggregate,
  onSync,
  onAddAccount,
  onToggleNotif,
  onToggleSettings,
  onCompose,
  onOpenDrafts,
}: Props) {
  const { t } = useTranslation()
  const confirm = useConfirm()
  // 站内未读通知数（铃铛角标）
  const { data: unreadNotifs = 0 } = useNotificationUnread()
  // 各账户未读（后端统一口径，非激活账户也能显示）
  const { data: accountUnread = {} } = useAccountUnread()
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

  // ── 账户右键菜单动作（编辑对话框 + 启停/删除 mutation，自包含于侧栏） ──
  const [editAcc, setEditAcc] = useState<Account | null>(null)
  const setEnabled = useSetAccountEnabled()
  const deleteAccount = useDeleteAccount()

  async function handleDeleteAccount(acc: Account) {
    const ok = await confirm({
      title: t('account.deleteConfirm'),
      body: t('account.deleteConfirmBody'),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    deleteAccount.mutate(acc.id)
  }

  // 聚合入口在邮件视图下才可能高亮
  // 聚合入口的高亮只在没有浮层压在上面时才代表「当前所在位置」
  const inMail = !notifOpen && !settingsOpen

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
        {/* 品牌方块 logo。通知改为浮层后没有「回到邮件视图」这回事了
            （主区域始终是邮件），于是退回纯展示元素，不再是可点击控件。 */}
        <div className="brand-mark" aria-hidden="true">F</div>
        {/* 品牌名 */}
        <div className="brand-name">{t('app.name')}</div>
        {/* brand-dot：视觉装饰 */}
        <div className="brand-dot" />
        {/* 铃铛按钮：开关通知浮层 */}
        <button
          type="button"
          className={'icon-btn bell-wrap' + (notifOpen ? ' active' : '')}
          title={t('notif.title')}
          aria-label={t('notif.title')}
          onClick={onToggleNotif}
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
        {accountsError != null && (
          <SideError text={t('sidebar.accountsError')} onRetry={onRetryAccounts} />
        )}
        {accounts.map((acc) => (
          <AccountBlock
            key={acc.id}
            acc={acc}
            expanded={!!expanded[acc.id]}
            active={inMail && activeAgg == null && acc.id === activeAccountId}
            // 只有激活账户才传入文件夹，其余传空数组节省渲染
            folders={acc.id === activeAccountId ? folders : []}
            foldersError={acc.id === activeAccountId ? foldersError : undefined}
            onRetryFolders={onRetryFolders}
            activeFolderId={activeFolderId}
            syncing={syncing && acc.id === syncingAccountId}
            syncStatus={acc.id === syncingAccountId ? syncStatus : null}
            unread={accountUnread[acc.id] ?? 0}
            onToggleExpand={() => {
              // 展开时同时切换账户选中（若点击非激活账户）
              if (acc.id !== activeAccountId) onSelectAccount(acc.id)
              toggleExpand(acc.id)
            }}
            onSync={() => onSync(acc.id)}
            onSelectFolder={onSelectFolder}
            onOpenDrafts={() => onOpenDrafts(acc.id)}
            onEdit={() => setEditAcc(acc)}
            onToggleEnabled={() => setEnabled.mutate({ id: acc.id, enabled: !acc.enabled })}
            onDelete={() => handleDeleteAccount(acc)}
          />
        ))}

        {/* labels 标签区：FlyMail 暂无标签功能，本阶段完全隐藏（不放静态假数据） */}

      </div>

      {/* 账户右键「编辑」对话框 */}
      <AccountDialog
        open={editAcc !== null}
        account={editAcc}
        onOpenChange={(open) => { if (!open) setEditAcc(null) }}
      />

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

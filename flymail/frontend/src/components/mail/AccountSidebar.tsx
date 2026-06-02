import { useTranslation } from 'react-i18next'
import {
  Inbox,
  Send,
  FileText,
  Trash2,
  ShieldAlert,
  Archive,
  Folder as FolderIcon,
  RefreshCw,
  Plus,
  Pencil,
  Settings,
  PenSquare,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { Account, Folder } from '@/lib/types'

// 文件夹类型到图标的映射
const folderIcon: Record<string, LucideIcon> = {
  inbox: Inbox,
  sent: Send,
  drafts: FileText,
  trash: Trash2,
  junk: ShieldAlert,
  archive: Archive,
}

interface Props {
  accounts: Account[]
  folders: Folder[]
  activeAccountId: number | null
  activeFolderId: number | null
  syncing: boolean
  onSelectAccount: (id: number) => void
  onSelectFolder: (id: number) => void
  onSync: (accountId: number) => void
  onAddAccount: () => void
  onEditAccount: (account: Account) => void
  onDeleteAccount: (account: Account) => void
  onOpenSettings: () => void
  onCompose: () => void
  onOpenDrafts: (accountId: number) => void
}

// ── 文件夹行 ─────────────────────────────────────────────

interface FolderRowProps {
  icon: LucideIcon
  label: string
  active: boolean
  unread?: number
  onClick: () => void
}

function FolderRow({ icon: Icon, label, active, unread, onClick }: FolderRowProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="group relative flex w-full items-center gap-2 rounded-md py-1.5 pl-8 pr-2.5 text-[13px] outline-none focus-visible:ring-2 transition-colors"
      style={{
        color: active ? 'var(--accent-ink)' : 'var(--ink-2)',
        background: active ? 'var(--accent-wash)' : 'transparent',
      }}
      onMouseEnter={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
      }}
      onMouseLeave={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'transparent'
      }}
    >
      {/* 激活状态左侧强调条 */}
      {active && (
        <span
          className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full"
          style={{ background: 'var(--accent-color)' }}
        />
      )}

      {/* 文件夹图标，激活时跟随强调色 */}
      <Icon
        size={14}
        style={{ color: active ? 'var(--accent-color)' : 'var(--ink-3)', flexShrink: 0 }}
      />

      {/* 文件夹名，左对齐，截断 */}
      <span className="flex-1 truncate text-left">{label}</span>

      {/* 未读 badge */}
      {unread !== undefined && unread > 0 && (
        <span
          className="ml-auto flex-shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-medium leading-none"
          style={{
            color: active ? 'var(--accent-ink)' : 'var(--ink-3)',
            background: active ? 'transparent' : 'var(--bg-sunk)',
            minWidth: 18,
            textAlign: 'center',
          }}
        >
          {unread > 99 ? '99+' : unread}
        </span>
      )}
    </button>
  )
}

// ── 账户区块 ─────────────────────────────────────────────

interface AccountBlockProps {
  acc: Account
  active: boolean
  folders: Folder[]
  activeFolderId: number | null
  syncing: boolean
  onSelect: () => void
  onSync: () => void
  onEdit: () => void
  onDelete: () => void
  onSelectFolder: (id: number) => void
  onOpenDrafts: () => void
}

function AccountBlock({
  acc,
  active,
  folders,
  activeFolderId,
  syncing,
  onSelect,
  onSync,
  onEdit,
  onDelete,
  onSelectFolder,
  onOpenDrafts,
}: AccountBlockProps) {
  const { t } = useTranslation()

  return (
    <div className="mb-1">
      {/* 账户标题行 */}
      <div className="group relative flex items-center gap-1 px-2 py-1">
        {/* 账户名按钮（主区域） */}
        <button
          type="button"
          onClick={onSelect}
          className="flex flex-1 min-w-0 items-center gap-1.5 rounded-md px-2 py-1 text-[11.5px] font-medium uppercase tracking-wide transition-colors outline-none focus-visible:ring-2"
          style={{
            color: active ? 'var(--ink-2)' : 'var(--ink-3)',
          }}
        >
          <span className="truncate">{acc.name || acc.email}</span>
        </button>

        {/* hover 时显示编辑/删除 */}
        <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
          <button
            type="button"
            title={t('account.edit')}
            aria-label={t('account.edit')}
            onClick={(e) => { e.stopPropagation(); onEdit() }}
            className="rounded p-1 transition-colors outline-none focus-visible:ring-2"
            style={{ color: 'var(--ink-4)' }}
            onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-2)' }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-4)' }}
          >
            <Pencil size={11} />
          </button>
          <button
            type="button"
            title={t('account.delete')}
            aria-label={t('account.delete')}
            onClick={(e) => { e.stopPropagation(); onDelete() }}
            className="rounded p-1 transition-colors outline-none focus-visible:ring-2"
            style={{ color: 'var(--ink-4)' }}
            onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-2)' }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-4)' }}
          >
            <Trash2 size={11} />
          </button>
        </div>

        {/* 同步按钮：始终可见，激活时带旋转动画 */}
        <button
          type="button"
          title={t('sync.trigger')}
          aria-label={t('sync.trigger')}
          onClick={(e) => { e.stopPropagation(); onSync() }}
          className="rounded p-1 transition-colors outline-none focus-visible:ring-2"
          style={{ color: 'var(--ink-4)' }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-2)' }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--ink-4)' }}
        >
          <RefreshCw
            size={11}
            className={syncing && active ? 'animate-spin' : ''}
          />
        </button>
      </div>

      {/* 展开的文件夹树（仅激活账户显示） */}
      {active && (
        <div className="mb-1">
          {folders
            .filter((f) => f.selectable)
            .map((f) => {
              const Icon: LucideIcon = folderIcon[f.type] ?? FolderIcon
              const label = f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
              return (
                <FolderRow
                  key={f.id}
                  icon={Icon}
                  label={label}
                  active={f.id === activeFolderId}
                  unread={f.unread_count}
                  onClick={() => onSelectFolder(f.id)}
                />
              )
            })}

          {/* 草稿箱（本地）入口 */}
          <FolderRow
            icon={FileText}
            label={t('compose.draftsBox')}
            active={false}
            onClick={onOpenDrafts}
          />
        </div>
      )}

      {/* 账户间分隔线（多账户时的视觉隔离） */}
      <div className="mx-2 mt-1" style={{ borderTop: '1px solid var(--rule)' }} />
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
  onSelectAccount,
  onSelectFolder,
  onSync,
  onAddAccount,
  onEditAccount,
  onDeleteAccount,
  onOpenSettings,
  onCompose,
  onOpenDrafts,
}: Props) {
  const { t } = useTranslation()

  return (
    <div className="flex h-full flex-col" style={{ background: 'var(--bg-alt)' }}>

      {/* ── 顶部标题栏 ──────────────────────────────────── */}
      <div
        className="flex flex-shrink-0 items-center justify-between px-4 py-3"
        style={{ borderBottom: '1px solid var(--rule)' }}
      >
        <span className="text-[13px] font-semibold tracking-wide" style={{ color: 'var(--ink)' }}>
          {t('app.name')}
        </span>
        <button
          type="button"
          onClick={onAddAccount}
          title={t('account.add')}
          aria-label={t('account.add')}
          className="rounded-md p-1.5 transition-colors outline-none focus-visible:ring-2"
          style={{ color: 'var(--ink-3)' }}
          onMouseEnter={(e) => {
            ;(e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
            ;(e.currentTarget as HTMLElement).style.color = 'var(--ink)'
          }}
          onMouseLeave={(e) => {
            ;(e.currentTarget as HTMLElement).style.background = 'transparent'
            ;(e.currentTarget as HTMLElement).style.color = 'var(--ink-3)'
          }}
        >
          <Plus size={15} />
        </button>
      </div>

      {/* ── 写邮件按钮：主 CTA，醒目 ────────────────────── */}
      <div className="flex-shrink-0 px-3 py-2.5">
        <button
          type="button"
          onClick={onCompose}
          className="flex w-full items-center justify-center gap-2 rounded-lg px-3 py-2 text-[13px] font-medium transition-opacity outline-none focus-visible:ring-2 hover:opacity-90 active:opacity-80"
          style={{
            background: 'var(--accent-color)',
            color: 'var(--surface)',
            boxShadow: 'var(--shadow-sm)',
          }}
        >
          <PenSquare size={14} />
          <span>{t('sidebar.compose')}</span>
        </button>
      </div>

      {/* ── 账户 / 文件夹滚动区 ──────────────────────────── */}
      <div className="flex-1 overflow-y-auto px-1 py-1">
        {accounts.map((acc) => (
          <AccountBlock
            key={acc.id}
            acc={acc}
            active={acc.id === activeAccountId}
            folders={acc.id === activeAccountId ? folders : []}
            activeFolderId={activeFolderId}
            syncing={syncing}
            onSelect={() => onSelectAccount(acc.id)}
            onSync={() => onSync(acc.id)}
            onEdit={() => onEditAccount(acc)}
            onDelete={() => onDeleteAccount(acc)}
            onSelectFolder={onSelectFolder}
            onOpenDrafts={() => onOpenDrafts(acc.id)}
          />
        ))}
      </div>

      {/* ── 底部设置入口 ─────────────────────────────────── */}
      <div className="flex-shrink-0" style={{ borderTop: '1px solid var(--rule)' }}>
        <button
          type="button"
          onClick={onOpenSettings}
          className="flex w-full items-center gap-2.5 px-4 py-3 text-[13px] transition-colors outline-none focus-visible:ring-2"
          style={{ color: 'var(--ink-3)' }}
          onMouseEnter={(e) => {
            ;(e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
            ;(e.currentTarget as HTMLElement).style.color = 'var(--ink-2)'
          }}
          onMouseLeave={(e) => {
            ;(e.currentTarget as HTMLElement).style.background = 'transparent'
            ;(e.currentTarget as HTMLElement).style.color = 'var(--ink-3)'
          }}
        >
          <Settings size={14} />
          <span>{t('settings.title')}</span>
        </button>
      </div>
    </div>
  )
}

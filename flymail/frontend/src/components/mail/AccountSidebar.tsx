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
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { Account, Folder } from '@/lib/types'

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
}

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
}: Props) {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col">
      <div
        className="flex items-center justify-between px-4 py-3"
        style={{ borderBottom: '1px solid var(--rule)' }}
      >
        <span className="text-base font-medium">{t('app.name')}</span>
        <button
          type="button"
          onClick={onAddAccount}
          title={t('account.add')}
          aria-label={t('account.add')}
          className="rounded-md p-1 opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
          style={{ color: 'var(--ink-2)' }}
        >
          <Plus size={16} />
        </button>
      </div>
      <div className="flex-1 overflow-y-auto px-2 py-2">
        {accounts.map((acc) => (
          <div key={acc.id} className="mb-2">
            <div className="group relative flex items-center">
              <button
                type="button"
                onClick={() => onSelectAccount(acc.id)}
                className="flex flex-1 min-w-0 items-center justify-between rounded-md px-2 py-1.5 text-sm"
                style={{
                  color: acc.id === activeAccountId ? 'var(--ink)' : 'var(--ink-2)',
                  background: acc.id === activeAccountId ? 'var(--bg-hover)' : 'transparent',
                }}
              >
                <span className="truncate">{acc.name || acc.email}</span>
                <RefreshCw
                  size={14}
                  className={syncing && acc.id === activeAccountId ? 'animate-spin' : ''}
                  onClick={(e) => {
                    e.stopPropagation()
                    onSync(acc.id)
                  }}
                  aria-label={t('sync.trigger')}
                />
              </button>
              {/* 编辑 / 删除操作按钮，hover 时显示 */}
              <div className="absolute right-7 flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  type="button"
                  title={t('account.edit')}
                  aria-label={t('account.edit')}
                  onClick={(e) => {
                    e.stopPropagation()
                    onEditAccount(acc)
                  }}
                  className="rounded p-1 hover:bg-[var(--bg-hover)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  style={{ color: 'var(--ink-3)' }}
                >
                  <Pencil size={12} />
                </button>
                <button
                  type="button"
                  title={t('account.delete')}
                  aria-label={t('account.delete')}
                  onClick={(e) => {
                    e.stopPropagation()
                    onDeleteAccount(acc)
                  }}
                  className="rounded p-1 hover:bg-[var(--bg-hover)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  style={{ color: 'var(--ink-3)' }}
                >
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
            {acc.id === activeAccountId &&
              folders
                .filter((f) => f.selectable)
                .map((f) => {
                  const Icon: LucideIcon = folderIcon[f.type] ?? FolderIcon
                  const label =
                    f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
                  return (
                    <button
                      key={f.id}
                      type="button"
                      onClick={() => onSelectFolder(f.id)}
                      className="flex w-full items-center gap-2 rounded-md py-1.5 pl-7 pr-2 text-[13px]"
                      style={{
                        color: f.id === activeFolderId ? 'var(--ink)' : 'var(--ink-2)',
                        background:
                          f.id === activeFolderId ? 'var(--accent-wash)' : 'transparent',
                      }}
                    >
                      <Icon size={14} style={{ color: 'var(--ink-3)' }} />
                      <span className="flex-1 truncate text-left">{label}</span>
                      {f.unread_count > 0 && (
                        <span className="text-[10.5px]" style={{ color: 'var(--ink-3)' }}>
                          {f.unread_count}
                        </span>
                      )}
                    </button>
                  )
                })}
          </div>
        ))}
      </div>
    </div>
  )
}

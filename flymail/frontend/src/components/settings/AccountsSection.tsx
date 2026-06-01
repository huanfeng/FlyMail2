import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, Pencil, Trash2, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { AccountDialog } from '@/components/mail/AccountDialog'
import {
  useAccounts,
  useDeleteAccount,
  useSetAccountEnabled,
  useAccountStats,
  useTriggerSync,
  useSyncStatus,
} from '@/lib/queries'
import type { Account, SyncPhase } from '@/lib/types'

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

function formatLastSync(lastSyncAt: string | undefined, never: string): string {
  if (!lastSyncAt) return never
  try {
    return new Date(lastSyncAt).toLocaleString()
  } catch {
    return lastSyncAt
  }
}

// ────────────────────────────────────────────────────────────────────────────────
// AccountRow
// ────────────────────────────────────────────────────────────────────────────────

interface AccountRowProps {
  account: Account
  onEdit: (account: Account) => void
  onDelete: (account: Account) => void
  onToggleEnabled: (account: Account) => void
}

function AccountRow({ account, onEdit, onDelete, onToggleEnabled }: AccountRowProps) {
  const { t } = useTranslation()
  const [syncing, setSyncing] = React.useState(false)

  const statsQuery = useAccountStats(account.id)
  const triggerSync = useTriggerSync()
  const syncStatusQuery = useSyncStatus(account.id, syncing)

  // 监听同步状态，完成或失败后停止轮询
  const syncPhase: SyncPhase = syncStatusQuery.data?.phase ?? 'none'
  React.useEffect(() => {
    if (syncing && (syncPhase === 'done' || syncPhase === 'error')) {
      setSyncing(false)
    }
  }, [syncing, syncPhase])

  function handleSync() {
    setSyncing(true)
    triggerSync.mutate(account.id)
  }

  // 同步进度文本
  function getSyncLabel(): string {
    if (!syncing) return t('settings.account.sync')
    switch (syncPhase) {
      case 'folders':
        return t('sync.folders')
      case 'messages':
        return t('sync.messages')
      case 'done':
        return t('settings.account.syncDone')
      case 'error':
        return t('settings.account.syncError')
      default:
        return t('settings.account.syncing')
    }
  }

  const stats = statsQuery.data
  const lastSync = formatLastSync(account.last_sync_at, t('settings.account.never'))

  return (
    <div
      className="rounded-lg p-4 flex flex-col gap-3"
      style={{ background: 'var(--surface)', border: '1px solid var(--rule)' }}
    >
      {/* 顶行：名称 + 邮箱 + 状态徽标 */}
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="font-medium text-sm truncate" style={{ color: 'var(--ink)' }}>
            {account.name}
          </span>
          <span className="text-xs truncate" style={{ color: 'var(--ink-3)' }}>
            {account.email}
          </span>
        </div>
        {/* 状态徽标 */}
        <span
          className="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
          style={{
            background: account.enabled ? 'var(--accent-wash)' : 'var(--bg)',
            color: account.enabled ? 'var(--accent-color)' : 'var(--ink-3)',
            border: '1px solid',
            borderColor: account.enabled ? 'var(--accent-color)' : 'var(--rule)',
          }}
        >
          {account.enabled ? t('settings.account.enabled') : t('settings.account.disabled')}
        </span>
      </div>

      {/* 元信息行：最后同步 + 邮件数 + 文件夹数 */}
      <div className="flex flex-wrap items-center gap-3 text-xs" style={{ color: 'var(--ink-3)' }}>
        <span>
          {t('settings.account.lastSync')}：{lastSync}
        </span>
        <span>
          {t('settings.account.messages')}：{stats ? stats.message_count : '…'}
        </span>
        <span>
          {t('settings.account.folders')}：{stats ? stats.folder_count : '…'}
        </span>
      </div>

      {/* 同步进度提示（仅同步中显示） */}
      {syncing && (
        <div
          className="rounded-md px-3 py-1.5 text-xs flex items-center gap-2"
          style={{ background: 'var(--accent-wash)', color: 'var(--accent-color)' }}
        >
          <RefreshCw size={12} className="animate-spin shrink-0" />
          {getSyncLabel()}
          {syncStatusQuery.data?.total != null && syncStatusQuery.data.processed != null && (
            <span style={{ color: 'var(--ink-2)' }}>
              ({syncStatusQuery.data.processed} / {syncStatusQuery.data.total})
            </span>
          )}
        </div>
      )}

      {/* 操作按钮行 */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* 立即同步 */}
        <Button
          variant="outline"
          size="sm"
          onClick={handleSync}
          disabled={syncing || triggerSync.isPending}
          className="flex items-center gap-1.5"
        >
          <RefreshCw size={13} className={syncing ? 'animate-spin' : ''} />
          {getSyncLabel()}
        </Button>

        {/* 启用 / 停用 */}
        <Button
          variant="outline"
          size="sm"
          onClick={() => onToggleEnabled(account)}
        >
          {account.enabled ? t('settings.account.disable') : t('settings.account.enable')}
        </Button>

        {/* 编辑 */}
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onEdit(account)}
          className="flex items-center gap-1.5"
          aria-label={t('settings.account.edit')}
        >
          <Pencil size={13} />
          {t('settings.account.edit')}
        </Button>

        {/* 删除 */}
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(account)}
          className="flex items-center gap-1.5"
          style={{ color: 'var(--destructive)' }}
          aria-label={t('settings.account.delete')}
        >
          <Trash2 size={13} />
          {t('settings.account.delete')}
        </Button>
      </div>
    </div>
  )
}

// ────────────────────────────────────────────────────────────────────────────────
// AccountsSection
// ────────────────────────────────────────────────────────────────────────────────

export function AccountsSection() {
  const { t } = useTranslation()

  // Dialog state
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<Account | null>(null)

  // Queries & mutations
  const accountsQuery = useAccounts()
  const deleteAccount = useDeleteAccount()
  const setAccountEnabled = useSetAccountEnabled()

  const accounts: Account[] = accountsQuery.data ?? []

  function handleAdd() {
    setEditingAccount(null)
    setDialogOpen(true)
  }

  function handleEdit(account: Account) {
    setEditingAccount(account)
    setDialogOpen(true)
  }

  function handleDelete(account: Account) {
    const confirmed = window.confirm(t('settings.account.deleteConfirm'))
    if (confirmed) {
      deleteAccount.mutate(account.id)
    }
  }

  function handleToggleEnabled(account: Account) {
    setAccountEnabled.mutate({ id: account.id, enabled: !account.enabled })
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      {/* 顶部操作栏 */}
      <div className="flex items-center justify-between">
        <Button
          size="sm"
          onClick={handleAdd}
          className="flex items-center gap-1.5"
        >
          <Plus size={14} />
          {t('settings.account.add')}
        </Button>
      </div>

      {/* 账户列表 */}
      {accounts.length === 0 ? (
        <div
          className="rounded-lg px-4 py-8 text-center text-sm"
          style={{ color: 'var(--ink-3)', border: '1px dashed var(--rule)' }}
        >
          {t('settings.account.none')}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {accounts.map((account) => (
            <AccountRow
              key={account.id}
              account={account}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onToggleEnabled={handleToggleEnabled}
            />
          ))}
        </div>
      )}

      {/* 账户对话框（添加/编辑） */}
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />
    </div>
  )
}

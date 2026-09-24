// 设置 → 账户：账户列表、排序、启停、同步，以及配置导入导出。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { PortabilityDialog } from '@/components/settings/PortabilityDialog'
import { useAccountSync } from '@/hooks/useAccountSync'
import {
  useAccounts,
  useAccountStats,
  useDeleteAccount,
  useReorderAccounts,
  useSetAccountEnabled,
} from '@/lib/queries'
import { moveItem } from '@/lib/reorder'
import type { Account } from '@/lib/types'
import { Toggle } from '../controls'

// ════════════════════════════════════════════════════════════
// 子组件：单个账户行（用于整页账户分区）
// ════════════════════════════════════════════════════════════

interface AccountCardRowProps {
  account: Account
  onEdit: () => void
  onDelete: () => void
  /** 上移/下移一位；为 null 表示已在首/末位，按钮置灰 */
  onMoveUp: (() => void) | null
  onMoveDown: (() => void) | null
}

function AccountCardRow({ account, onEdit, onDelete, onMoveUp, onMoveDown }: AccountCardRowProps) {
  const { t } = useTranslation()
  const setEnabled = useSetAccountEnabled()
  const statsQuery = useAccountStats(account.id)
  // 与侧栏那个同步按钮共用同一套触发/轮询/收手逻辑，见 useAccountSync 的头注释。
  // 原先这里是自己写的第二份，同样踩了「上一轮遗留的 done 让第二次点击直接空转」
  // 和「触发失败照样开轮询」两个坑。
  const accountSync = useAccountSync()
  const syncing = accountSync.syncing && accountSync.accountId === account.id

  function handleSync() {
    accountSync.start(account.id)
  }

  function handleToggle() {
    setEnabled.mutate({ id: account.id, enabled: !account.enabled })
  }

  /** 从名称取首字母（最多 2 个） */
  function initials(name: string): string {
    return name
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0] ?? '')
      .join('')
      .toUpperCase()
  }

  const stats = statsQuery.data

  return (
    <div className="account-card">
      {/* 头像 */}
      <div
        className="ac-avatar"
        style={{ background: 'var(--accent)' }}
        aria-hidden="true"
      >
        {initials(account.name || account.email)}
      </div>

      {/* 名称 + 邮箱 */}
      <div style={{ minWidth: 0 }}>
        <div className="ac-name">{account.name || account.email}</div>
        <div className="ac-mail">{account.email}</div>
        {stats && (
          <div style={{ fontSize: 11, color: 'var(--ink-3)', marginTop: 3, fontFamily: 'var(--font-mono)' }}>
            {t('settings.account.messages')}: {stats.message_count}
            &nbsp;·&nbsp;
            {t('settings.account.folders')}: {stats.folder_count}
          </div>
        )}
      </div>

      {/* 右侧操作区 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
        {/* 排序：上移 / 下移。
            首行的上移与末行的下移置灰而不是隐藏——隐藏会让按钮列
            在不同行之间错位，眼睛得重新找一次「下移」在哪。 */}
        <div className="ac-reorder">
          <button
            type="button"
            className="icon-btn"
            onClick={() => onMoveUp?.()}
            disabled={onMoveUp == null}
            title={t('settings.account.moveUp')}
            aria-label={t('settings.account.moveUp')}
          >
            <Icon name="chevron-up" size={13} />
          </button>
          <button
            type="button"
            className="icon-btn"
            onClick={() => onMoveDown?.()}
            disabled={onMoveDown == null}
            title={t('settings.account.moveDown')}
            aria-label={t('settings.account.moveDown')}
          >
            <Icon name="chevron-down" size={13} />
          </button>
        </div>

        {/* 状态徽标 */}
        <span
          className={'ac-status' + (account.enabled ? ' live' : '')}
        >
          {account.enabled ? t('settings.account.enabled') : t('settings.account.disabled')}
        </span>

        {/* 启/停 toggle */}
        <Toggle
          on={account.enabled}
          onChange={handleToggle}
          ariaLabel={account.enabled ? t('settings.account.disable') : t('settings.account.enable')}
        />

        {/* 立即同步 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.sync')}
          aria-label={t('settings.account.sync')}
          onClick={handleSync}
          disabled={syncing || !account.enabled}
        >
          <Icon
            name="circle-dot"
            size={13}
            className={syncing ? 'spin-anim' : undefined}
          />
        </button>

        {/* 编辑 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.edit')}
          aria-label={t('settings.account.edit')}
          onClick={onEdit}
        >
          <Icon name="compose" size={13} />
        </button>

        {/* 删除 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.delete')}
          aria-label={t('settings.account.delete')}
          onClick={onDelete}
          style={{ color: 'var(--destructive)' }}
        >
          <Icon name="trash" size={13} />
        </button>
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：账户分区
// ════════════════════════════════════════════════════════════

export function AccountsSection() {
  const { t } = useTranslation()
  const confirm = useConfirm()
  const { toast } = useToast()
  const { data: accounts = [] } = useAccounts()
  const deleteAccount = useDeleteAccount()
  const reorder = useReorderAccounts()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<Account | null>(null)
  // null = 未打开；'export' / 'import' 决定进哪一半。
  // 两个动作共用一个对话框：它们的账户勾选列表与交互形态一致，
  // 拆成两个组件会把那份列表抄两遍。
  const [portable, setPortable] = React.useState<'export' | 'import' | null>(null)

  function handleAdd() {
    setEditingAccount(null)
    setDialogOpen(true)
  }

  function handleEdit(account: Account) {
    setEditingAccount(account)
    setDialogOpen(true)
  }

  function handleMove(index: number, delta: number) {
    const next = moveItem(accounts, index, delta)
    // moveItem 越界时原样返回入参，此时一个请求都不该发
    if (next === accounts) return
    // 失败时 onSettled 的重取会把列表弹回原位。不说一声的话，
    // 用户看到的就是「点了箭头没反应」，只会以为按钮坏了。
    reorder.mutate(next.map((a) => a.id), {
      onError: () => toast(t('settings.account.reorderFailed')),
    })
  }

  async function handleDelete(account: Account) {
    // 文案与侧栏那处合并到 account.*：两处说的是同一件事，此前是一字不差的两份
    const ok = await confirm({
      title: t('account.deleteConfirm'),
      body: t('account.deleteConfirmBody'),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    deleteAccount.mutate(account.id)
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.navAccounts')}</h3>
      <p className="help">{t('settings.page.accountsHelp')}</p>

      {/* 账户卡片列表 */}
      {accounts.length === 0 ? (
        <div style={{ color: 'var(--ink-3)', fontSize: 13, padding: '12px 0' }}>
          {t('settings.account.none')}
        </div>
      ) : (
        accounts.map((account, i) => (
          <AccountCardRow
            key={account.id}
            account={account}
            onEdit={() => handleEdit(account)}
            onDelete={() => handleDelete(account)}
            onMoveUp={i > 0 ? () => handleMove(i, -1) : null}
            onMoveDown={i < accounts.length - 1 ? () => handleMove(i, 1) : null}
          />
        ))
      )}

      {/* 账户操作：添加 / 导出 / 导入。
          三个按钮同排：它们都是"对账户整体做点什么"，分开放会让用户
          以为导出属于某一个账户。 */}
      <div className="settings-actions">
        <button type="button" className="pill-btn" onClick={handleAdd}>
          <Icon name="plus" size={12} />
          {t('settings.account.add')}
        </button>
        <button
          type="button"
          className="pill-btn"
          onClick={() => setPortable('export')}
          disabled={accounts.length === 0}
        >
          <Icon name="archive" size={12} />
          {t('settings.portable.exportTitle')}
        </button>
        <button type="button" className="pill-btn" onClick={() => setPortable('import')}>
          <Icon name="cloud" size={12} />
          {t('settings.portable.importTitle')}
        </button>
      </div>

      {/* AccountDialog 复用 */}
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />

      {/* 打开时才挂载：对话框内部的初始状态因此每次都是干净的，
          不需要一个「open 变真就重置一遍」的 effect（那会在 effect 里
          同步 setState、触发级联渲染）。 */}
      {portable != null && (
        <PortabilityDialog
          open
          mode={portable}
          accounts={accounts}
          onOpenChange={(o) => { if (!o) setPortable(null) }}
        />
      )}
    </div>
  )
}

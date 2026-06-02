import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { AppLayout } from '@/components/mail/AppLayout'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { MailList } from '@/components/mail/MailList'
import { DraftsList } from '@/components/mail/DraftsList'
import { Reader } from '@/components/mail/Reader'
import { ComposeDialog } from '@/components/mail/ComposeDialog'
import type { ComposeInitial } from '@/components/mail/ComposeDialog'
import { buildReply, buildForward } from '@/lib/compose-prefill'
import {
  useAccounts,
  useFolders,
  useInfiniteMessages,
  useSyncStatus,
  useTriggerSync,
  useDeleteAccount,
  useMarkRead,
  useToggleFlag,
} from '@/lib/queries'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import { getListStyle, setListStyle } from '@/lib/list-prefs'
import type { ListStyle } from '@/lib/list-prefs'
import type { Account, Draft, MessageDetail } from '@/lib/types'

export function ShellPage() {
  // 注意：GET /folders/:fid/messages 不绑定 account，依赖单管理员假设；
  // 未来支持多用户时需补 ownership 校验。

  // 订阅 SSE 实时推送，新邮件到达时自动刷新 folders/messages 缓存
  useRealtimeSync()

  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const { t } = useTranslation()
  const accountId = params.get('account') ? Number(params.get('account')) : null
  const folderId = params.get('folder') ? Number(params.get('folder')) : null
  const messageId = params.get('message') ? Number(params.get('message')) : null

  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders(accountId)
  const {
    data: messagesData,
    isLoading: messagesLoading,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useInfiniteMessages(folderId)

  // 将分页数据展平为平坦列表
  const messages = messagesData?.pages.flat() ?? []

  // 列表样式偏好（持久化到 localStorage）
  const [listStyle, setListStyleState] = useState<ListStyle>(() => getListStyle())

  function handleChangeListStyle(style: ListStyle) {
    setListStyle(style)
    setListStyleState(style)
  }

  const markRead = useMarkRead()
  const toggleFlag = useToggleFlag()

  // 打开未读邮件时自动标已读（对展平后的 messages 生效）
  useEffect(() => {
    if (messageId == null) return
    const m = messages.find((x) => x.id === messageId)
    if (m && !m.seen) {
      markRead.mutate({ id: messageId, read: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messageId, messages])

  const [syncEnabled, setSyncEnabled] = useState(false)
  const { data: syncStatus } = useSyncStatus(accountId, syncEnabled)
  const triggerSync = useTriggerSync()
  const syncing = syncStatus?.phase === 'folders' || syncStatus?.phase === 'messages'

  // ── 账户对话框 state ─────────────────────────────────────────────────────────
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)
  const deleteAccount = useDeleteAccount()

  // ── 设置对话框 state ─────────────────────────────────────────────────────────
  const [settingsOpen, setSettingsOpen] = useState(false)

  // ── 中栏视图 state ───────────────────────────────────────────────────────────
  const [view, setView] = useState<'messages' | 'drafts'>('messages')

  // ── 撰写/回复/转发 state ──────────────────────────────────────────────────────
  const [composeOpen, setComposeOpen] = useState(false)
  const [composeInitial, setComposeInitial] = useState<ComposeInitial | undefined>(undefined)
  const [composeDraftId, setComposeDraftId] = useState<number | null>(null)

  function setParam(mut: (p: URLSearchParams) => void, replace = false) {
    const next = new URLSearchParams(params)
    mut(next)
    setParams(next, { replace })
  }

  useEffect(() => {
    if (accountId == null && accounts.length > 0) {
      setParam((p) => p.set('account', String(accounts[0].id)), true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId, accounts])

  useEffect(() => {
    if (syncStatus?.phase === 'done' || syncStatus?.phase === 'error') {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSyncEnabled(false)
    }
    // 同步抓完后刷新文件夹（未读数/新文件夹）和邮件列表缓存。
    if (syncStatus?.phase === 'done') {
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [syncStatus?.phase])

  const activeFolder = useMemo(
    () => folders.find((f) => f.id === folderId) ?? null,
    [folders, folderId],
  )

  function selectAccount(id: number) {
    setView('messages')
    setParam((p) => {
      p.set('account', String(id))
      p.delete('folder')
      p.delete('message')
    })
  }

  function selectFolder(id: number) {
    setView('messages')
    setParam((p) => {
      p.set('folder', String(id))
      p.delete('message')
    })
  }

  function selectMessage(id: number) {
    setParam((p) => p.set('message', String(id)))
  }

  function onSync(id: number) {
    setSyncEnabled(true)
    triggerSync.mutate(id)
  }

  // ── 账户增删改回调 ────────────────────────────────────────────────────────────

  function onAddAccount() {
    setEditingAccount(null)
    setDialogOpen(true)
  }

  function onEditAccount(a: Account) {
    setEditingAccount(a)
    setDialogOpen(true)
  }

  function onReply(d: MessageDetail) {
    setComposeInitial(buildReply(d))
    setComposeDraftId(null)
    setComposeOpen(true)
  }

  function onForward(d: MessageDetail) {
    setComposeInitial(buildForward(d))
    setComposeDraftId(null)
    setComposeOpen(true)
  }

  function onCompose() {
    setComposeInitial(undefined)
    setComposeDraftId(null)
    setComposeOpen(true)
  }

  function onOpenDrafts(accountId: number) {
    setParam((p) => p.set('account', String(accountId)), true)
    setView('drafts')
  }

  function openDraft(d: Draft) {
    setComposeInitial({
      to: d.to,
      cc: d.cc,
      subject: d.subject,
      bodyHtml: d.body_html,
      inReplyTo: d.in_reply_to || undefined,
      references: d.references || undefined,
    })
    setComposeDraftId(d.id)
    setComposeOpen(true)
  }

  function onDeleteAccount(a: Account) {
    if (window.confirm(t('account.deleteConfirm'))) {
      deleteAccount.mutate(a.id)
      if (a.id === accountId) {
        setParam((p) => {
          p.delete('account')
          p.delete('folder')
          p.delete('message')
        })
      }
    }
  }

  return (
    <>
      <AppLayout
        sidebar={
          <AccountSidebar
            accounts={accounts}
            folders={folders}
            activeAccountId={accountId}
            activeFolderId={folderId}
            syncing={syncing}
            onSelectAccount={selectAccount}
            onSelectFolder={selectFolder}
            onSync={onSync}
            onAddAccount={onAddAccount}
            onEditAccount={onEditAccount}
            onDeleteAccount={onDeleteAccount}
            onOpenSettings={() => setSettingsOpen(true)}
            onCompose={onCompose}
            onOpenDrafts={onOpenDrafts}
          />
        }
        list={
          view === 'drafts' && accountId != null ? (
            <DraftsList accountId={accountId} onOpenDraft={openDraft} />
          ) : (
            <MailList
              folder={activeFolder}
              messages={messages}
              loading={messagesLoading}
              activeMessageId={messageId}
              onSelectMessage={selectMessage}
              onToggleFlag={(id, flagged) => toggleFlag.mutate({ id, flagged })}
              listStyle={listStyle}
              hasNextPage={hasNextPage ?? false}
              isFetchingNextPage={isFetchingNextPage}
              onLoadMore={() => { void fetchNextPage() }}
            />
          )
        }
        reader={
          <Reader
            messageId={messageId}
            onReply={onReply}
            onForward={onForward}
          />
        }
      />
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />
      <SettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        listStyle={listStyle}
        onChangeListStyle={handleChangeListStyle}
      />
      <ComposeDialog
        open={composeOpen}
        onOpenChange={setComposeOpen}
        accountId={accountId}
        initial={composeInitial}
        draftId={composeDraftId}
      />
    </>
  )
}

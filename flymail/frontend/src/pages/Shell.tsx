import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { AppLayout } from '@/components/mail/AppLayout'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import type { AppView } from '@/components/mail/AccountSidebar'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { NotificationsPage } from '@/components/notifications/NotificationsPage'
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
  useInfiniteAggregate,
  useAggregateCounts,
  useMessageDetail,
  useSyncStatus,
  useTriggerSync,
  useMarkRead,
  useToggleFlag,
} from '@/lib/queries'
import type { AggregateView } from '@/lib/queries'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'
import { getListStyle, setListStyle } from '@/lib/list-prefs'
import type { ListStyle } from '@/lib/list-prefs'
import type { Account, Draft, MessageDetail } from '@/lib/types'

/** 校验 URL 中的 agg 参数是否为合法聚合视图 */
function parseAgg(v: string | null): AggregateView | null {
  return v === 'inbox' || v === 'unread' || v === 'starred' ? v : null
}

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
  // 聚合视图（跨所有账户）：inbox / unread / starred；非聚合时为 null
  const agg = parseAgg(params.get('agg'))

  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders(accountId)

  // 单文件夹列表（agg 激活时禁用）
  const folderInfinite = useInfiniteMessages(agg ? null : folderId)
  // 跨账户聚合列表（仅 agg 激活时启用）
  const aggInfinite = useInfiniteAggregate(agg)
  // 聚合入口徽标计数
  const { data: aggCounts = { inbox: 0, unread: 0, starred: 0 } } = useAggregateCounts()

  // 当前生效的数据源元信息
  const messages = agg
    ? (aggInfinite.data?.pages.flatMap((p) => p.messages) ?? [])
    : (folderInfinite.data?.pages.flat() ?? [])
  const messagesLoading = agg ? aggInfinite.isLoading : folderInfinite.isLoading
  const hasNextPage = (agg ? aggInfinite.hasNextPage : folderInfinite.hasNextPage) ?? false
  const isFetchingNextPage = agg ? aggInfinite.isFetchingNextPage : folderInfinite.isFetchingNextPage
  function loadMore() {
    if (agg) void aggInfinite.fetchNextPage()
    else void folderInfinite.fetchNextPage()
  }

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

  // ── 账户对话框 state（新增账户用）─────────────────────────────────────────────
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)

  // ── 第三栏视图 state（邮件 / 通知）────────────────────────────────────────────
  const [appView, setAppView] = useState<AppView>('mail')
  // ── 设置浮层 state ────────────────────────────────────────────────────────────
  const [settingsOpen, setSettingsOpen] = useState(false)

  // ── 中栏视图 state（邮件视图内部：messages / drafts）───────────────────────────
  const [view, setView] = useState<'messages' | 'drafts'>('messages')

  // ── 撰写/回复/转发 state ──────────────────────────────────────────────────────
  const [composeOpen, setComposeOpen] = useState(false)
  const [composeInitial, setComposeInitial] = useState<ComposeInitial | undefined>(undefined)
  const [composeDraftId, setComposeDraftId] = useState<number | null>(null)

  // 快捷键 r 回复时需要当前邮件的完整数据；Reader 已请求过，此处只复用缓存
  const { data: activeMessageDetail } = useMessageDetail(messageId)

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
    // 同步抓完后刷新文件夹（未读数/新文件夹）、邮件列表与聚合计数缓存。
    if (syncStatus?.phase === 'done') {
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [syncStatus?.phase])

  const activeFolder = useMemo(
    () => folders.find((f) => f.id === folderId) ?? null,
    [folders, folderId],
  )

  function selectAccount(id: number) {
    setView('messages')
    setAppView('mail')
    setSettingsOpen(false)
    setParam((p) => {
      p.set('account', String(id))
      p.delete('folder')
      p.delete('message')
      p.delete('agg')
    })
  }

  function selectFolder(id: number) {
    setView('messages')
    setAppView('mail')
    setSettingsOpen(false)
    setParam((p) => {
      p.set('folder', String(id))
      p.delete('message')
      p.delete('agg')
    })
  }

  // 选择聚合入口（跨所有账户）
  function selectAggregate(v: AggregateView) {
    setView('messages')
    setAppView('mail')
    setSettingsOpen(false)
    setParam((p) => {
      p.set('agg', v)
      p.delete('folder')
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

  function onAddAccount() {
    setEditingAccount(null)
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

  // ── 全局键盘快捷键 ────────────────────────────────────────────────────────────
  useKeyboardShortcuts({
    onCompose,
    // 仅当有选中邮件且其详情已缓存时才允许快捷键回复
    onReply: activeMessageDetail != null ? () => onReply(activeMessageDetail) : null,
    messages,
    activeMessageId: messageId,
    selectMessage,
    onCloseCompose: () => setComposeOpen(false),
    composeOpen,
  })

  function onOpenDrafts(accId: number) {
    setParam((p) => p.set('account', String(accId)), true)
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

  // ── 聚合视图标题/副标题 ───────────────────────────────────────────────────────
  const aggLabelKey: Record<AggregateView, string> = {
    inbox: 'sidebar.allInboxes',
    unread: 'sidebar.allUnread',
    starred: 'sidebar.starred',
  }
  const aggTitle = agg ? t(aggLabelKey[agg]) : undefined
  const aggSubtitle = agg ? t('list.totalCount', { count: messages.length }) : undefined

  // ── Sidebar（常驻所有视图）────────────────────────────────────────────────────
  const sidebar = (
    <AccountSidebar
      accounts={accounts}
      folders={folders}
      activeAccountId={accountId}
      activeFolderId={folderId}
      syncing={syncing}
      activeView={appView}
      settingsOpen={settingsOpen}
      activeAgg={agg}
      aggCounts={aggCounts}
      onSelectAccount={selectAccount}
      onSelectFolder={selectFolder}
      onSelectAggregate={selectAggregate}
      onSync={onSync}
      onAddAccount={onAddAccount}
      onSetView={setAppView}
      onToggleSettings={() => setSettingsOpen((o) => !o)}
      onCompose={onCompose}
      onOpenDrafts={onOpenDrafts}
    />
  )

  return (
    <>
      <AppLayout
        sidebar={sidebar}
        list={
          view === 'drafts' && accountId != null ? (
            <DraftsList accountId={accountId} onOpenDraft={openDraft} />
          ) : (
            <MailList
              // 按文件夹/聚合 + 样式重建：切换时重置滚动位置、重建虚拟列表，避免误触翻页与定位漂移。
              key={`${agg ?? folderId}-${listStyle}`}
              folder={agg ? null : activeFolder}
              titleOverride={aggTitle}
              subtitleOverride={aggSubtitle}
              messages={messages}
              loading={messagesLoading}
              activeMessageId={messageId}
              onSelectMessage={selectMessage}
              onToggleFlag={(id, flagged) => toggleFlag.mutate({ id, flagged })}
              listStyle={listStyle}
              hasNextPage={hasNextPage}
              isFetchingNextPage={isFetchingNextPage}
              onLoadMore={loadMore}
            />
          )
        }
        reader={
          appView === 'notif' ? (
            <NotificationsPage onBack={() => setAppView('mail')} />
          ) : (
            <Reader
              messageId={messageId}
              onReply={onReply}
              onForward={onForward}
            />
          )
        }
      />
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />
      <ComposeDialog
        open={composeOpen}
        onOpenChange={setComposeOpen}
        accountId={accountId}
        initial={composeInitial}
        draftId={composeDraftId}
      />
      {/* 设置弹框（覆盖层 modal）*/}
      {settingsOpen && (
        <SettingsDialog
          listStyle={listStyle}
          onChangeListStyle={handleChangeListStyle}
          onClose={() => setSettingsOpen(false)}
        />
      )}
    </>
  )
}

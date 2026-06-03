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
  useInfiniteSearch,
  useAggregateCounts,
  useMessageDetail,
  useSyncStatus,
  useTriggerSync,
  useMarkRead,
  useToggleFlag,
  useBatchDelete,
  useBatchMove,
  useBatchRead,
  useBatchFlag,
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

  // 列表样式偏好（持久化到 localStorage）；提前声明供 sourceKey 使用
  const [listStyle, setListStyleState] = useState<ListStyle>(() => getListStyle())
  function handleChangeListStyle(style: ListStyle) {
    setListStyle(style)
    setListStyleState(style)
  }

  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders(accountId)

  // ── 搜索（跨账户，后端）：输入防抖 300ms 再发请求 ──────────────────────────────
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  useEffect(() => {
    const id = setTimeout(() => setDebouncedQuery(searchQuery.trim()), 300)
    return () => clearTimeout(id)
  }, [searchQuery])
  const searching = debouncedQuery.length > 0

  // 三选一数据源：搜索 > 聚合 > 单文件夹（互斥，未选中的禁用以免多余请求）
  const folderInfinite = useInfiniteMessages(searching || agg ? null : folderId)
  const aggInfinite = useInfiniteAggregate(searching ? null : agg)
  const searchInfinite = useInfiniteSearch(debouncedQuery)
  // 聚合入口徽标计数
  const { data: aggCounts = { inbox: 0, unread: 0, starred: 0 } } = useAggregateCounts()

  // 当前生效的数据源元信息
  const messages = searching
    ? (searchInfinite.data?.pages.flatMap((p) => p.messages) ?? [])
    : agg
      ? (aggInfinite.data?.pages.flatMap((p) => p.messages) ?? [])
      : (folderInfinite.data?.pages.flat() ?? [])
  const messagesLoading = searching
    ? searchInfinite.isLoading
    : agg
      ? aggInfinite.isLoading
      : folderInfinite.isLoading
  const hasNextPage = (searching
    ? searchInfinite.hasNextPage
    : agg
      ? aggInfinite.hasNextPage
      : folderInfinite.hasNextPage) ?? false
  const isFetchingNextPage = searching
    ? searchInfinite.isFetchingNextPage
    : agg
      ? aggInfinite.isFetchingNextPage
      : folderInfinite.isFetchingNextPage
  function loadMore() {
    if (searching) void searchInfinite.fetchNextPage()
    else if (agg) void aggInfinite.fetchNextPage()
    else void folderInfinite.fetchNextPage()
  }

  // 数据源标识：搜索/聚合/文件夹 + 列表样式（驱动 MailList 滚动重置与选择清空）
  const sourceKey = `${searching ? 'search' : (agg ?? folderId)}-${listStyle}`

  // ── 批量选择 ──────────────────────────────────────────────────────────────
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  // 切换数据源/样式时清空选择，避免跨上下文误操作
  useEffect(() => {
    setSelectedIds(new Set())
  }, [sourceKey])

  function toggleSelect(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  function selectAllVisible() {
    setSelectedIds(new Set(messages.map((m) => m.id)))
  }
  function clearSelection() {
    setSelectedIds(new Set())
  }

  // 已选邮件的共同账户（跨账户为 null）；批量移动目标取该账户的文件夹
  const selectionAccountId = useMemo(() => {
    let acc: number | null = null
    for (const id of selectedIds) {
      const m = messages.find((x) => x.id === id)
      if (!m) continue
      if (acc === null) acc = m.account_id
      else if (acc !== m.account_id) return null
    }
    return acc
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedIds, messages])
  const { data: moveTargets = [] } = useFolders(selectionAccountId)

  const batchDelete = useBatchDelete()
  const batchMove = useBatchMove()
  const batchRead = useBatchRead()
  const batchFlag = useBatchFlag()

  function onBatchDelete() {
    const ids = [...selectedIds]
    if (ids.length === 0) return
    if (!window.confirm(t('list.batchDeleteConfirm', { count: ids.length }))) return
    batchDelete.mutate(ids, { onSuccess: clearSelection })
  }
  function onBatchRead(read: boolean) {
    const ids = [...selectedIds]
    if (ids.length === 0) return
    batchRead.mutate({ ids, read }, { onSuccess: clearSelection })
  }
  function onBatchFlag(flagged: boolean) {
    const ids = [...selectedIds]
    if (ids.length === 0) return
    batchFlag.mutate({ ids, flagged }, { onSuccess: clearSelection })
  }
  function onBatchMove(targetFolderId: number) {
    const ids = [...selectedIds]
    if (ids.length === 0) return
    batchMove.mutate({ ids, folderId: targetFolderId }, { onSuccess: clearSelection })
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
    setSearchQuery('')
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
    setSearchQuery('')
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
    setSearchQuery('')
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
  // 列表标题/副标题：搜索 > 聚合 > 文件夹（文件夹由 MailList 内部据 folder 计算）
  const listTitle = searching ? t('list.searchTitle') : agg ? t(aggLabelKey[agg]) : undefined
  const listSubtitle = searching || agg ? t('list.totalCount', { count: messages.length }) : undefined

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
              // 用 sourceKey（而非 React key）驱动内部滚动重置：避免重挂载打断搜索框输入焦点。
              // 搜索态固定为 'search'（不随关键词变），切文件夹/聚合/样式时重置滚动。
              sourceKey={sourceKey}
              folder={searching || agg ? null : activeFolder}
              titleOverride={listTitle}
              subtitleOverride={listSubtitle}
              messages={messages}
              loading={messagesLoading}
              activeMessageId={messageId}
              onSelectMessage={selectMessage}
              onToggleFlag={(id, flagged) => toggleFlag.mutate({ id, flagged })}
              listStyle={listStyle}
              hasNextPage={hasNextPage}
              isFetchingNextPage={isFetchingNextPage}
              onLoadMore={loadMore}
              searchValue={searchQuery}
              onSearchChange={setSearchQuery}
              selectedIds={selectedIds}
              onToggleSelect={toggleSelect}
              onSelectAllVisible={selectAllVisible}
              onClearSelection={clearSelection}
              onBatchRead={onBatchRead}
              onBatchFlag={onBatchFlag}
              onBatchDelete={onBatchDelete}
              onBatchMove={onBatchMove}
              moveTargets={moveTargets}
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
              onClose={() => setParam((p) => p.delete('message'))}
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

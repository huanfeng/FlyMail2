import { useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { AppLayout } from '@/components/mail/AppLayout'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { NotificationsPage } from '@/components/notifications/NotificationsPage'
import { MailList } from '@/components/mail/MailList'
import { DraftsList } from '@/components/mail/DraftsList'
import { Reader } from '@/components/mail/Reader'
import { ThreadReader } from '@/components/mail/ThreadReader'
import { ComposeDialog } from '@/components/mail/ComposeDialog'
import type { ComposeInitial } from '@/components/mail/ComposeDialog'
import { ShortcutsCheatsheet } from '@/components/mail/ShortcutsCheatsheet'
import { useToast } from '@/components/ui/Toast'
import { buildReply, buildReplyAll, buildForward, buildMailtoCompose } from '@/lib/compose-prefill'
import {
  useAccounts,
  useFolders,
  useInfiniteMessages,
  useInfiniteAggregate,
  useInfiniteSearch,
  listTotalOf,
  useAggregateCounts,
  useMessageDetail,
  useSyncStatus,
  useTriggerSync,
  useMarkRead,
  useToggleFlag,
  useDeleteMessage,
  useMoveMessage,
  useBatchDelete,
  useBatchMove,
  useBatchRead,
  useBatchFlag,
  useInfiniteThreads,
  useInfiniteAggregateThreads,
  useInfiniteSearchThreads,
  useThreadBatchRead,
  useThreadBatchFlag,
  useThreadBatchDelete,
  useThreadBatchMove,
} from '@/lib/queries'
import type { AggregateView } from '@/lib/queries'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import { useUnreadBadge } from '@/hooks/useUnreadBadge'
import { useKeyboardShortcuts, COMPOSE_CLOSE_EVENT } from '@/hooks/useKeyboardShortcuts'
import type { GoTarget } from '@/lib/shortcuts'
import { useUndoable } from '@/hooks/useUndoable'
import {
  getListStyle,
  setListStyle,
  getAlwaysShowSelect,
  setAlwaysShowSelect,
  getConversationView,
  setConversationView,
} from '@/lib/list-prefs'
import { commonAccountId, selectedThreads } from '@/lib/thread-format'
import { accountColorMap } from '@/lib/account-color'
import type { ListStyle } from '@/lib/list-prefs'
import { EMPTY_FILTER, filterKey, isFilterActive, toggleFilter } from '@/lib/list-filters'
import type { FilterKey, ListFilter } from '@/lib/list-filters'
import { getLayoutMode, setLayoutMode } from '@/lib/layout-mode'
import { createAutoReadGate } from '@/lib/list-guards'
import type { LayoutMode } from '@/lib/layout-mode'
import api from '@/lib/api'
import type { Account, Draft, Folder, MessageDetail, Notification, ThreadListItem } from '@/lib/types'

/**
 * 撤销窗口。删除/归档/移动的请求挂起这么久才真正发出。
 *
 * 5 秒是主流客户端的取值：短于此来不及看清提示，长于此则「已删除」的状态
 * 悬空太久——切文件夹、关窗口都会强制落地，窗口越长越容易撞上这些边界。
 */
const UNDO_WINDOW_MS = 5000

/** 校验 URL 中的 agg 参数是否为合法聚合视图 */
function parseAgg(v: string | null): AggregateView | null {
  return v === 'inbox' || v === 'unread' || v === 'starred' ? v : null
}

export function ShellPage() {
  // 注意：GET /folders/:fid/messages 不绑定 account，依赖单管理员假设；
  // 未来支持多用户时需补 ownership 校验。

  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const { t } = useTranslation()
  const accountId = params.get('account') ? Number(params.get('account')) : null
  const folderId = params.get('folder') ? Number(params.get('folder')) : null
  const messageId = params.get('message') ? Number(params.get('message')) : null
  // 会话视图下第三栏由 thread 参数驱动（thread_id 是字符串，不能复用 message）
  const threadId = params.get('thread')
  // 聚合视图（跨所有账户）：inbox / unread / starred；非聚合时为 null
  const agg = parseAgg(params.get('agg'))

  // 列表样式偏好（持久化到 localStorage）；提前声明供 sourceKey 使用
  const [listStyle, setListStyleState] = useState<ListStyle>(() => getListStyle())
  function handleChangeListStyle(style: ListStyle) {
    setListStyle(style)
    setListStyleState(style)
  }

  // 行内选择框是否常显（持久化偏好，见 list-prefs）
  const [alwaysShowSelect, setAlwaysShowSelectState] = useState<boolean>(() => getAlwaysShowSelect())
  function handleChangeAlwaysShowSelect(on: boolean) {
    setAlwaysShowSelect(on)
    setAlwaysShowSelectState(on)
  }

  // 会话视图偏好（会话折叠 vs 单封列表）；关闭后一切回到既有单封行为
  const [conversationView, setConversationViewState] = useState<boolean>(() => getConversationView())
  function handleChangeConversationView(on: boolean) {
    setConversationView(on)
    setConversationViewState(on)
    // 两种模式的第三栏参数不通用：切换时把当前选中项清掉，
    // 否则会留下一个另一套视图根本解释不了的 message/thread 参数。
    setParam((p) => { p.delete('message'); p.delete('thread') })
  }

  // 布局模式偏好（三栏 / 双栏浮动阅读）
  const [layoutMode, setLayoutModeState] = useState<LayoutMode>(() => getLayoutMode())
  function handleChangeLayoutMode(mode: LayoutMode) {
    setLayoutMode(mode)
    setLayoutModeState(mode)
  }

  // 侧栏的两条主链路都要往下传 error：请求失败时 data 回落成空数组，
  // 界面与「一个账户都没有」「这个账户没有文件夹」完全同形——
  // 用户看到的是一个空侧栏，既判断不出是故障也没有重试的入口。
  const accountsQuery = useAccounts()
  const foldersQuery = useFolders(accountId)
  // useMemo 固定住空数组的引用：请求未完成时每渲染新建一个 []，
  // 会让下游依赖它的 useMemo 每渲染必重算。
  const accountsData = accountsQuery.data
  const accounts = useMemo(() => accountsData ?? [], [accountsData])
  const foldersData = foldersQuery.data
  const folders = useMemo(() => foldersData ?? [], [foldersData])
  // 同样只认「没有数据可显示」的失败。useFolders 带 30 秒轮询、账户增删会
  // invalidate ['accounts']——用裸 error 的话，一次后台重取失败就会在一份
  // 完整的列表上方挂一行「加载失败」，直到下次轮询成功才消失。
  // 重取在途时先不报错：gcTime 内切回一个上次取数失败过的账户，
  // refetch-on-mount 还没落地就先闪一下红色错误行，自愈之后又消失。
  const accountsError =
    accountsQuery.isLoadingError && !accountsQuery.isFetching ? accountsQuery.error : null
  const foldersError =
    foldersQuery.isLoadingError && !foldersQuery.isFetching ? foldersQuery.error : null
  // 账户识别色：聚合/搜索视图把多个账户的邮件混在一列里，
  // 全用同一个 accent 底色就看不出哪封属于哪个邮箱。
  const acctColors = useMemo(() => accountColorMap(accounts), [accounts])

  // 本人邮箱集合：会话行头像要避开自己（自己发起的讨论不该显示自己的首字母）
  const selfAddrs = useMemo(
    () => new Set(accounts.map((a) => a.email.trim().toLowerCase()).filter(Boolean)),
    [accounts],
  )

  // ── 搜索（跨账户，后端）：输入防抖 300ms 再发请求 ──────────────────────────────
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  useEffect(() => {
    const id = setTimeout(() => setDebouncedQuery(searchQuery.trim()), 300)
    return () => clearTimeout(id)
  }, [searchQuery])
  const searching = debouncedQuery.length > 0

  // ── 列表筛选（未读 / 星标 / 有附件，可叠加）────────────────────────────────────
  // 状态提在这里而不是 MailList 内部：它要进三个 query 的 key 与请求参数，
  // 由后端而非前端分页做筛选（否则「未读」只筛已加载的那 50 封）。
  //
  // 切换文件夹/聚合/搜索时**不重置**：「我现在只想看未读」是跨文件夹成立的浏览意图，
  // 且 chip 常驻在列表正上方且有激活态，不会出现「列表空了却不知为何」。
  const [filter, setFilter] = useState<ListFilter>(EMPTY_FILTER)
  function onToggleFilter(key: FilterKey) {
    setFilter((f) => toggleFilter(f, key))
  }

  // 三选一数据源：搜索 > 聚合 > 单文件夹（互斥，未选中的禁用以免多余请求）。
  // 会话/单封各一套三条链路，另一套由 conversationView 整体禁掉——
  // 两套同时在跑意味着每次切文件夹发两份请求，其中一份的结果永远不会被渲染。
  const folderInfinite = useInfiniteMessages(conversationView || searching || agg ? null : folderId, filter)
  const aggInfinite = useInfiniteAggregate(conversationView || searching ? null : agg, filter)
  const searchInfinite = useInfiniteSearch(conversationView ? '' : debouncedQuery, filter)
  const folderThreads = useInfiniteThreads(!conversationView || searching || agg ? null : folderId, filter)
  const aggThreads = useInfiniteAggregateThreads(!conversationView || searching ? null : agg, filter)
  const searchThreads = useInfiniteSearchThreads(conversationView ? debouncedQuery : '', filter)
  // 聚合入口徽标计数
  const { data: aggCounts = { inbox: 0, unread: 0, starred: 0, inboxTotal: 0 } } = useAggregateCounts()

  // ── 新邮件提醒 ───────────────────────────────────────────────────────────────
  //
  // 读屏播报的文本。seq 是必需的：aria-live 的语义是「内容变化时播报」，
  // 连着来两封主题相同的信时文本一模一样，DOM 没变化 = 不播报。
  // 用一个随次数增减的零宽字符让文本"变"一下，对视觉与朗读都没有副作用。
  const [announce, setAnnounce] = useState<{ text: string; seq: number }>({ text: '', seq: 0 })

  // 订阅 SSE 实时推送：new_mail 刷新缓存，notify 走提醒（桌面通知 / 提示音 / 播报）
  useRealtimeSync({
    // 必须走 openMailById 而不是 selectMessage：点通知时用户多半停在别的账户、
    // 聚合视图或搜索结果里，只写 message 参数会切不过去；会话视图下更是直接一片空白。
    onOpenMessage: (id) => void openMailById(id),
    onAnnounce: (text) => setAnnounce((p) => ({ text, seq: p.seq + 1 })),
  })

  // 标签页标题与站点图标上的未读角标。不需要任何权限，也是切走之后唯一还看得见的提醒。
  useUnreadBadge(aggCounts.unread)

  // 当前生效的数据源（搜索 > 聚合 > 文件夹）。两套的分页形状不同，各取各的。
  const msgSource = searching ? searchInfinite : agg ? aggInfinite : folderInfinite
  const threadSource = searching ? searchThreads : agg ? aggThreads : folderThreads

  // ── 撤销窗口内「已消失但还没提交」的条目 ────────────────────────────────────
  //
  // 删除/归档/移动都走延迟提交（见 useUndoable）：请求在撤销窗口结束后才发出，
  // 但列表必须立刻把它们移除，否则用户看不出操作生效了。
  // 过滤放在这两个 useMemo 里，下游的 j/k 导航、上一封/下一封、全选、列表渲染
  // 就都自动跟着走，不必逐处记得排除。
  const [hiddenIds, setHiddenIds] = useState<Set<number>>(() => new Set())
  const [hiddenThreadIds, setHiddenThreadIds] = useState<Set<string>>(() => new Set())
  const undoable = useUndoable()
  // toast 与 undoable 是一对：撤销窗口的可见部分在 toast 上，
  // 强制落地时必须同时收起它，否则撤销按钮会变成哑巴。
  const { toast, dismiss: dismissToast } = useToast()

  // 两个列表都用 useMemo 固定引用：flatMap 每渲染都产出新数组，
  // 直接进 useMemo/useEffect 的依赖数组等于「每渲染必重算」。
  const msgPages = msgSource.data?.pages
  const messages = useMemo(() => {
    if (conversationView) return []
    const all = msgPages?.flatMap((p) => p.messages) ?? []
    return hiddenIds.size === 0 ? all : all.filter((m) => !hiddenIds.has(m.id))
  }, [conversationView, msgPages, hiddenIds])
  // null = 单封模式；MailList 据此决定渲染哪种行
  const threadPages = threadSource.data?.pages
  const threads: ThreadListItem[] | null = useMemo(() => {
    if (!conversationView) return null
    const all = threadPages?.flatMap((p) => p.threads) ?? []
    return hiddenThreadIds.size === 0 ? all : all.filter((th) => !hiddenThreadIds.has(th.thread_id))
  }, [conversationView, threadPages, hiddenThreadIds])
  // 会话列表的裸数组：j/k 导航与「上一条/下一条」都要按它的顺序走，
  // 声明位置必须早于快捷键 hook。
  const threadList = threads ?? []
  const messagesLoading = conversationView ? threadSource.isLoading : msgSource.isLoading
  // 错误必须与 loading 一起往下传：只传 loading 的话，请求失败时列表走的是
  // itemCount === 0 的空态，把后端故障显示成「这个文件夹里还没有邮件」。
  //
  // ⚠ 必须用 isLoadingError（= isError && 没有数据）而不是裸 error：
  // react-query 的 status 是整个 query 的，**翻页失败同样会把它置为 'error'**
  // 并填上 error（已加载的页原样保留）。传裸 error 下去，第 2 页一失败
  // 就会让整屏错误态吃掉屏幕上那 50 封邮件——比不修更糟。
  // 翻页失败由下面的 nextPageError 单独表达，落在列表底部那一行。
  // 同理，有数据时的后台重取失败（isRefetchError）也不该掀掉整个列表。
  // 这套状态位的语义有 lib/query-semantics.test.ts 固定住。
  const messagesError = conversationView
    ? (threadSource.isLoadingError ? threadSource.error : null)
    : (msgSource.isLoadingError ? msgSource.error : null)
  const refetchMessages = conversationView ? threadSource.refetch : msgSource.refetch
  const hasNextPage = (conversationView ? threadSource.hasNextPage : msgSource.hasNextPage) ?? false
  const isFetchingNextPage = conversationView
    ? threadSource.isFetchingNextPage
    : msgSource.isFetchingNextPage
  // 翻页失败与首屏失败是两回事：首屏失败整列表走错误态，翻页失败时前几页还在屏幕上，
  // 只有底部那一行能表达「后面还有，但这次没取到」。
  const nextPageError = conversationView
    ? threadSource.isFetchNextPageError
    : msgSource.isFetchNextPageError
  // 后台刷新：已有内容的前提下又在取数。三种 loading 里唯独这一种此前没有出口。
  // 触发来源核对过：无限查询链路自身既没有 refetchInterval 也没有 placeholderData，
  // 让它亮起来的是各处的 invalidate ['messages'] / ['threads']——同步完成后的批量刷新
  //（见下方 syncStatus 那段）、SSE 推送，以及删除/移动/标记已读等 mutation 的收尾。
  // 换搜索词不在此列：queryKey 变了就是一个全新查询，走 isLoading 的骨架屏。
  const refreshing = conversationView
    ? threadSource.isFetching && !threadSource.isLoading && !threadSource.isFetchingNextPage
    : msgSource.isFetching && !msgSource.isLoading && !msgSource.isFetchingNextPage
  function loadMore() {
    if (conversationView) void threadSource.fetchNextPage()
    else void msgSource.fetchNextPage()
  }

  // 数据源标识：视图形态 + 搜索/聚合/文件夹 + 列表样式 + 筛选
  //（驱动 MailList 滚动重置）。
  // 筛选进 key：换了筛选就是另一份结果集，停在原滚动位置会落在一片空白里。
  // 搜索态固定写作 'search'：每敲一个字都重置滚动会把搜索框的焦点也打断。
  const sourceKey = `${conversationView ? 'th' : 'ms'}-${searching ? 'search' : (agg ?? folderId)}-${listStyle}-${filterKey(filter)}`

  // 选择作用域标识：比 sourceKey 多一个搜索词。
  //
  // 两者必须分开。上面为了保住搜索框焦点，把所有搜索都归成同一个 'search'，
  // 于是「搜发票 → 勾 5 封 → 改搜合同」全程 key 不变，选择就留了下来：
  // 工具栏显示「已选 5 封」而列表里一封都不高亮，此时点批量删除，
  // 删掉的是屏幕上根本看不见的那 5 封。滚动重置和选择清空是两件事，
  // 合用一个 key 就必然有一边是错的。
  const selectionKey = `${sourceKey}-${debouncedQuery}`

  // ── 批量选择 ──────────────────────────────────────────────────────────────
  //
  // 两种模式各持一套集合，而不是合成 `Set<number | string>`：
  // 选中项要回头去列表里找对应条目（求共同账户就是这么做的），而数字与字符串
  // 在 TS 里比较合法、运行时永不相等——合成一套会得到一个编译期看不出的空结果。
  // 两套集合互斥使用，selectionKey 变化时一并清空。
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  const [selectedThreadIds, setSelectedThreadIds] = useState<Set<string>>(() => new Set())
  // 切换数据源/样式/视图形态/搜索词时清空选择，避免跨上下文误操作。
  //
  // 挂起的删除必须同时落地：撤销入口马上要随当前列表一起消失了，
  // 留着它等于把一个再也无法撤销、也永远不会提交的操作丢在半空。
  // 而落地之后 toast 也必须立刻收起——它有自己独立的 5 秒计时，不收的话
  // 撤销按钮会继续挂在屏幕上，点下去却已无事可撤：用户以为撤销成功了，
  // 邮件其实已经永久删除。这是整套延迟提交里唯一会骗人的一条路径。
  useEffect(() => {
    undoable.flush()
    dismissToast()
    setSelectedIds(new Set())
    setSelectedThreadIds(new Set())
    setHiddenIds(new Set())
    setHiddenThreadIds(new Set())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectionKey])

  function toggleSelect(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  function toggleSelectThread(id: string) {
    setSelectedThreadIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }
  function selectAllVisible() {
    if (conversationView) setSelectedThreadIds(new Set((threads ?? []).map((th) => th.thread_id)))
    else setSelectedIds(new Set(messages.map((m) => m.id)))
  }
  function clearSelection() {
    setSelectedIds(new Set())
    setSelectedThreadIds(new Set())
  }

  // 已选条目的共同账户（跨账户为 null）；批量移动目标取该账户的文件夹
  const selectionAccountId = useMemo(() => {
    if (conversationView) return commonAccountId(selectedThreads(threads ?? [], selectedThreadIds))
    let acc: number | null = null
    for (const id of selectedIds) {
      const m = messages.find((x) => x.id === id)
      if (!m) continue
      if (acc === null) acc = m.account_id
      else if (acc !== m.account_id) return null
    }
    return acc
  }, [conversationView, selectedIds, selectedThreadIds, messages, threads])
  const { data: moveTargets = [] } = useFolders(selectionAccountId)

  const batchDelete = useBatchDelete()
  const batchMove = useBatchMove()
  const batchRead = useBatchRead()
  const batchFlag = useBatchFlag()
  const threadRead = useThreadBatchRead()
  const threadFlag = useThreadBatchFlag()
  const threadDelete = useThreadBatchDelete()
  const threadMove = useThreadBatchMove()
  const deleteOne = useDeleteMessage()
  const moveOne = useMoveMessage()

  // ── 可撤销操作 ──────────────────────────────────────────────────────────────
  //
  // 标准客户端不拦截删除，而是删完给一个撤销入口：confirm 打断操作节奏，
  // 而且一旦点了确认反而再也救不回来。这里请求挂起到撤销窗口结束才发出（见 useUndoable），
  // 撤销就是取消那次发送。

  /**
   * 当前打开的邮件在 ids 之列时，把阅读区推进到下一封（没有下一封则退回上一封）。
   *
   * 必须在把 ids 加进 hiddenIds **之前**调用：那之后 messages 已经过滤掉它们，
   * 就找不出"下一封是谁"了。
   */
  function advanceFromMessages(ids: number[]) {
    if (messageId == null || !ids.includes(messageId)) return
    const idx = messages.findIndex((m) => m.id === messageId)
    if (idx === -1) return
    // 先往后找，到底了再从当前位置往前回溯
    const rest = [...messages.slice(idx + 1), ...messages.slice(0, idx).reverse()]
    const next = rest.find((m) => !ids.includes(m.id))
    if (next) selectMessage(next.id)
    else setParam((p) => p.delete('message'))
  }

  function advanceFromThreads(ids: string[]) {
    if (threadId == null || !ids.includes(threadId)) return
    const idx = threadList.findIndex((th) => th.thread_id === threadId)
    if (idx === -1) return
    const rest = [...threadList.slice(idx + 1), ...threadList.slice(0, idx).reverse()]
    const next = rest.find((th) => !ids.includes(th.thread_id))
    if (next) selectThread(next.thread_id)
    else setParam((p) => p.delete('thread'))
  }

  /** 条目先从列表消失 → 请求挂起 → 弹出带「撤销」的提示。撤销则连同阅读位置一起还原。 */
  function runUndoable(opts: {
    message: string
    ids?: number[]
    threadIds?: string[]
    commit: () => void
  }) {
    const { ids = [], threadIds = [], message, commit } = opts
    if (ids.length === 0 && threadIds.length === 0) return
    const restoreMessageId = messageId
    const restoreThreadId = threadId

    if (ids.length > 0) advanceFromMessages(ids)
    if (threadIds.length > 0) advanceFromThreads(threadIds)

    if (ids.length > 0) setHiddenIds((prev) => new Set([...prev, ...ids]))
    if (threadIds.length > 0) setHiddenThreadIds((prev) => new Set([...prev, ...threadIds]))
    // 只把被操作的条目从选择里摘掉，而不是整个清空：用户可能正选着另一批，
    // 顺手删掉一封不该让那批选择一起消失。批量删除时这两者等价。
    if (ids.length > 0) {
      setSelectedIds((prev) => {
        if (prev.size === 0) return prev
        const next = new Set(prev)
        for (const id of ids) next.delete(id)
        return next
      })
    }
    if (threadIds.length > 0) {
      setSelectedThreadIds((prev) => {
        if (prev.size === 0) return prev
        const next = new Set(prev)
        for (const id of threadIds) next.delete(id)
        return next
      })
    }

    undoable.begin({
      commit,
      rollback: () => {
        if (ids.length > 0) {
          setHiddenIds((prev) => {
            const next = new Set(prev)
            for (const id of ids) next.delete(id)
            return next
          })
        }
        if (threadIds.length > 0) {
          setHiddenThreadIds((prev) => {
            const next = new Set(prev)
            for (const id of threadIds) next.delete(id)
            return next
          })
        }
        // 撤销的语义是「当作没发生过」，所以阅读区也回到操作前那一封
        if (restoreMessageId != null) setParam((p) => p.set('message', String(restoreMessageId)))
        if (restoreThreadId != null) setParam((p) => p.set('thread', restoreThreadId))
      },
    })

    toast(message, {
      actionLabel: t('app.undo'),
      duration: UNDO_WINDOW_MS,
      onAction: handleUndo,
      onExpire: undoable.flush,
    })
  }

  /**
   * 撤销，并保证用户一定收到反馈。
   *
   * undo 返回 false 意味着挂起项已被别处强制落地（切文件夹、切搜索词、页面将关闭），
   * 此时那封邮件已经真的删掉了。静默无操作会让用户以为撤销成功——
   * 宁可告诉他「来不及了」，也不能让他带着错误的认知离开。
   */
  function handleUndo(): boolean {
    const ok = undoable.undo()
    if (!ok) toast(t('list.undoUnavailable'))
    return ok
  }

  // 列表行 hover 快捷删除单封
  function onDeleteOne(id: number) {
    runUndoable({
      message: t('list.deletedToast'),
      ids: [id],
      commit: () => deleteOne.mutate(id),
    })
  }

  /**
   * 会话级删除/移动的作用域。
   *
   * 文件夹视图带上当前文件夹：只动这个文件夹里的成员，否则「在收件箱里删掉一条会话」
   * 会把已发送里自己的回复一并删掉。聚合/搜索视图不带，由后端排除 sent / drafts。
   */
  const threadScope = !searching && !agg && folderId != null ? folderId : undefined

  // 批量删除不再弹 confirm：改由撤销兜底（见 runUndoable 的头注释）
  function onBatchDelete() {
    if (conversationView) {
      const ids = [...selectedThreadIds]
      if (ids.length === 0) return
      runUndoable({
        message: t('list.thread.deletedToast', { count: ids.length }),
        threadIds: ids,
        commit: () => threadDelete.mutate({ threadIds: ids, inFolderId: threadScope }),
      })
      return
    }
    const ids = [...selectedIds]
    if (ids.length === 0) return
    runUndoable({
      message: t('list.deletedCountToast', { count: ids.length }),
      ids,
      commit: () => batchDelete.mutate(ids),
    })
  }
  function onBatchRead(read: boolean) {
    if (conversationView) {
      const ids = [...selectedThreadIds]
      if (ids.length === 0) return
      threadRead.mutate({ threadIds: ids, read }, { onSuccess: clearSelection })
      return
    }
    const ids = [...selectedIds]
    if (ids.length === 0) return
    batchRead.mutate({ ids, read }, { onSuccess: clearSelection })
  }
  function onBatchFlag(flagged: boolean) {
    if (conversationView) {
      const ids = [...selectedThreadIds]
      if (ids.length === 0) return
      threadFlag.mutate({ threadIds: ids, flagged }, { onSuccess: clearSelection })
      return
    }
    const ids = [...selectedIds]
    if (ids.length === 0) return
    batchFlag.mutate({ ids, flagged }, { onSuccess: clearSelection })
  }
  function onBatchMove(targetFolderId: number) {
    if (conversationView) {
      const ids = [...selectedThreadIds]
      if (ids.length === 0) return
      runUndoable({
        message: t('list.movedCountToast', { count: ids.length }),
        threadIds: ids,
        commit: () =>
          threadMove.mutate({ threadIds: ids, folderId: targetFolderId, inFolderId: threadScope }),
      })
      return
    }
    const ids = [...selectedIds]
    if (ids.length === 0) return
    runUndoable({
      message: t('list.movedCountToast', { count: ids.length }),
      ids,
      commit: () => batchMove.mutate({ ids, folderId: targetFolderId }),
    })
  }

  // ── 单条会话的行内操作（列表 hover 按钮与右键菜单）────────────────────────
  function onDeleteThread(item: ThreadListItem) {
    runUndoable({
      message: t('list.deletedToast'),
      threadIds: [item.thread_id],
      commit: () => threadDelete.mutate({ threadIds: [item.thread_id], inFolderId: threadScope }),
    })
  }
  function onToggleFlagThread(item: ThreadListItem, flagged: boolean) {
    threadFlag.mutate({ threadIds: [item.thread_id], flagged })
  }
  function onMarkReadThread(item: ThreadListItem, read: boolean) {
    threadRead.mutate({ threadIds: [item.thread_id], read })
  }
  function onMoveThread(item: ThreadListItem, targetFolderId: number) {
    runUndoable({
      message: t('list.movedToast'),
      threadIds: [item.thread_id],
      commit: () =>
        threadMove.mutate({
          threadIds: [item.thread_id],
          folderId: targetFolderId,
          inFolderId: threadScope,
        }),
    })
  }

  const markRead = useMarkRead()
  const toggleFlag = useToggleFlag()

  // 打开未读邮件时自动标已读（对展平后的 messages 生效）。
  //
  // ⚠ 必须走闸门「一封只发一次」，不能反复照着缓存里的 seen 重试：乐观更新落在
  // onMutate 的 `await cancelQueries` 之后，而 mutate() 立刻触发一次重渲染，
  // 这段窗口里 seen 仍是 false；onSettled 的 invalidate 又会让列表 refetch 把 seen
  // 刷回未读。叠加成 render → mutate → render 自激后就是 React error #185。
  // 详见 list-guards.ts。
  const autoReadGate = useRef(createAutoReadGate())
  // 只取一个布尔量当依赖：messages 是 flatMap 出来的新数组，
  // 直接进依赖数组等于「每渲染必跑」。
  const activeUnread =
    messageId != null && messages.some((m) => m.id === messageId && !m.seen)

  useEffect(() => {
    // shouldSend 为真时 messageId 必然非 null；这里多一句判断是给 TS 收窄类型用的
    if (autoReadGate.current.shouldSend(messageId, activeUnread) && messageId != null) {
      markRead.mutate({ id: messageId, read: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messageId, activeUnread])

  const [syncEnabled, setSyncEnabled] = useState(false)
  const { data: syncStatus } = useSyncStatus(accountId, syncEnabled)
  const triggerSync = useTriggerSync()
  const syncing = syncStatus?.phase === 'folders' || syncStatus?.phase === 'messages'

  // ── 账户对话框 state（新增账户用）─────────────────────────────────────────────
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)

  // ── 第三栏视图 state（邮件 / 通知）────────────────────────────────────────────
  // 通知中心浮层开关（与设置浮层同级；不再占用第三栏，见 NotificationsPage 头注释）
  const [notifOpen, setNotifOpen] = useState(false)
  // ── 设置浮层 state ────────────────────────────────────────────────────────────
  const [settingsOpen, setSettingsOpen] = useState(false)
  // ── 快捷键速查浮层 state（`?` 触发）────────────────────────────────────────────
  const [helpOpen, setHelpOpen] = useState(false)
  // ── 移动端侧栏抽屉 state ───────────────────────────────────────────────────────
  const [drawerOpen, setDrawerOpen] = useState(false)

  // ── 中栏视图 state（邮件视图内部：messages / drafts）───────────────────────────
  const [view, setView] = useState<'messages' | 'drafts'>('messages')

  // ── 撰写/回复/转发 state ──────────────────────────────────────────────────────
  const [composeOpen, setComposeOpen] = useState(false)
  const [composeInitial, setComposeInitial] = useState<ComposeInitial | undefined>(undefined)
  const [composeDraftId, setComposeDraftId] = useState<number | null>(null)

  // 会话模式下「当前这一封」由 ThreadReader 上报（手风琴里最近点开的那封）
  const [threadActiveMessageId, setThreadActiveMessageId] = useState<number | null>(null)
  // 快捷键 r 回复时需要当前邮件的完整数据；Reader / 手风琴已请求过，此处只复用缓存
  const { data: activeMessageDetail } = useMessageDetail(
    conversationView ? threadActiveMessageId : messageId,
  )

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
      void qc.invalidateQueries({ queryKey: ['threads'] })
      void qc.invalidateQueries({ queryKey: ['thread-messages'] })
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
    setNotifOpen(false)
    setSettingsOpen(false)
    setDrawerOpen(false)
    setSearchQuery('')
    setParam((p) => {
      p.set('account', String(id))
      p.delete('folder')
      p.delete('message')
      p.delete('thread')
      p.delete('agg')
    })
  }

  function selectFolder(id: number) {
    setView('messages')
    setNotifOpen(false)
    setSettingsOpen(false)
    setDrawerOpen(false)
    setSearchQuery('')
    setParam((p) => {
      p.set('folder', String(id))
      p.delete('message')
      p.delete('thread')
      p.delete('agg')
    })
  }

  // 选择聚合入口（跨所有账户）
  function selectAggregate(v: AggregateView) {
    setView('messages')
    setNotifOpen(false)
    setSettingsOpen(false)
    setDrawerOpen(false)
    setSearchQuery('')
    setParam((p) => {
      p.set('agg', v)
      p.delete('folder')
      p.delete('message')
      p.delete('thread')
    })
  }

  function selectMessage(id: number) {
    // 必须同时切回邮件视图：通知视图下第三栏渲染的是 NotificationsPage，
    // 只改 URL 的话 Reader 根本没挂载，点列表看起来「毫无反应」。
    // 列表与第三栏共处一屏，点列表就是要求第三栏显示那封邮件——
    // 视图归属应当跟随这个意图，而不是让用户先手动退出通知。
    setNotifOpen(false)
    setParam((p) => p.set('message', String(id)))
  }

  // 会话模式下第三栏由 thread_id 驱动；latest_id / account_id 从当前列表行取，
  // 因此这里只记 id，展开哪一封由 ThreadReader 依据列表行给出的 latestId 决定。
  function selectThread(id: string) {
    setNotifOpen(false)
    setParam((p) => p.set('thread', id))
  }

  /**
   * 按 id 打开一封邮件，并把视图切到能看见它的状态。成功返回 true。
   *
   * 「点了通知却跳到一片空白」在这条路径上踩过一次，所以**所有从通知类入口
   * 打开邮件的地方都必须走这里**，不能图省事用 selectMessage——后者只写 message
   * 参数，既不切账户/文件夹、也不管会话视图要的是 thread。
   * 站内通知与浏览器桌面通知共用此函数。
   */
  async function openMailById(messageId: number): Promise<boolean> {
    try {
      const { data } = await api.get<MessageDetail>(`/messages/${messageId}`)
      setView('messages')
      setNotifOpen(false)
      setSearchQuery('')
      setParam((p) => {
        p.set('account', String(data.account_id))
        p.set('folder', String(data.folder_id))
        p.delete('agg')
        // 会话视图的第三栏只认 thread 参数：只写 message 的话点通知会跳到一片空白。
        // 详情 DTO 带 thread_id，据此定位到那条会话；这封是新到的未读，
        // 手风琴的默认展开规则（最新一封 + 全部未读）保证它是打开的。
        if (conversationView && data.thread_id) {
          p.set('thread', data.thread_id)
          p.delete('message')
        } else {
          p.set('message', String(data.id))
          p.delete('thread')
        }
      })
      return true
    } catch {
      /* 邮件已删除等情况 → 由调用方决定回退 */
      return false
    }
  }

  // 点击通知跳转：单封新邮件（带 message_id）→ 精准打开该邮件；
  // openMailById 返回 false（邮件已删除等）时落到下面的账户收件箱回退。
  // 回退留在这里而不是塞进 openMailById：浏览器桌面通知那条路径根本没有回退可言，
  // 两个入口因此才能共用同一段定位逻辑。
  async function openNotification(n: Notification) {
    if (n.type !== 'mail_new' && !n.account_id) return
    if (n.message_id) {
      if (await openMailById(n.message_id)) return
    }
    if (!n.account_id) return
    try {
      const { data } = await api.get<{ folders: Folder[] }>(`/accounts/${n.account_id}/folders`)
      const inbox = data.folders?.find((f) => f.type === 'inbox')
      setView('messages')
      setNotifOpen(false)
      setSearchQuery('')
      setParam((p) => {
        p.set('account', String(n.account_id))
        if (inbox) p.set('folder', String(inbox.id))
        else p.delete('folder')
        p.delete('message')
        p.delete('thread')
        p.delete('agg')
      })
    } catch {
      selectAccount(n.account_id)
    }
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

  /**
   * 正文里的 mailto: 链接：打开应用自己的撰写器，而不是甩给系统默认邮件程序——
   * 在一个邮件客户端里点收件人地址却弹出别的客户端，是明显的断裂。
   *
   * href 由正文 iframe 经 postMessage 上报（去掉 allow-same-origin 后父窗口
   * 碰不到那份文档），协议已在 parseFrameMessage 里校验过是 mailto:。
   */
  function onMailto(href: string) {
    const initial = buildMailtoCompose(href)
    if (!initial) return
    setComposeInitial(initial)
    setComposeDraftId(null)
    setComposeOpen(true)
  }


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
      // M13 之前的草稿没有 from_alias，读出来是 undefined —— 落到账户默认发件项
      fromAlias: d.from_alias,
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
  // ── 阅读区上一封 / 下一封 ─────────────────────────────────────────────────────
  // 顺序取当前列表（已含筛选与排序），与 j/k 快捷键完全一致；
  // 位于边界时给 null，Reader 据此置灰按钮。
  const activeIndex = messageId == null ? -1 : messages.findIndex((m) => m.id === messageId)
  const prevMessageId = activeIndex > 0 ? messages[activeIndex - 1].id : null
  const nextMessageId =
    activeIndex >= 0 && activeIndex < messages.length - 1 ? messages[activeIndex + 1].id : null

  // 会话模式下「上一封/下一封」按会话切换，与 j/k 同一套顺序
  const activeThreadIndex =
    threadId == null ? -1 : threadList.findIndex((th) => th.thread_id === threadId)
  const prevThreadId = activeThreadIndex > 0 ? threadList[activeThreadIndex - 1].thread_id : null
  const nextThreadId =
    activeThreadIndex >= 0 && activeThreadIndex < threadList.length - 1
      ? threadList[activeThreadIndex + 1].thread_id
      : null
  // 当前会话行：ThreadReader 需要它的 latest_id（默认展开哪一封）与 account_id
  const activeThread = activeThreadIndex >= 0 ? threadList[activeThreadIndex] : null

  // ── 当前条目的删除 / 归档 / 移动 ──────────────────────────────────────────
  //
  // 工具栏按钮与键盘快捷键都打到这里：一个动作只有一处实现，不会两边各做一套
  // 然后慢慢漂移。三者都走 runUndoable，所以都带撤销、都会自动前进到下一封。
  // 当前是否有打开的条目——决定删除/星标这类快捷键是否可用
  const hasCurrent = conversationView ? threadId != null : messageId != null
  const currentAccountId = conversationView
    ? (activeThread?.account_id ?? null)
    : (activeMessageDetail?.account_id ?? null)
  const { data: currentFolders = [] } = useFolders(currentAccountId)
  const archiveFolder = currentFolders.find((f) => f.type === 'archive' && f.selectable) ?? null
  // 单封：已经在归档里就没有再归档一次的意义。
  // 会话跨文件夹，「整条已经在归档里」不成立，只要账户有归档文件夹就给入口。
  const canArchiveCurrent =
    archiveFolder != null &&
    (conversationView || archiveFolder.id !== activeMessageDetail?.folder_id)

  function deleteCurrent() {
    if (conversationView) {
      if (threadId == null) return
      runUndoable({
        message: t('list.deletedToast'),
        threadIds: [threadId],
        commit: () => threadDelete.mutate({ threadIds: [threadId], inFolderId: threadScope }),
      })
      return
    }
    if (messageId == null) return
    runUndoable({
      message: t('list.deletedToast'),
      ids: [messageId],
      commit: () => deleteOne.mutate(messageId),
    })
  }

  function moveCurrent(targetFolderId: number) {
    if (conversationView) {
      if (threadId == null) return
      runUndoable({
        message: t('list.movedToast'),
        threadIds: [threadId],
        commit: () =>
          threadMove.mutate({
            threadIds: [threadId],
            folderId: targetFolderId,
            inFolderId: threadScope,
          }),
      })
      return
    }
    if (messageId == null) return
    runUndoable({
      message: t('list.movedToast'),
      ids: [messageId],
      commit: () => moveOne.mutate({ id: messageId, folderId: targetFolderId }),
    })
  }

  function archiveCurrent() {
    if (archiveFolder == null) return
    const target = archiveFolder.id
    if (conversationView) {
      if (threadId == null) return
      runUndoable({
        message: t('reader.archivedToast'),
        threadIds: [threadId],
        commit: () =>
          threadMove.mutate({ threadIds: [threadId], folderId: target, inFolderId: threadScope }),
      })
      return
    }
    if (messageId == null) return
    runUndoable({
      message: t('reader.archivedToast'),
      ids: [messageId],
      commit: () => moveOne.mutate({ id: messageId, folderId: target }),
    })
  }

  /** 星标 / 标未读是即时可逆的，不进撤销窗口——再按一次就回去了。 */
  function toggleFlagCurrent() {
    if (conversationView) {
      if (threadId == null) return
      threadFlag.mutate({ threadIds: [threadId], flagged: !activeThread?.flagged })
      return
    }
    if (messageId == null || activeMessageDetail == null) return
    toggleFlag.mutate({ id: messageId, flagged: !activeMessageDetail.flagged })
  }

  function markUnreadCurrent() {
    if (conversationView) {
      if (threadId == null) return
      threadRead.mutate({ threadIds: [threadId], read: false })
      return
    }
    if (messageId == null) return
    markRead.mutate({ id: messageId, read: false })
  }

  /** g + i/s/t/d：跳到收件箱 / 星标 / 已发送 / 草稿。 */
  function goTo(target: GoTarget) {
    if (target === 'inbox') return selectAggregate('inbox')
    if (target === 'starred') return selectAggregate('starred')
    if (target === 'drafts') {
      if (accountId != null) onOpenDrafts(accountId)
      return
    }
    // 已发送没有聚合视图，落到当前账户的 sent 文件夹；账户没有这个文件夹就不动
    const sent = folders.find((f) => f.type === 'sent' && f.selectable)
    if (sent) selectFolder(sent.id)
  }

  /** x：把当前这一条纳入/移出批量选择。 */
  function toggleSelectCurrent() {
    if (conversationView) {
      if (threadId != null) toggleSelectThread(threadId)
      return
    }
    if (messageId != null) toggleSelect(messageId)
  }

  /**
   * Shift+J / Shift+K：把选择扩展到相邻一条，并把光标一起移过去。
   *
   * 与 j/k 的区别只在于「沿途的条目都留在选择里」——这正是批量处理一段连续
   * 邮件时最省事的走法。
   */
  function extendSelection(dir: 1 | -1) {
    if (conversationView) {
      if (threadId == null) return
      const idx = threadList.findIndex((th) => th.thread_id === threadId)
      const next = threadList[idx + dir]
      if (idx === -1 || !next) return
      setSelectedThreadIds((prev) => new Set([...prev, threadId, next.thread_id]))
      selectThread(next.thread_id)
      return
    }
    if (messageId == null) return
    const idx = messages.findIndex((m) => m.id === messageId)
    const next = messages[idx + dir]
    if (idx === -1 || !next) return
    setSelectedIds((prev) => new Set([...prev, messageId, next.id]))
    selectMessage(next.id)
  }

  // ── 全局键盘快捷键 ────────────────────────────────────────────────────────────
  useKeyboardShortcuts({
    onCompose,
    // 仅当有选中邮件且其详情已缓存时才允许快捷键回复
    onReply: activeMessageDetail != null ? () => onReply(activeMessageDetail) : null,
    onReplyAll:
      activeMessageDetail != null
        ? () => {
            setComposeInitial(buildReplyAll(activeMessageDetail, selfAddrs))
            setComposeDraftId(null)
            setComposeOpen(true)
          }
        : null,
    onForward: activeMessageDetail != null ? () => onForward(activeMessageDetail) : null,
    // j/k 在会话模式下按会话走，在单封模式下按邮件走——同一套导航逻辑，两种 id
    navIds: conversationView ? threadList.map((th) => th.thread_id) : messages.map((m) => m.id),
    activeNavId: conversationView ? threadId : messageId,
    onNavigate: (id) => {
      if (conversationView) selectThread(String(id))
      else selectMessage(Number(id))
    },
    // 这四个与工具栏按钮共用同一份实现，见上面的 deleteCurrent / archiveCurrent
    onArchive: canArchiveCurrent ? archiveCurrent : null,
    onDelete: hasCurrent ? deleteCurrent : null,
    onToggleStar: hasCurrent ? toggleFlagCurrent : null,
    onMarkUnread: hasCurrent ? markUnreadCurrent : null,
    onBack: onMobileBack,
    onGo: goTo,
    onToggleSelectCurrent: toggleSelectCurrent,
    onExtendSelection: extendSelection,
    // 不直接关：撰写器可能有未保存的内容，由它自己决定要不要先问一句
    onCloseCompose: () => window.dispatchEvent(new CustomEvent(COMPOSE_CLOSE_EVENT)),
    composeOpen,
    // Esc：清空当前邮件 / 关闭双栏浮动阅读 / 退出通知视图
    onEscape: onMobileBack,
    // ? 切换速查浮层；Esc 时优先关闭它
    onToggleHelp: () => setHelpOpen((o) => !o),
    onCloseHelp: () => setHelpOpen(false),
    helpOpen,
    // 其它浮层遮挡时屏蔽单键，并让出 Esc。
    // 漏掉任何一个的后果都是：那个浮层开着时按 # 会删掉背后的邮件，
    // 而撤销条在屏幕底部、用户此刻根本看不到。
    overlayOpen: settingsOpen || notifOpen || dialogOpen || drawerOpen,
    // 这里传裸的 undo：反馈由 onUndoUnavailable 给，
    // 用 handleUndo 会连 toast 带 hook 各弹一次。
    onUndo: undoable.undo,
    onUndoUnavailable: () => toast(t('list.undoUnavailable')),
  })

  // 移动端单栏：有选中邮件/会话或处于通知视图时显示阅读面板，否则显示列表面板
  // 通知已改为浮层，不再参与移动端的主面板切换——只看有没有选中条目
  const mobilePane: 'list' | 'reader' =
    (conversationView ? threadId != null : messageId != null) ? 'reader' : 'list'
  function onMobileBack() {
    setNotifOpen(false)
    setParam((p) => { p.delete('message'); p.delete('thread') })
  }

  // 列表标题/副标题：搜索 > 聚合 > 文件夹（文件夹由 MailList 内部据 folder 计算）
  const listTitle = searching ? t('list.searchTitle') : agg ? t(aggLabelKey[agg]) : undefined
  // 聚合视图的「共 N 封」取后端的真实总数，而不是 messages.length——
  // 后者只是当前已加载的那一页，翻页时数字还会往上跳，读起来像邮箱里只有 50 封。
  // 收件箱聚合额外带上未读数，与文件夹视图的副标题保持同一种写法。
  //
  // 筛选生效时一律改用后端返回的筛选后总数：否则会出现「共 320 封 · 32 未读」
  // 配着一屏 5 条的场面——那 320 是全量计数，与眼前这份筛过的列表不是一回事。
  const filtering = isFilterActive(filter)
  const listSubtitle = (() => {
    // 会话模式下条目是会话不是邮件，「共 320 封」配着 87 行会话读起来是错的。
    // 三条链路首页都会带 total（会话数没法从 folders 表现成拿），统一用它。
    if (conversationView) {
      const total = listTotalOf(threadPages) ?? threadList.length
      // 搜索态说「共 N 个会话」会被读成文件夹里一共这么多，实际是命中数
      if (searching) return t('list.thread.searchCount', { count: total })
      return filtering
        ? t('list.thread.filteredCount', { count: total })
        : t('list.thread.totalCount', { count: total })
    }
    if (searching) {
      // 命中总数由后端给出（已含筛选）；尚未返回时（首页在途）退回已加载条数
      const total = listTotalOf(searchInfinite.data?.pages) ?? messages.length
      return t('list.totalCount', { count: total })
    }
    if (!agg) {
      // 文件夹视图：不筛选时返回 undefined，交回 MailList 用 folder 上的现成计数。
      if (!filtering) return undefined
      const total = listTotalOf(folderInfinite.data?.pages) ?? messages.length
      return t('list.filteredCount', { count: total })
    }
    if (filtering) {
      const total = listTotalOf(aggInfinite.data?.pages) ?? messages.length
      return t('list.filteredCount', { count: total })
    }
    if (agg === 'inbox') {
      const total = t('list.totalCount', { count: aggCounts.inboxTotal })
      return aggCounts.inbox > 0
        ? `${total} · ${t('list.unreadCount', { count: aggCounts.inbox })}`
        : total
    }
    return t('list.totalCount', { count: aggCounts[agg] })
  })()

  // ── Sidebar（常驻所有视图）────────────────────────────────────────────────────
  const sidebar = (
    <AccountSidebar
      accounts={accounts}
      accountsError={accountsError}
      onRetryAccounts={() => void accountsQuery.refetch()}
      folders={folders}
      foldersError={foldersError}
      onRetryFolders={() => void foldersQuery.refetch()}
      activeAccountId={accountId}
      activeFolderId={folderId}
      syncing={syncing}
      notifOpen={notifOpen}
      settingsOpen={settingsOpen}
      activeAgg={agg}
      aggCounts={aggCounts}
      onSelectAccount={selectAccount}
      onSelectFolder={selectFolder}
      onSelectAggregate={selectAggregate}
      onSync={onSync}
      onAddAccount={() => { onAddAccount(); setDrawerOpen(false) }}
      onToggleNotif={() => { setNotifOpen((o) => !o); setDrawerOpen(false) }}
      onToggleSettings={() => { setSettingsOpen((o) => !o); setDrawerOpen(false) }}
      onCompose={() => { onCompose(); setDrawerOpen(false) }}
      onOpenDrafts={(id) => { onOpenDrafts(id); setDrawerOpen(false) }}
    />
  )

  return (
    <>
      {/* 新邮件的读屏播报区。常驻——aria-live 的语义是「这个区域的内容变化时播报」，
          区域本身和内容一起插入 DOM 时多数读屏不播报（与 Toast 里那个同理）。
          视觉用户能看见未读徽标跳变，读屏用户此前对新邮件到达完全无感知。 */}
      <div className="sr-only" role="status" aria-live="polite">
        {announce.text ? announce.text + '\u200b'.repeat(announce.seq % 2) : ''}
      </div>

      <AppLayout
        sidebar={sidebar}
        mobilePane={mobilePane}
        drawerOpen={drawerOpen}
        onDrawerOpenChange={setDrawerOpen}
        onMobileBack={onMobileBack}
        layoutMode={layoutMode}
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
              threads={threads}
              activeThreadId={threadId}
              onSelectThread={(item) => selectThread(item.thread_id)}
              onToggleFlagThread={onToggleFlagThread}
              onDeleteThread={onDeleteThread}
              onMarkReadThread={onMarkReadThread}
              onMoveThread={onMoveThread}
              selectedThreadIds={selectedThreadIds}
              onToggleSelectThread={toggleSelectThread}
              selfAddrs={selfAddrs}
              loading={messagesLoading}
              // 账户取不到时邮件列表无从谈起：此时消息链路多半是禁用状态
              // （没有 folderId），既不 loading 也没有 error，itemCount 为 0——
              // 不把账户的错误接进来，中间栏就会用空态谎报一次账户请求失败。
              error={messagesError ?? accountsError}
              refreshing={refreshing}
              // 只有确实取到了一个空账户列表才说「还没有邮箱账户」。
              // 判据问的是「收到过一份列表吗」而不是 status：后台重取失败时
              // status 变 error（isSuccess 转假）而缓存里的 [] 还在，
              // 用 isSuccess 会让 noAccounts 与 error 两头落空，
              // 主栏落进通用空态「暂无邮件」——新手刚添加完账户碰上重取失败，
              // 看到的就是既没有错误也没有添加入口的一片空白。
              // 这里的 data != null 与 isLoadingError 内部的 hasData 是同一个概念。
              noAccounts={accountsQuery.data != null && accounts.length === 0}
              acctColorOf={
                // 单文件夹视图里所有邮件同属一个账户，点色点只是噪声
                (agg != null || searching) && accounts.length > 1
                  ? (id) => acctColors.get(id) ?? null
                  : undefined
              }
              onAddAccount={() => { setEditingAccount(null); setDialogOpen(true) }}
              onRetry={() => {
                if (accountsError != null) void accountsQuery.refetch()
                void refetchMessages()
              }}
              activeMessageId={messageId}
              onSelectMessage={selectMessage}
              onToggleFlag={(id, flagged) => toggleFlag.mutate({ id, flagged })}
              listStyle={listStyle}
              hasNextPage={hasNextPage}
              isFetchingNextPage={isFetchingNextPage}
              nextPageError={nextPageError}
              onLoadMore={loadMore}
              onRetryNextPage={loadMore}
              searchValue={searchQuery}
              onSearchChange={setSearchQuery}
              searching={searching}
              filter={filter}
              onToggleFilter={onToggleFilter}
              onClearFilter={() => setFilter(EMPTY_FILTER)}
              selectedIds={selectedIds}
              onSelectRange={(ids) => setSelectedIds((prev) => new Set([...prev, ...ids]))}
              onSelectRangeThread={(ids) =>
                setSelectedThreadIds((prev) => new Set([...prev, ...ids]))
              }
              onToggleSelect={toggleSelect}
              onSelectAllVisible={selectAllVisible}
              onClearSelection={clearSelection}
              onBatchRead={onBatchRead}
              onBatchFlag={onBatchFlag}
              onBatchDelete={onBatchDelete}
              onBatchMove={onBatchMove}
              moveTargets={moveTargets}
              alwaysShowSelect={alwaysShowSelect}
              onDeleteMessage={onDeleteOne}
              folders={folders}
              onMarkRead={(id, read) => markRead.mutate({ id, read })}
              onMoveMessage={(id, fid) => moveOne.mutate({ id, folderId: fid })}
            />
          )
        }
        reader={
          // 第三栏专属于阅读区。通知已改为浮层，不再与之争抢——
          // 那正是「开着通知点邮件没反应」的成因。
          conversationView ? (
            <ThreadReader
              threadId={threadId}
              latestId={activeThread?.latest_id ?? null}
              accountId={activeThread?.account_id ?? accountId}
              inFolderId={threadScope}
              onReply={onReply}
              onForward={onForward}
              onDelete={deleteCurrent}
              onArchive={canArchiveCurrent ? archiveCurrent : null}
              onMove={moveCurrent}
              onPrev={prevThreadId != null ? () => selectThread(prevThreadId) : null}
              onNext={nextThreadId != null ? () => selectThread(nextThreadId) : null}
              onActiveMessageChange={setThreadActiveMessageId}
              onMailto={onMailto}
            />
          ) : (
            <Reader
              messageId={messageId}
              onReply={onReply}
              onForward={onForward}
              onDelete={deleteCurrent}
              onArchive={canArchiveCurrent ? archiveCurrent : null}
              onMove={moveCurrent}
              onPrev={prevMessageId != null ? () => selectMessage(prevMessageId) : null}
              onNext={nextMessageId != null ? () => selectMessage(nextMessageId) : null}
              onMailto={onMailto}
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
      {/* 通知中心（覆盖层 modal，与设置同级）*/}
      {notifOpen && (
        <NotificationsPage
          onClose={() => setNotifOpen(false)}
          onOpen={(n) => void openNotification(n)}
        />
      )}
      {/* 设置弹框（覆盖层 modal）*/}
      {settingsOpen && (
        <SettingsDialog
          listStyle={listStyle}
          onChangeListStyle={handleChangeListStyle}
          conversationView={conversationView}
          onChangeConversationView={handleChangeConversationView}
          alwaysShowSelect={alwaysShowSelect}
          onChangeAlwaysShowSelect={handleChangeAlwaysShowSelect}
          layoutMode={layoutMode}
          onChangeLayoutMode={handleChangeLayoutMode}
          onClose={() => setSettingsOpen(false)}
        />
      )}
      {/* 快捷键速查浮层（? 触发）*/}
      {helpOpen && <ShortcutsCheatsheet onClose={() => setHelpOpen(false)} />}
    </>
  )
}

import { keepPreviousData, useInfiniteQuery, useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { useSyncExternalStore } from 'react'
import api from '@/lib/api'
import { EMPTY_FILTER, applyFilterParams, filterKey } from '@/lib/list-filters'
import type { ListFilter } from '@/lib/list-filters'
import { getRemoteImageDefault, subscribePrivacyPrefs } from '@/lib/privacy-prefs'
import { isNewerStatus, syncStatusKey, writeSyncStatus } from '@/lib/sync-cache'
import { isSyncActive } from '@/lib/types'
import {
  applyThreadUnreadDelta,
  applyUnreadDelta,
  beginOptimistic,
  bumpAggregateCount,
  findCachedMessages,
  findCachedThreads,
  patchMessageDetail,
  patchMessages,
  patchThreads,
  patchThreadsEach,
  removeMessages,
  removeThreads,
  restoreMail,
} from '@/lib/optimistic'
import type { ThreadPatch } from '@/lib/optimistic'
import type { Account, AccountHealth, AccountInput, AccountStats, Alias, AliasInput, AppSettings, BlockEntry, BodySyncMode, ConnectionTestResult, Contact, DiagnosticsResponse, Draft, DraftRequest, Folder, MessageDetail, MessageListItem, MonitoringOverview, Notification, NotifyChannel, NotifyChannelInput, NotifyLog, OAuthCompleteInput, OAuthFlowStatus, OAuthProviderInfo, OAuthStartInput, OAuthStartResponse, Profile, RemoteSearchResult, Rule, RuleInput, RuleRun, RuleTestResult, SendRequest, Signature, SignatureInput, SyncStatus, ThreadCursor, ThreadPage, TrustedSender } from '@/lib/types'

/** 取单个账户的文件夹。useFolders 与 useFoldersOfAccounts 共用，保证两处 query key 与解包方式一致。 */
async function fetchFolders(accountId: number): Promise<Folder[]> {
  const { data } = await api.get<{ folders: Folder[] }>(`/accounts/${accountId}/folders`)
  return data.folders ?? []
}

export function useAccounts() {
  return useQuery({
    queryKey: ['accounts'],
    queryFn: async (): Promise<Account[]> => {
      // /accounts 返回裸数组（与 folders/messages 的包裹形状不同）；兼容两种形状。
      const { data } = await api.get<Account[] | { accounts: Account[] }>('/accounts')
      return Array.isArray(data) ? data : (data.accounts ?? [])
    },
  })
}

export function useFolders(accountId: number | null) {
  return useQuery({
    queryKey: ['folders', accountId],
    enabled: accountId != null,
    // 轮询兜底：桌面端（Wails）SSE 经 WebView2 自定义协议可能失效/缓冲，
    // 新账户初始同步逐步发现文件夹时也没有 SSE 事件，定时刷新保证列表自更新。
    refetchInterval: 30_000,
    // enabled 已保证 accountId 非空
    queryFn: (): Promise<Folder[]> => fetchFolders(Number(accountId)),
  })
}

export function useMessages(folderId: number | null) {
  return useQuery({
    queryKey: ['messages', folderId],
    enabled: folderId != null,
    queryFn: async (): Promise<MessageListItem[]> => {
      const { data } = await api.get<{ messages: MessageListItem[] }>(`/folders/${folderId}/messages?limit=50`)
      return data.messages ?? []
    },
  })
}

/**
 * 无限加载版邮件查询，复用 query key ['messages', folderId] 使现有 invalidate 生效。
 * before_uid=0 表示不限制，返回最新 50 封；翻页时传入上一页最后一封的 uid。
 *
 * filter 必须进 query key：否则切换筛选后 react-query 认作同一个查询，
 * 直接返回旧缓存，点了 chip 界面纹丝不动。
 */
export function useInfiniteMessages(folderId: number | null, filter: ListFilter = EMPTY_FILTER) {
  return useInfiniteQuery({
    queryKey: ['messages', folderId, filterKey(filter)],
    enabled: folderId != null,
    initialPageParam: 0,
    queryFn: async ({ pageParam }): Promise<AggregatePage> => {
      const params = new URLSearchParams({ limit: '50', before_uid: String(pageParam ?? 0) })
      applyFilterParams(params, filter)
      const { data } = await api.get<{ messages: MessageListItem[]; total?: number }>(
        `/folders/${folderId}/messages?${params.toString()}`,
      )
      // 与聚合/搜索统一成 { messages, total } 形状：optimistic.mapListCache 按数据形状
      // 分派（不认 query key），三条链路同形状后那边就只剩一个分支要维护。
      return { messages: data.messages ?? [], next_cursor: null, total: data.total }
    },
    getNextPageParam: (lastPage) => {
      const last = lastPage.messages.at(-1)
      return lastPage.messages.length < 50 || !last ? undefined : last.uid
    },
  })
}

/** 聚合视图：所有收件箱 / 所有未读 / 星标（跨所有账户） */
export type AggregateView = 'inbox' | 'unread' | 'starred'

/** 聚合列表翻页游标（不透明，由后端回传，前端原样传回） */
interface AggCursor {
  before_date: string
  before_id: number
}

/** 三条列表链路（文件夹 / 聚合 / 搜索）共用的分页形状 */
interface AggregatePage {
  messages: MessageListItem[]
  /** keyset 游标。文件夹链路走 before_uid，此项恒为 null。 */
  next_cursor: AggCursor | null
  /**
   * 该查询条件下的条目总数，后端只在第一页给出（翻页时结果不变，重复扫表纯属浪费）。
   * 搜索链路总是返回；文件夹/聚合链路仅在筛选生效时返回——不筛选时前端用
   * folders 表 / aggregate-counts 里现成的计数。
   */
  total?: number
}

/**
 * 跨账户聚合邮件列表（无限加载）。
 * 游标采用后端回传的 (date, id) keyset，规避跨文件夹 UID 不唯一与日期截断问题。
 * query key 以 'messages' 开头，使现有 invalidateQueries(['messages']) 一并刷新。
 */
export function useInfiniteAggregate(view: AggregateView | null, filter: ListFilter = EMPTY_FILTER) {
  return useInfiniteQuery({
    queryKey: ['messages', 'aggregate', view, filterKey(filter)],
    enabled: view != null,
    initialPageParam: null as AggCursor | null,
    queryFn: async ({ pageParam }): Promise<AggregatePage> => {
      const params = new URLSearchParams({ view: view as string, limit: '50' })
      if (pageParam) {
        params.set('before_date', pageParam.before_date)
        params.set('before_id', String(pageParam.before_id))
      }
      applyFilterParams(params, filter)
      const { data } = await api.get<AggregatePage>(`/aggregate/messages?${params.toString()}`)
      return { messages: data.messages ?? [], next_cursor: data.next_cursor ?? null, total: data.total }
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/**
 * 聚合入口计数。
 *
 * ⚠ inbox 是收件箱聚合的**未读数**（入口徽标的语义），不是条目总数；
 * 列表标题要显示的「共几封」用 inboxTotal。
 * unread / starred 视图里每条都符合该条件，徽标数本身就是总数。
 */
export function useAggregateCounts() {
  return useQuery({
    queryKey: ['aggregate-counts'],
    // 与 useFolders 同理：SSE 失效时的轮询兜底
    refetchInterval: 30_000,
    queryFn: async (): Promise<Record<AggregateView, number> & { inboxTotal: number }> => {
      const { data } = await api.get<{ counts: Record<string, number> }>('/aggregate/counts')
      const c = data.counts ?? {}
      return {
        inbox: c.inbox ?? 0,
        unread: c.unread ?? 0,
        starred: c.starred ?? 0,
        inboxTotal: c.inbox_total ?? 0,
      }
    },
  })
}

/**
 * 各账户未读数（侧栏账户角标）。
 *
 * 不能用「该账户各文件夹 unread_count 求和」代替：Gmail 把标签映射成 IMAP 文件夹，
 * 同一封未读会被 INBOX 与各标签文件夹重复累加。后端按 收件箱+自定义 的口径统计
 * 并对跨文件夹副本去重，与「全部未读」聚合入口保持同一个数。
 */
export function useAccountUnread() {
  return useQuery({
    queryKey: ['account-unread'],
    // 与 useFolders 同理：SSE 失效时的轮询兜底
    refetchInterval: 30_000,
    queryFn: async (): Promise<Record<number, number>> => {
      const { data } = await api.get<{ counts: Record<string, number> }>('/aggregate/account-unread')
      const out: Record<number, number> = {}
      for (const [id, n] of Object.entries(data.counts ?? {})) out[Number(id)] = n
      return out
    },
  })
}

/**
 * 跨账户全文搜索（无限加载）。q 为空时禁用。
 * 与聚合同款 (date,id) keyset 游标；query key 以 'messages' 开头便于统一失效。
 */
export function useInfiniteSearch(q: string, filter: ListFilter = EMPTY_FILTER) {
  const query = q.trim()
  return useInfiniteQuery({
    queryKey: ['messages', 'search', query, filterKey(filter)],
    enabled: query.length > 0,
    initialPageParam: null as AggCursor | null,
    queryFn: async ({ pageParam }): Promise<AggregatePage> => {
      const params = new URLSearchParams({ q: query, limit: '50' })
      if (pageParam) {
        params.set('before_date', pageParam.before_date)
        params.set('before_id', String(pageParam.before_id))
      }
      applyFilterParams(params, filter)
      const { data } = await api.get<AggregatePage>(`/search/messages?${params.toString()}`)
      return {
        messages: data.messages ?? [],
        next_cursor: data.next_cursor ?? null,
        total: data.total,
      }
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/**
 * 重建全文索引（FTS5）。
 *
 * 兜底手段：索引与 messages 表理论上由触发器保持同步，但导入/迁移/异常中断
 * 后可能对不上，表现为「明明有这封邮件却搜不到」。后端同步重建，邮件多时较慢。
 * 完成后清掉搜索缓存，让用户立刻能用新索引复查。
 */
export function useReindexSearch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => { await api.post('/search/reindex') },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['messages', 'search'] })
      qc.invalidateQueries({ queryKey: ['threads', 'search'] })
    },
  })
}

/**
 * 服务端兜底搜索：把当前查询翻译成 IMAP SEARCH 发给所有启用账户，
 * 把服务器命中但本地没有的邮件补抓入库。
 *
 * 同步执行、最长约 90 秒——本地库只存已同步的部分，深层历史必须回服务器捞，
 * 这是「明明记得有这封信却搜不到」的唯一出路。
 * 成功后失效整个 ['messages'] 前缀：补抓的邮件既要出现在搜索结果里，
 * 也会出现在它所属的文件夹列表里。
 */
export function useRemoteSearch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (q: string): Promise<RemoteSearchResult> => {
      const { data } = await api.post<RemoteSearchResult>('/search/remote', { q })
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['messages'] })
      qc.invalidateQueries({ queryKey: ['threads'] })
      qc.invalidateQueries({ queryKey: ['thread-messages'] })
    },
  })
}

/**
 * 从无限加载结果里取条目总数（后端只在第一页给出）。
 * 三条链路通用：搜索总是有值；文件夹/聚合仅在筛选生效时有值，否则为 undefined，
 * 调用方回落到 folders 表 / aggregate-counts 的现成计数。
 */
export function listTotalOf(pages: { total?: number }[] | undefined): number | undefined {
  return pages?.[0]?.total
}

/**
 * 从缓存里移除一批邮件的乐观更新（删除/移动共用）：
 * 列表立即去掉这些行，未读的还要把各级角标减回去。
 */
function optimisticRemove(qc: ReturnType<typeof useQueryClient>, ids: Set<number>) {
  const affected = findCachedMessages(qc, ids)
  removeMessages(qc, ids)
  applyUnreadDelta(qc, affected.filter((m) => !m.seen), -1)
}

/** 删除邮件（移到回收站；已在回收站则永久删除，由后端判定）。 */
export function useDeleteMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/messages/${id}/delete`)
    },
    // 乐观更新：本地立即消失，服务器侧由后端回写队列异步完成
    onMutate: async (id: number) => {
      const snap = await beginOptimistic(qc)
      optimisticRemove(qc, new Set([id]))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 移动邮件到同账户的另一个文件夹。 */
export function useMoveMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, folderId }: { id: number; folderId: number }) => {
      await api.post(`/messages/${id}/move`, { folder_id: folderId })
    },
    // 移动后邮件从源文件夹消失；目标文件夹里的那份由下次同步补齐
    onMutate: async ({ id }) => {
      const snap = await beginOptimistic(qc)
      optimisticRemove(qc, new Set([id]))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/**
 * 批量操作统一的缓存失效（邮件列表/会话列表/文件夹/聚合计数/账户未读）。
 *
 * 单封操作也要失效 ['threads']：会话行上的未读数、星标、封数都是成员的汇总，
 * 在手风琴里把一封标已读之后，左边那条会话仍然显示加粗才是最刺眼的不一致。
 */
function invalidateMailCaches(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ['messages'] })
  void qc.invalidateQueries({ queryKey: ['thread-messages'] })
  void qc.invalidateQueries({ queryKey: ['threads'] })
  void qc.invalidateQueries({ queryKey: ['folders'] })
  void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
  void qc.invalidateQueries({ queryKey: ['account-unread'] })
}

/**
 * 标记已读/未读的乐观更新（单封与批量共用）。
 * 未读角标只按「状态确实会变」的那些邮件调整，重复标记不会让计数漂移。
 */
function optimisticRead(qc: ReturnType<typeof useQueryClient>, ids: Set<number>, read: boolean) {
  const changed = findCachedMessages(qc, ids).filter((m) => m.seen !== read)
  patchMessages(qc, ids, { seen: read })
  for (const id of ids) patchMessageDetail(qc, id, { seen: read })
  applyUnreadDelta(qc, changed, read ? -1 : 1)
}

/** 加/取消星标的乐观更新（单封与批量共用）。 */
function optimisticFlag(qc: ReturnType<typeof useQueryClient>, ids: Set<number>, flagged: boolean) {
  const changed = findCachedMessages(qc, ids).filter((m) => m.flagged !== flagged)
  patchMessages(qc, ids, { flagged })
  for (const id of ids) patchMessageDetail(qc, id, { flagged })
  bumpAggregateCount(qc, 'starred', changed.length * (flagged ? 1 : -1))
}

/** 批量删除（移到回收站；已在回收站则永久删除）。 */
export function useBatchDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (ids: number[]) => { await api.post('/batch/delete', { ids }) },
    onMutate: async (ids: number[]) => {
      const snap = await beginOptimistic(qc)
      optimisticRemove(qc, new Set(ids))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 批量移动到同账户的目标文件夹。 */
export function useBatchMove() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, folderId }: { ids: number[]; folderId: number }) => {
      await api.post('/batch/move', { ids, folder_id: folderId })
    },
    onMutate: async ({ ids }) => {
      const snap = await beginOptimistic(qc)
      optimisticRemove(qc, new Set(ids))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 批量标记已读/未读。 */
export function useBatchRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, read }: { ids: number[]; read: boolean }) => {
      await api.post('/batch/read', { ids, read })
    },
    onMutate: async ({ ids, read }) => {
      const snap = await beginOptimistic(qc)
      optimisticRead(qc, new Set(ids), read)
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 批量加/取消星标。 */
export function useBatchFlag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, flagged }: { ids: number[]; flagged: boolean }) => {
      await api.post('/batch/flag', { ids, flagged })
    },
    onMutate: async ({ ids, flagged }) => {
      const snap = await beginOptimistic(qc)
      optimisticFlag(qc, new Set(ids), flagged)
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

// ── 会话线程（M10）────────────────────────────────────────────────────────────
//
// 三条会话列表链路与单封版一一对应，参数完全相同，只有分页游标从 before_id/before_uid
// 换成 (before_date, before_thread)。query key 一律以 'threads' 打头，
// 于是「刷新邮件」的地方只要多写一句 invalidate(['threads']) 就全覆盖了。

/** 会话列表的分页游标写入查询串（三条链路共用） */
function applyThreadCursor(params: URLSearchParams, cursor: ThreadCursor | null): void {
  if (!cursor) return
  params.set('before_date', cursor.before_date)
  params.set('before_thread', cursor.before_thread)
}

/** 统一收口响应形状，缺字段时给出安全默认值 */
function normalizeThreadPage(data: Partial<ThreadPage>): ThreadPage {
  return {
    threads: data.threads ?? [],
    next_cursor: data.next_cursor ?? null,
    total: data.total,
  }
}

/** 单文件夹的会话列表（无限加载）。 */
export function useInfiniteThreads(folderId: number | null, filter: ListFilter = EMPTY_FILTER) {
  return useInfiniteQuery({
    queryKey: ['threads', folderId, filterKey(filter)],
    enabled: folderId != null,
    initialPageParam: null as ThreadCursor | null,
    queryFn: async ({ pageParam }): Promise<ThreadPage> => {
      const params = new URLSearchParams({ limit: '50' })
      applyThreadCursor(params, pageParam)
      applyFilterParams(params, filter)
      const { data } = await api.get<ThreadPage>(`/folders/${folderId}/threads?${params.toString()}`)
      return normalizeThreadPage(data)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/** 跨账户聚合视图的会话列表（无限加载）。 */
export function useInfiniteAggregateThreads(
  view: AggregateView | null,
  filter: ListFilter = EMPTY_FILTER,
) {
  return useInfiniteQuery({
    queryKey: ['threads', 'aggregate', view, filterKey(filter)],
    enabled: view != null,
    initialPageParam: null as ThreadCursor | null,
    queryFn: async ({ pageParam }): Promise<ThreadPage> => {
      const params = new URLSearchParams({ view: view as string, limit: '50' })
      applyThreadCursor(params, pageParam)
      applyFilterParams(params, filter)
      const { data } = await api.get<ThreadPage>(`/aggregate/threads?${params.toString()}`)
      return normalizeThreadPage(data)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/** 跨账户全文搜索的会话列表（无限加载）。q 为空时禁用。 */
export function useInfiniteSearchThreads(q: string, filter: ListFilter = EMPTY_FILTER) {
  const query = q.trim()
  return useInfiniteQuery({
    queryKey: ['threads', 'search', query, filterKey(filter)],
    enabled: query.length > 0,
    initialPageParam: null as ThreadCursor | null,
    queryFn: async ({ pageParam }): Promise<ThreadPage> => {
      const params = new URLSearchParams({ q: query, limit: '50' })
      applyThreadCursor(params, pageParam)
      applyFilterParams(params, filter)
      const { data } = await api.get<ThreadPage>(`/search/threads?${params.toString()}`)
      return normalizeThreadPage(data)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/**
 * 一条会话的全部成员（日期升序，账户内跨文件夹、已去重）。
 *
 * thread_id 形如 `3:<CAF=abc@mail.gmail.com>`，带 `:`／`@`／`<>`——
 * 不编码会被当成查询串分隔符，服务端拿到的是被截断的 id。
 */
export function useThreadMessages(threadId: string | null) {
  return useQuery({
    // 独立前缀而不是 ['threads', 'messages', ...]：会话行列表与成员列表的失效时机
    // 完全不同。挂在 ['threads'] 下面，任何一次单封操作的 invalidate/cancelQueries
    // 都会顺手把正在读的这条会话的成员列表也重取一遍、甚至掐掉在途请求。
    queryKey: ['thread-messages', threadId],
    enabled: threadId != null && threadId.length > 0,
    // 与 useMessageDetail 同理：切换会话时留住上一条，免得手风琴整块闪一下
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<MessageListItem[]> => {
      const { data } = await api.get<{ messages: MessageListItem[] }>(
        `/threads/messages?thread_id=${encodeURIComponent(threadId as string)}`,
      )
      return data.messages ?? []
    },
  })
}

/**
 * 会话级删除/移动的作用范围。
 *
 * 文件夹视图传当前 folderId：只动这个文件夹里的成员，否则「在收件箱里删掉一条会话」
 * 会把已发送里的自己的回复一并删掉。聚合/搜索视图不传，由后端排除 sent/drafts。
 */
export interface ThreadScope {
  inFolderId?: number
}

/** 会话级标记已读/未读。 */
export function useThreadBatchRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ threadIds, read }: { threadIds: string[]; read: boolean }) => {
      await api.post('/threads/batch/read', { thread_ids: threadIds, read })
    },
    onMutate: async ({ threadIds, read }) => {
      const snap = await beginOptimistic(qc)
      const ids = new Set(threadIds)
      // 标为已读：未读数清零，各级角标按会话的未读封数扣回去。
      // 标为未读：无从得知整条里本来有几封已读，unread 只能先按封数顶格估；
      // 角标不动，等 onSettled 的 invalidate 拿服务端的真实值覆盖。
      if (read) {
        applyThreadUnreadDelta(qc, findCachedThreads(qc, ids), -1)
        patchThreads(qc, ids, { unread: 0 })
      } else {
        const patches = new Map<string, ThreadPatch>()
        for (const th of findCachedThreads(qc, ids)) patches.set(th.thread_id, { unread: th.count })
        patchThreadsEach(qc, patches)
      }
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 会话级加/取消星标（作用于全部成员）。 */
export function useThreadBatchFlag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ threadIds, flagged }: { threadIds: string[]; flagged: boolean }) => {
      await api.post('/threads/batch/flag', { thread_ids: threadIds, flagged })
    },
    onMutate: async ({ threadIds, flagged }) => {
      const snap = await beginOptimistic(qc)
      patchThreads(qc, new Set(threadIds), { flagged })
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 会话级删除（移到回收站；已在回收站则永久删除，由后端判定）。 */
export function useThreadBatchDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ threadIds, inFolderId }: { threadIds: string[] } & ThreadScope) => {
      await api.post('/threads/batch/delete', {
        thread_ids: threadIds,
        ...(inFolderId != null ? { in_folder_id: inFolderId } : {}),
      })
    },
    onMutate: async ({ threadIds }) => {
      const snap = await beginOptimistic(qc)
      // ⚠ 只把行从列表里拿掉，不动未读角标：带 in_folder_id 时后端只删这个文件夹里的
      // 成员，而会话的 unread 是**整条**（跨文件夹）的计数，照它扣会把别处仍然未读的
      // 那些也一起扣掉。角标交给 onSettled 的 invalidate 从服务端取真值。
      removeThreads(qc, new Set(threadIds))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/** 会话级移动到同账户的目标文件夹。 */
export function useThreadBatchMove() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({
      threadIds,
      folderId,
      inFolderId,
    }: { threadIds: string[]; folderId: number } & ThreadScope) => {
      await api.post('/threads/batch/move', {
        thread_ids: threadIds,
        folder_id: folderId,
        ...(inFolderId != null ? { in_folder_id: inFolderId } : {}),
      })
    },
    onMutate: async ({ threadIds }) => {
      const snap = await beginOptimistic(qc)
      // 与删除同理：作用范围可能只是整条会话的一部分，未读角标不做乐观估算
      removeThreads(qc, new Set(threadIds))
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: () => invalidateMailCaches(qc),
  })
}

/**
 * 整库重建会话归属。
 *
 * 与「重建搜索索引」同类的兜底：老库里的行没有 In-Reply-To/References 头，
 * 只能靠这一趟按主题兜底重放规则把它们并起来。
 */
export function useRebuildThreads() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<number> => {
      const { data } = await api.post<{ ok: boolean; threads: number }>('/threads/rebuild')
      return data.threads ?? 0
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['threads'] })
      // 重建会打散/合并会话，正在读的那条的成员列表也不再可信
      void qc.invalidateQueries({ queryKey: ['thread-messages'] })
    },
  })
}

/** 收件人自动补全：按输入片段检索历史往来联系人（按频率降序）。 */
export function useContacts(q: string, enabled: boolean) {
  return useQuery({
    queryKey: ['contacts', q],
    enabled,
    staleTime: 60_000,
    queryFn: async (): Promise<Contact[]> => {
      const { data } = await api.get<{ contacts: Contact[] }>(
        `/contacts?q=${encodeURIComponent(q)}&limit=8`,
      )
      return data.contacts ?? []
    },
  })
}

// ── 通知中心 ──────────────────────────────────────────────────────────────────

interface NotificationsPage {
  notifications: Notification[]
  unread_count: number
}

/** 站内通知 feed（无限加载，before_id 游标）。 */
export function useNotifications() {
  return useInfiniteQuery({
    queryKey: ['notifications'],
    initialPageParam: 0,
    queryFn: async ({ pageParam }): Promise<NotificationsPage> => {
      const { data } = await api.get<NotificationsPage>(`/notifications?limit=30&before_id=${pageParam ?? 0}`)
      return { notifications: data.notifications ?? [], unread_count: data.unread_count ?? 0 }
    },
    getNextPageParam: (lastPage) => {
      const arr = lastPage.notifications
      return arr.length < 30 ? undefined : arr[arr.length - 1].id
    },
  })
}

/** 轻量未读计数（铃铛角标用，定时刷新）。 */
export function useNotificationUnread() {
  return useQuery({
    queryKey: ['notifications-unread'],
    refetchInterval: 30_000,
    queryFn: async (): Promise<number> => {
      const { data } = await api.get<NotificationsPage>('/notifications?limit=1')
      return data.unread_count ?? 0
    },
  })
}

function invalidateNotifs(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ['notifications'] })
  void qc.invalidateQueries({ queryKey: ['notifications-unread'] })
}

export function useMarkNotificationRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.post('/notifications/read', { id }) },
    onSuccess: () => invalidateNotifs(qc),
  })
}

export function useMarkAllNotificationsRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => { await api.post('/notifications/read-all') },
    onSuccess: () => invalidateNotifs(qc),
  })
}

export function useClearNotifications() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => { await api.post('/notifications/clear') },
    onSuccess: () => invalidateNotifs(qc),
  })
}

// ── 外发渠道 ──────────────────────────────────────────────────────────────────

export function useNotifyChannels() {
  return useQuery({
    queryKey: ['notify-channels'],
    queryFn: async (): Promise<NotifyChannel[]> => {
      const { data } = await api.get<{ channels: NotifyChannel[] }>('/notify/channels')
      return data.channels ?? []
    },
  })
}

export function useCreateNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: NotifyChannelInput) => { await api.post('/notify/channels', input) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useUpdateNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: NotifyChannelInput }) => {
      await api.put(`/notify/channels/${id}`, input)
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useDeleteNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.delete(`/notify/channels/${id}`) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useTestNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.post(`/notify/channels/${id}/test`) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-logs'] }) },
  })
}

export function useNotifyLogs() {
  return useQuery({
    queryKey: ['notify-logs'],
    queryFn: async (): Promise<NotifyLog[]> => {
      const { data } = await api.get<{ logs: NotifyLog[] }>('/notify/logs?limit=50')
      return data.logs ?? []
    },
  })
}

// ── 系统监控 ──────────────────────────────────────────────────────────────────

/** 系统概览（打开监控面板时自动刷新）。enabled 控制仅在面板可见时轮询。 */
export function useMonitoringOverview(enabled: boolean) {
  return useQuery({
    queryKey: ['monitoring-overview'],
    enabled,
    refetchInterval: enabled ? 5000 : false,
    queryFn: async (): Promise<MonitoringOverview> => {
      const { data } = await api.get<MonitoringOverview>('/monitoring/overview')
      return data
    },
  })
}

/** 各账户健康。 */
export function useMonitoringAccounts(enabled: boolean) {
  return useQuery({
    queryKey: ['monitoring-accounts'],
    enabled,
    refetchInterval: enabled ? 5000 : false,
    queryFn: async (): Promise<AccountHealth[]> => {
      const { data } = await api.get<{ accounts: AccountHealth[] }>('/monitoring/accounts')
      return data.accounts ?? []
    },
  })
}

/** 单账户运行时诊断（仅在账户行展开时轮询，2s 一次）。 */
export function useMonitoringDiagnostics(accountId: number | null, enabled: boolean) {
  return useQuery({
    queryKey: ['monitoring-diagnostics', accountId],
    enabled: enabled && accountId != null,
    refetchInterval: enabled ? 2000 : false,
    queryFn: async (): Promise<DiagnosticsResponse> => {
      const { data } = await api.get<DiagnosticsResponse>(
        `/monitoring/accounts/${accountId}/diagnostics`,
      )
      return data
    },
  })
}

/** 取一次某账户的同步状态。抽出来是因为 SSE 重连后的对账也要用它。 */
export async function fetchSyncStatus(accountId: number): Promise<SyncStatus> {
  const { data } = await api.get<SyncStatus>(`/accounts/${accountId}/sync/status`)
  return data
}

/**
 * SSE（重新）连上时，对一遍那些「缓存里还在同步」的账户。
 *
 * ── 为什么必须有这一步 ─────────────────────────────────────────────────────
 *
 * SSE 是**尽力推送**，不是可靠投递：合盖唤醒、切网络、后端重启期间的事件全丢，
 * 而慢客户端还会被 hub 主动丢掉可丢的那一类（同步进度正是可丢的那类）。
 * 一旦错过的那条恰好是 `done`，缓存就永远停在 `messages`——
 * 而没有任何一方会来纠正它：账户行只读缓存不发请求，手动触发那一路早就收手了。
 * 用户看到的是某个账户**永久转圈**，只能靠刷新页面。
 *
 * 只对活跃态的账户：已经是 done/error/none 的没什么可对的，
 * 而无差别全拉会在每次重连时打出 N 个请求（弱网下重连很频繁）。
 */
export async function reconcileSyncStatus(qc: QueryClient): Promise<void> {
  const stale: number[] = []
  for (const q of qc.getQueryCache().findAll({ queryKey: ['sync-status'] })) {
    const st = q.state.data as SyncStatus | undefined
    const id = (q.queryKey as unknown[])[1]
    if (typeof id === 'number' && id > 0 && isSyncActive(st?.phase)) stale.push(id)
  }
  await Promise.all(
    stale.map(async (id) => {
      try {
        writeSyncStatus(qc, id, await fetchSyncStatus(id))
      } catch {
        // 对账是护栏不是主路径：失败就算了，下一次重连或下一轮推送还会有机会，
        // 而在这里抛出去会把 SSE 的 onState 回调打断。
      }
    }),
  )
}

export function useSyncStatus(accountId: number | null, enabled: boolean) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: syncStatusKey(accountId ?? 0),
    enabled: accountId != null && enabled,
    refetchInterval: enabled ? 1000 : false,
    queryFn: async (): Promise<SyncStatus> => {
      const data = await fetchSyncStatus(Number(accountId))
      // ⚠ 这里要挡一次旧响应：一个在同步期间发出、尚未落地的请求可能在 SSE 推来
      // done 之后才返回，把状态改回 messages——而那之后轮询已经停了，
      // 再没有任何东西会来纠正它。详见 sync-cache.ts 的 writeSyncStatus。
      const prev = qc.getQueryData<SyncStatus>(syncStatusKey(Number(accountId)))
      return isNewerStatus(prev, data) ? data : (prev as SyncStatus)
    },
  })
}

export function useTriggerSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      await api.post(`/accounts/${accountId}/sync`)
    },
    onSuccess: (_data, accountId) => {
      void qc.invalidateQueries({ queryKey: syncStatusKey(accountId) })
    },
  })
}

export function useCreateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<Account> => {
      const { data } = await api.post<Account>('/accounts', input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useUpdateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: AccountInput }): Promise<Account> => {
      const { data } = await api.put<Account>(`/accounts/${id}`, input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useDeleteAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number): Promise<void> => {
      await api.delete(`/accounts/${id}`)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['accounts'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
    },
  })
}

// ---- OAuth 授权 ----

/** 可用的 OAuth 提供方及其配置状态。 */
export function useOAuthProviders() {
  return useQuery({
    queryKey: ['oauth-providers'],
    queryFn: async (): Promise<OAuthProviderInfo[]> => {
      const { data } = await api.get<OAuthProviderInfo[]>('/accounts/oauth/providers')
      return data ?? []
    },
    // 部署级配置在运行期不会变，没必要反复拉。
    staleTime: Infinity,
  })
}

/** 发起一次授权流程，返回用户需要执行的动作。 */
export function useStartOAuth() {
  return useMutation({
    mutationFn: async (input: OAuthStartInput): Promise<OAuthStartResponse> => {
      const { data } = await api.post<OAuthStartResponse>('/accounts/oauth/start', input)
      return data
    },
  })
}

/**
 * 轮询授权进度。
 *
 * 后端等待浏览器回调或设备码轮询，前端无从得知何时完成，只能轮询；
 * 流程一旦离开 pending 就停下来，避免无谓请求。
 */
export function useOAuthFlowStatus(flowId: string | null) {
  return useQuery({
    queryKey: ['oauth-flow', flowId],
    enabled: !!flowId,
    queryFn: async (): Promise<OAuthFlowStatus> => {
      const { data } = await api.get<OAuthFlowStatus>(`/accounts/oauth/flows/${flowId}`)
      return data
    },
    refetchInterval: (query) => (query.state.data?.status === 'pending' ? 1500 : false),
    // 流程失效后后端返回 404，重试没有意义。
    retry: false,
  })
}

/** 用授权结果建号或为既有账户续上新令牌。 */
export function useCompleteOAuth() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: OAuthCompleteInput): Promise<Account> => {
      const { data } = await api.post<Account>('/accounts/oauth/complete', input)
      return data
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['accounts'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
    },
  })
}

/** 主动放弃一次授权流程，释放后端占用的本地回调端口。 */
export function useCancelOAuthFlow() {
  return useMutation({
    mutationFn: async (flowId: string): Promise<void> => {
      await api.delete(`/accounts/oauth/flows/${flowId}`)
    },
  })
}

export function useTestConnection() {
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<ConnectionTestResult> => {
      const { data } = await api.post<ConnectionTestResult>('/accounts/test', input)
      return data
    },
  })
}

/** useMessageDetail 的可选参数 */
export interface MessageDetailOptions {
  /**
   * 显式要求服务端保留正文里的远程引用（请求带 remote=1）。
   *
   * 不传时看全局的「默认显示远程图片」开关——把这个默认值放在这里而不是各个调用点，
   * 是为了让「开关打开 = 所有详情请求都带 remote=1」这件事只有一处实现。
   */
  remote?: boolean
}

export function useMessageDetail(messageId: number | null, opts?: MessageDetailOptions) {
  // 订阅而不是直读：开关参与下面的 query key，只有让已挂载的详情查询
  // 在开关改变时重新渲染、先换 key 再取数，才不会按旧口径白白再请求一次。
  const defaultRemote = useSyncExternalStore(
    subscribePrivacyPrefs,
    getRemoteImageDefault,
    getRemoteImageDefault,
  )
  const remote = opts?.remote ?? defaultRemote
  return useQuery({
    // ⚠ remote 必须进 key：同一封邮件的「挡住远程引用」与「放行远程引用」是两份不同的正文，
    // 共用一个缓存条目会让点过「显示图片」的那一封在下次打开时直接命中放行版本，
    // 等于把一次性的选择悄悄变成永久的。
    // 前缀仍是 ['message', id]，invalidateQueries / setQueriesData 按前缀照常命中两份。
    queryKey: ['message', messageId, remote],
    enabled: messageId != null,
    // 切换邮件时保留上一封的数据，直到新数据到达。
    // 否则每次点击都要走一遍 isLoading → 骨架屏 → 内容：本地接口只要几毫秒，
    // 这一两帧的骨架表现为第三栏"闪一下"。
    // ⚠ 保留期内 data 属于上一封邮件（data.id !== messageId），
    // Reader 必须据此禁用工具栏，避免"看着旧邮件、操作新邮件"。
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<MessageDetail> => {
      const { data } = await api.get<MessageDetail>(
        `/messages/${messageId}${remote ? '?remote=1' : ''}`,
      )
      return data
    },
  })
}

export function useMarkRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, read }: { id: number; read: boolean }) => {
      await api.post(`/messages/${id}/read`, { read })
    },
    // 乐观更新：列表行、详情、各级未读角标立即变，服务器回写走后端队列
    onMutate: async ({ id, read }) => {
      const snap = await beginOptimistic(qc)
      optimisticRead(qc, new Set([id]), read)
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: (_d, _e, { id }) => {
      invalidateMailCaches(qc)
      void qc.invalidateQueries({ queryKey: ['message', id] })
    },
  })
}

export function useToggleFlag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, flagged }: { id: number; flagged: boolean }) => {
      await api.post(`/messages/${id}/flag`, { flagged })
    },
    onMutate: async ({ id, flagged }) => {
      const snap = await beginOptimistic(qc)
      optimisticFlag(qc, new Set([id]), flagged)
      return snap
    },
    onError: (_e, _v, snap) => restoreMail(qc, snap),
    onSettled: (_d, _e, { id }) => {
      invalidateMailCaches(qc)
      void qc.invalidateQueries({ queryKey: ['message', id] })
    },
  })
}

/** 后端返回的正文预取模式做一次白名单校验，取值异常时回落到默认的「仅新邮件」。 */
function parseBodySyncMode(raw: string | undefined): BodySyncMode {
  return raw === 'recent' || raw === 'all' ? raw : 'new'
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: async (): Promise<AppSettings> => {
      const { data } = await api.get<{ settings: Record<string, string> }>('/settings')
      return {
        sync_depth: Number(data.settings?.sync_depth ?? 1000) || 1000,
        sync_poll_interval: Number(data.settings?.sync_poll_interval ?? 180) || 180,
        body_sync_mode: parseBodySyncMode(data.settings?.body_sync_mode),
        body_sync_recent_days: Number(data.settings?.body_sync_recent_days ?? 30) || 30,
      }
    },
  })
}

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (settings: Record<string, string>) => {
      await api.put('/settings', { settings })
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['settings'] }) },
  })
}

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: async (): Promise<Profile> => {
      const { data } = await api.get<Profile>('/auth/me')
      return data
    },
  })
}

export function useUpdateProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: { display_name: string; email: string }): Promise<Profile> => {
      const { data } = await api.put<Profile>('/auth/profile', req)
      return data
    },
    onSuccess: (data) => { qc.setQueryData(['me'], data) },
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: async ({ oldPassword, newPassword }: { oldPassword: string; newPassword: string }) => {
      await api.post('/auth/change-password', { old_password: oldPassword, new_password: newPassword })
    },
  })
}

export function useSetAccountEnabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) => {
      await api.post(`/accounts/${id}/enabled`, { enabled })
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useAccountStats(accountId: number | null) {
  return useQuery({
    queryKey: ['account-stats', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<AccountStats> => {
      const { data } = await api.get<AccountStats>(`/accounts/${accountId}/stats`)
      return data
    },
  })
}

/**
 * 从 axios 的上传进度事件算出 0~1 的比例；算不出来时返回 null。
 *
 * 抽成导出的函数是为了能被真正测到——把这三行留在 onUploadProgress 的闭包里，
 * 测试就只能照着它再写一遍判据，那样测的是「我对自己实现的理解」而不是实现本身。
 *
 * `total` 在两种情况下不可用：某些代理不回 Content-Length（undefined），
 * 以及空请求体（0，除零会得到 Infinity / NaN 并一路显示成「NaN%」）。
 */
export function uploadRatio(e: { loaded: number; total?: number }): number | null {
  if (e.total == null || e.total <= 0) return null
  return e.loaded / e.total
}

export function useSend() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({
      req,
      files,
      inline,
      onProgress,
    }: {
      req: SendRequest
      files?: File[]
      inline?: File[]
      /**
       * 上传进度（0~1）。只有走 multipart 那一支才会调用——纯文本正文是
       * 一次性发出去的，没有可报的中间状态。
       *
       * 进度留在调用方而不是这个 hook 里：mutation 的返回值是 react-query 的
       * 追踪代理，往上面 `{...mutation, progress}` 一摊就破坏了它的按需订阅。
       */
      onProgress?: (p: number) => void
    }) => {
      const hasAttach = (files?.length ?? 0) > 0
      const hasInline = (inline?.length ?? 0) > 0
      if (hasAttach || hasInline) {
        // 有附件或内联图：用 multipart/form-data，payload 为 JSON 字段。
        // 普通附件走 attachments，内联资源走 inline —— 后者的 Content-ID 由
        // payload.inline_cids 的**同下标项**给出，所以这里必须原样按序 append。
        const fd = new FormData()
        fd.append('payload', JSON.stringify(req))
        for (const f of files ?? []) fd.append('attachments', f, f.name)
        for (const f of inline ?? []) fd.append('inline', f, f.name)
        // 显式置空 Content-Type，让浏览器/axios 自动补全带 boundary 的 multipart 头。
        await api.post('/send', fd, {
          headers: { 'Content-Type': 'multipart/form-data' },
          // 10MB 附件此前零反馈：按钮变成「发送中…」然后一动不动几十秒，
          // 与卡死在界面上完全一样。
          onUploadProgress: (e) => {
            const r = uploadRatio(e)
            if (r != null) onProgress?.(r)
          },
        })
      } else {
        await api.post('/send', req)
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['threads'] })
    },
  })
}

export function useDrafts(accountId: number | null) {
  return useQuery({
    queryKey: ['drafts', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<Draft[]> => {
      const { data } = await api.get<{ drafts: Draft[] } | Draft[]>(`/accounts/${accountId}/drafts`)
      // 后端可能返回 {drafts:[]} 或裸数组，兼容两种形式
      return Array.isArray(data) ? data : (data.drafts ?? [])
    },
  })
}

export function useCreateDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: DraftRequest): Promise<Draft> => {
      const { data } = await api.post<Draft>('/drafts', req)
      return data
    },
    onSuccess: (_d, req) => { void qc.invalidateQueries({ queryKey: ['drafts', req.account_id] }) },
  })
}

export function useUpdateDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, req }: { id: number; req: DraftRequest }): Promise<Draft> => {
      const { data } = await api.put<Draft>(`/drafts/${id}`, req)
      return data
    },
    onSuccess: (_d, { req }) => { void qc.invalidateQueries({ queryKey: ['drafts', req.account_id] }) },
  })
}

export function useDeleteDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; accountId: number }) => { await api.delete(`/drafts/${id}`) },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['drafts', accountId] }) },
  })
}

export function useSendDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; accountId: number }) => { await api.post(`/drafts/${id}/send`) },
    onSuccess: (_d, { accountId }) => {
      void qc.invalidateQueries({ queryKey: ['drafts', accountId] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['threads'] })
    },
  })
}

// ── M11 规则引擎 ──────────────────────────────────────────────────────────────

/**
 * 若干账户的文件夹并集（规则编辑框的「移动到」目标）。
 *
 * 复用 ['folders', accountId] 这个 query key：与侧栏用的是同一份缓存，
 * 打开编辑框时多半直接命中，不会为了一个下拉框把所有账户的文件夹再拉一遍。
 */
export function useFoldersOfAccounts(accountIds: number[]) {
  return useQueries({
    queries: accountIds.map((id) => ({
      queryKey: ['folders', id],
      queryFn: (): Promise<Folder[]> => fetchFolders(id),
    })),
    combine: (results) => ({
      folders: results.flatMap((r) => r.data ?? []),
      isLoading: results.some((r) => r.isLoading),
    }),
  })
}

export function useRules() {
  return useQuery({
    queryKey: ['rules'],
    queryFn: async (): Promise<Rule[]> => {
      const { data } = await api.get<{ rules: Rule[] }>('/rules')
      return data.rules ?? []
    },
  })
}

export function useCreateRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: RuleInput): Promise<Rule> => {
      const { data } = await api.post<Rule>('/rules', input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['rules'] }) },
  })
}

export function useUpdateRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: RuleInput }): Promise<Rule> => {
      const { data } = await api.put<Rule>(`/rules/${id}`, input)
      return data
    },
    // onSettled 而非 onSuccess：失败时列表里的启用开关可能已经翻过去了，
    // 不重新拉一次就会停在一个服务端并不认可的状态上
    onSettled: () => { void qc.invalidateQueries({ queryKey: ['rules'] }) },
  })
}

export function useDeleteRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.delete(`/rules/${id}`) },
    onSettled: () => { void qc.invalidateQueries({ queryKey: ['rules'] }) },
  })
}

/**
 * 按传入的 id 顺序重写 priority（上下箭头调序）。
 *
 * 乐观更新：点一下箭头要等一轮往返才动，连点几下会看到行来回跳。
 * 先在缓存里按新顺序排好，失败再整段回滚。
 */
export function useReorderRules() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (ids: number[]) => { await api.post('/rules/reorder', { ids }) },
    onMutate: async (ids: number[]) => {
      await qc.cancelQueries({ queryKey: ['rules'] })
      const previous = qc.getQueryData<Rule[]>(['rules'])
      if (previous) {
        const byId = new Map(previous.map((r) => [r.id, r]))
        // 只取 ids 里认得的规则；期间被别处删掉的 id 直接跳过，不会塞进 undefined
        const next = ids.map((id) => byId.get(id)).filter((r): r is Rule => r !== undefined)
        if (next.length === previous.length) qc.setQueryData(['rules'], next)
      }
      return { previous }
    },
    onError: (_err, _ids, ctx) => {
      if (ctx?.previous) qc.setQueryData(['rules'], ctx.previous)
    },
    onSettled: () => { void qc.invalidateQueries({ queryKey: ['rules'] }) },
  })
}

/** 试运行：只读求值，不失效任何缓存（后端保证无副作用） */
export function useTestRule() {
  return useMutation({
    mutationFn: async ({ rule, limit }: { rule: RuleInput; limit?: number }): Promise<RuleTestResult> => {
      const { data } = await api.post<RuleTestResult>('/rules/test', { rule, limit })
      return {
        matched: data.matched ?? [],
        scanned: data.scanned ?? 0,
        without_body: data.without_body ?? 0,
        truncated: data.truncated ?? false,
      }
    },
  })
}

/** 执行日志。enabled 让它只在折叠区展开时才拉取。 */
export function useRuleRuns(enabled: boolean) {
  return useQuery({
    queryKey: ['rule-runs'],
    enabled,
    // 日志是诊断信息，展开的那一刻就该是最新的；缓存命中会让人以为规则没跑
    refetchOnMount: 'always',
    staleTime: 0,
    queryFn: async (): Promise<RuleRun[]> => {
      const { data } = await api.get<{ runs: RuleRun[] }>('/rules/runs?limit=50')
      return data.runs ?? []
    },
  })
}

// ── 黑名单 ────────────────────────────────────────────────────────────────────

export function useBlocklist() {
  return useQuery({
    queryKey: ['blocklist'],
    queryFn: async (): Promise<BlockEntry[]> => {
      const { data } = await api.get<{ entries: BlockEntry[] }>('/blocklist')
      return data.entries ?? []
    },
  })
}

/** 添加黑名单的结果：existed 表示后端返回 409（该 pattern 已在名单里） */
export interface AddBlockResult {
  entry: BlockEntry | null
  existed: boolean
}

/**
 * 添加黑名单。
 *
 * 409（已存在）在这里被吃掉转成 `existed: true` 而不是抛出：右键「屏蔽此发件人」重复点两次
 * 对用户而言就是「已经屏蔽了」，走 onError 分支会让每个调用点都得自己拆 axios 错误。
 * 400（pattern 非法）仍然照常抛出。
 */
export function useAddBlock() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { pattern: string; note?: string }): Promise<AddBlockResult> => {
      try {
        const { data } = await api.post<BlockEntry>('/blocklist', input)
        return { entry: data, existed: false }
      } catch (err) {
        if (axios.isAxiosError(err) && err.response?.status === 409) return { entry: null, existed: true }
        throw err
      }
    },
    // 只失效黑名单本身：黑名单作用于此后新收的邮件，已入库的一封都不动，
    // 顺手失效邮件列表只会让整个列表白重拉一遍
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['blocklist'] }) },
  })
}

export function useDeleteBlock() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.delete(`/blocklist/${id}`) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['blocklist'] }) },
  })
}

// ── M12 发件人信任名单 ────────────────────────────────────────────────────────

/**
 * 信任名单：名单里的发件人，其邮件详情接口直接返回保留远程引用的正文
 * （remote_allowed=true），不再显示拦截横幅。
 *
 * 只按精确地址匹配，不做域名——「总是显示此发件人的图片」是对一个人的信任，
 * 放宽到整个域名等于替用户做了一个他没做的决定。
 */
export function useTrustedSenders() {
  return useQuery({
    queryKey: ['trusted-senders'],
    queryFn: async (): Promise<TrustedSender[]> => {
      const { data } = await api.get<{ senders: TrustedSender[] }>('/privacy/trusted-senders')
      return data.senders ?? []
    },
  })
}

/** 添加信任发件人的结果：existed 表示后端返回 409（该地址已在名单里） */
export interface AddTrustedResult {
  sender: TrustedSender | null
  existed: boolean
}

/**
 * 把发件人加入信任名单。
 *
 * 409（已存在）吃掉转成 existed 而不是抛错：从两封不同邮件上各点一次
 * 「总是显示此发件人的图片」，对用户而言第二次就是「已经信任了」，不是失败。
 * 400（地址非法）仍照常抛出。
 *
 * ⚠ 成功后必须失效整个 ['message'] 前缀：名单是按发件人生效的，
 * 受影响的不只是当前这封，还有缓存里同一个人的其它邮件——
 * 只失效当前 id 会让用户在别的邮件上再看到一次拦截横幅。
 */
export function useAddTrustedSender() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (address: string): Promise<AddTrustedResult> => {
      try {
        const { data } = await api.post<TrustedSender>('/privacy/trusted-senders', { address })
        return { sender: data, existed: false }
      } catch (err) {
        if (axios.isAxiosError(err) && err.response?.status === 409) {
          return { sender: null, existed: true }
        }
        throw err
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['trusted-senders'] })
      void qc.invalidateQueries({ queryKey: ['message'] })
    },
  })
}

export function useDeleteTrustedSender() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.delete(`/privacy/trusted-senders/${id}`) },
    // 同上：撤销信任后，已缓存的详情里 remote_allowed 还是旧的 true，必须一并作废
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['trusted-senders'] })
      void qc.invalidateQueries({ queryKey: ['message'] })
    },
  })
}


// ── M13 撰写器：发件人别名与签名 ──────────────────────────────────────────────

/** 取单个账户的别名。useAliases 与 useAliasesOfAccounts 共用，保证 query key 一致。 */
async function fetchAliases(accountId: number): Promise<Alias[]> {
  const { data } = await api.get<{ aliases: Alias[] } | Alias[]>(`/accounts/${accountId}/aliases`)
  return Array.isArray(data) ? data : (data.aliases ?? [])
}

export function useAliases(accountId: number | null) {
  return useQuery({
    queryKey: ['aliases', accountId],
    enabled: accountId != null,
    queryFn: (): Promise<Alias[]> => fetchAliases(Number(accountId)),
  })
}

/**
 * 若干账户的别名，按账户 id 归组（撰写器的发件人下拉需要一次拿全）。
 *
 * 与 useFoldersOfAccounts 同构：复用 ['aliases', id] 这个 key，
 * 设置页里打开过的账户别名在这里直接命中缓存。
 */
export function useAliasesOfAccounts(accountIds: number[]) {
  return useQueries({
    queries: accountIds.map((id) => ({
      queryKey: ['aliases', id],
      queryFn: (): Promise<Alias[]> => fetchAliases(id),
    })),
    combine: (results) => {
      const byAccount: Record<number, Alias[]> = {}
      results.forEach((r, i) => { byAccount[accountIds[i]] = r.data ?? [] })
      return { byAccount, isLoading: results.some((r) => r.isLoading) }
    },
  })
}

export function useCreateAlias() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ accountId, input }: { accountId: number; input: AliasInput }): Promise<Alias> => {
      const { data } = await api.post<Alias>(`/accounts/${accountId}/aliases`, input)
      return data
    },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['aliases', accountId] }) },
  })
}

export function useUpdateAlias() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ accountId, aliasId, input }: { accountId: number; aliasId: number; input: AliasInput }): Promise<Alias> => {
      const { data } = await api.put<Alias>(`/accounts/${accountId}/aliases/${aliasId}`, input)
      return data
    },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['aliases', accountId] }) },
  })
}

export function useDeleteAlias() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ accountId, aliasId }: { accountId: number; aliasId: number }) => {
      await api.delete(`/accounts/${accountId}/aliases/${aliasId}`)
    },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['aliases', accountId] }) },
  })
}

/**
 * 账户签名。未配置时后端返回空对象，这里补齐字段，调用方不必到处判 undefined。
 *
 * 刻意不给 placeholderData：撰写器要等这个查询**落定**才决定插不插签名，
 * 占位数据会让它先按"没有签名"处理一次，真签名到货时就再也插不进去了。
 */
export function useSignature(accountId: number | null) {
  return useQuery({
    queryKey: ['signature', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<Signature> => {
      const { data } = await api.get<Partial<Signature>>(`/accounts/${accountId}/signature`)
      return {
        body_html: data?.body_html ?? '',
        use_on_new: data?.use_on_new ?? false,
        use_on_reply: data?.use_on_reply ?? false,
        updated_at: data?.updated_at,
      }
    },
  })
}

export function useSaveSignature() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ accountId, input }: { accountId: number; input: SignatureInput }): Promise<Signature> => {
      const { data } = await api.put<Signature>(`/accounts/${accountId}/signature`, input)
      return data
    },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['signature', accountId] }) },
  })
}

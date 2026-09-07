// 邮件操作的乐观更新工具。
//
// 背景：已读/星标/删除/移动都要经服务器回写 IMAP，慢的服务商上一次往返要数秒。
// 若等接口返回再刷新界面，点一下要愣好几秒。后端已改成「本地先落库 + 回写队列异步补」，
// 前端这里对应地做「先改缓存 + 失败回滚」，让点击即时生效。
//
// 缓存形状有三种（见 queries.ts）：
//   1. MessageListItem[]                       —— useMessages（非无限加载版）
//   2. { pages: MessageListItem[][] }          —— 历史形状，现无生产者，保留兜底
//   3. { pages: { messages: MessageListItem[] }[] } —— 文件夹 / 聚合 / 搜索三条无限加载链路
// mapListCache 把三种统一成「对一段列表做映射」，按数据形状分派而非 query key，
// 因此 query key 增减片段（如加入筛选标识）不影响这里。

import type { QueryClient, QueryKey } from '@tanstack/react-query'
import type { AggregateView } from '@/lib/queries'
import type { Folder, MessageListItem, ThreadListItem } from '@/lib/types'

/** 可乐观修改的邮件字段 */
export type MessagePatch = Partial<Pick<MessageListItem, 'seen' | 'flagged'>>

/** 快照条目：[queryKey, 原数据]，回滚时原样写回 */
export type MailSnapshot = [QueryKey, unknown][]

/**
 * 单封邮件列表的两个缓存前缀。
 *
 * ['messages'] 是文件夹/聚合/搜索三条链路；['thread-messages'] 是会话手风琴的成员列表。
 * 后者装的同样是 MessageListItem，因此单封操作（星标/已读/删除）必须同时改这两处——
 * 只改前者的话，在手风琴里点一下星标要等 refetch 回来才变。
 */
const MESSAGE_LIST_KEYS: QueryKey[] = [['messages'], ['thread-messages']]

/** 受邮件操作影响的所有缓存前缀（快照/回滚统一走这一份清单） */
const MAIL_QUERY_KEYS: QueryKey[] = [
  ['messages'],
  ['thread-messages'],
  ['message'],
  ['threads'],
  ['folders'],
  ['aggregate-counts'],
  ['account-unread'],
]

/** 对任意一种邮件列表缓存形状施加列表级映射；形状不认识时原样返回。 */
function mapListCache(
  old: unknown,
  fn: (list: MessageListItem[]) => MessageListItem[],
): unknown {
  if (!old) return old
  if (Array.isArray(old)) return fn(old as MessageListItem[])

  const paged = old as { pages?: unknown[] }
  if (!Array.isArray(paged.pages)) return old

  return {
    ...paged,
    pages: paged.pages.map((page) => {
      if (Array.isArray(page)) return fn(page as MessageListItem[])
      const wrapped = page as { messages?: MessageListItem[] }
      if (!Array.isArray(wrapped.messages)) return page
      return { ...wrapped, messages: fn(wrapped.messages) }
    }),
  }
}

/** 收集当前所有列表缓存里命中 ids 的邮件（去重，用于计算未读/星标增量）。 */
export function findCachedMessages(qc: QueryClient, ids: Set<number>): MessageListItem[] {
  const found = new Map<number, MessageListItem>()
  for (const key of MESSAGE_LIST_KEYS) {
    for (const [, data] of qc.getQueriesData({ queryKey: key })) {
      mapListCache(data, (list) => {
        for (const m of list) {
          if (ids.has(m.id) && !found.has(m.id)) found.set(m.id, m)
        }
        return list
      })
    }
  }
  return [...found.values()]
}

/** 就地修改列表缓存中命中 ids 的邮件字段。 */
export function patchMessages(qc: QueryClient, ids: Set<number>, patch: MessagePatch): void {
  for (const key of MESSAGE_LIST_KEYS) {
    qc.setQueriesData({ queryKey: key }, (old: unknown) =>
      mapListCache(old, (list) =>
        list.map((m) => (ids.has(m.id) ? { ...m, ...patch } : m)),
      ),
    )
  }
}

/** 从所有列表缓存中移除若干邮件（删除/移动后本地立即消失）。 */
export function removeMessages(qc: QueryClient, ids: Set<number>): void {
  for (const key of MESSAGE_LIST_KEYS) {
    qc.setQueriesData({ queryKey: key }, (old: unknown) =>
      mapListCache(old, (list) => list.filter((m) => !ids.has(m.id))),
    )
  }
}

/** 同步修改单封邮件详情缓存（阅读器正在显示这封时立即反映）。 */
export function patchMessageDetail(qc: QueryClient, id: number, patch: MessagePatch): void {
  qc.setQueryData(['message', id], (old: unknown) =>
    old ? { ...(old as object), ...patch } : old,
  )
}

/** 调整聚合入口徽标（unread / starred），结果不小于 0。 */
export function bumpAggregateCount(qc: QueryClient, view: AggregateView, delta: number): void {
  if (delta === 0) return
  qc.setQueryData(['aggregate-counts'], (old: unknown) => {
    if (!old) return old
    const counts = old as Record<AggregateView, number>
    return { ...counts, [view]: Math.max(0, (counts[view] ?? 0) + delta) }
  })
}

/** 调整某账户的未读角标，结果不小于 0。 */
export function bumpAccountUnread(qc: QueryClient, accountId: number, delta: number): void {
  if (delta === 0) return
  qc.setQueryData(['account-unread'], (old: unknown) => {
    if (!old) return old
    const counts = old as Record<number, number>
    return { ...counts, [accountId]: Math.max(0, (counts[accountId] ?? 0) + delta) }
  })
}

/** 调整某文件夹行的未读角标，结果不小于 0。 */
export function bumpFolderUnread(
  qc: QueryClient,
  accountId: number,
  folderId: number,
  delta: number,
): void {
  if (delta === 0) return
  qc.setQueryData(['folders', accountId], (old: unknown) => {
    if (!Array.isArray(old)) return old
    return (old as Folder[]).map((f) =>
      f.id === folderId ? { ...f, unread_count: Math.max(0, (f.unread_count ?? 0) + delta) } : f,
    )
  })
}

/**
 * 按一批邮件的实际未读状态调整各级未读角标。
 * delta 为每封未读邮件带来的增量：标为已读传 -1，标为未读传 +1。
 * 只统计状态确实会改变的邮件（重复标记不应让角标漂移）。
 */
export function applyUnreadDelta(qc: QueryClient, msgs: MessageListItem[], delta: number): void {
  for (const m of msgs) {
    bumpFolderUnread(qc, m.account_id, m.folder_id, delta)
    bumpAccountUnread(qc, m.account_id, delta)
    bumpAggregateCount(qc, 'unread', delta)
    bumpAggregateCount(qc, 'inbox', delta)
  }
}

/** 取受影响缓存的快照，供 onError 回滚。 */
export function snapshotMail(qc: QueryClient): MailSnapshot {
  const snap: MailSnapshot = []
  for (const key of MAIL_QUERY_KEYS) {
    snap.push(...qc.getQueriesData({ queryKey: key }))
  }
  return snap
}

/** 用快照还原缓存（乐观更新失败时调用）。 */
export function restoreMail(qc: QueryClient, snap: MailSnapshot | undefined): void {
  if (!snap) return
  for (const [key, data] of snap) qc.setQueryData(key, data)
}

/**
 * 乐观更新的统一前置：取消进行中的请求（避免旧响应覆盖刚写入的乐观值）并取快照。
 * 注意 cancelQueries 是异步的，调用方需 await。
 */
export async function beginOptimistic(qc: QueryClient): Promise<MailSnapshot> {
  await qc.cancelQueries({ queryKey: ['messages'] })
  await qc.cancelQueries({ queryKey: ['thread-messages'] })
  await qc.cancelQueries({ queryKey: ['threads'] })
  return snapshotMail(qc)
}

// ── 会话列表缓存（M10）────────────────────────────────────────────────────────
//
// 会话级操作只对 ['threads'] 会话行缓存做乐观更新，不去连带改单封列表：
// 前端手里只有 thread_id，成员 id 要再发一次请求才知道，为了让列表少闪一下
// 去多打一趟接口不划算。单封列表由 onSettled 的 invalidate 补齐，
// 而会话行本身（未读加粗、星标、整行消失）是用户此刻真正在看的东西。

/** 会话列表缓存形状：useInfiniteQuery 的 { pages: ThreadPage[] } */
function mapThreadCache(
  old: unknown,
  fn: (list: ThreadListItem[]) => ThreadListItem[],
): unknown {
  if (!old) return old
  const paged = old as { pages?: unknown[] }
  if (!Array.isArray(paged.pages)) return old
  return {
    ...paged,
    pages: paged.pages.map((page) => {
      const wrapped = page as { threads?: ThreadListItem[] }
      if (!Array.isArray(wrapped.threads)) return page
      return { ...wrapped, threads: fn(wrapped.threads) }
    }),
  }
}

/** 可乐观修改的会话字段 */
export type ThreadPatch = Partial<Pick<ThreadListItem, 'unread' | 'flagged'>>

/** 就地修改会话列表缓存里命中 ids 的会话字段 */
export function patchThreads(qc: QueryClient, ids: Set<string>, patch: ThreadPatch): void {
  qc.setQueriesData({ queryKey: ['threads'] }, (old: unknown) =>
    mapThreadCache(old, (list) =>
      list.map((th) => (ids.has(th.thread_id) ? { ...th, ...patch } : th)),
    ),
  )
}

/**
 * 按 thread_id 写入各不相同的补丁，整份缓存只遍历一次。
 *
 * 「整条标为未读」要把每条会话的 unread 顶到它自己的 count，逐条调 patchThreads
 * 等于把所有会话列表缓存扫 N 遍（N = 选中条数），一次批量操作就能扫上几十遍。
 */
export function patchThreadsEach(qc: QueryClient, patches: Map<string, ThreadPatch>): void {
  if (patches.size === 0) return
  qc.setQueriesData({ queryKey: ['threads'] }, (old: unknown) =>
    mapThreadCache(old, (list) =>
      list.map((th) => {
        const patch = patches.get(th.thread_id)
        return patch ? { ...th, ...patch } : th
      }),
    ),
  )
}

/** 从会话列表缓存中移除若干会话（会话级删除/移动后本地立即消失） */
export function removeThreads(qc: QueryClient, ids: Set<string>): void {
  qc.setQueriesData({ queryKey: ['threads'] }, (old: unknown) =>
    mapThreadCache(old, (list) => list.filter((th) => !ids.has(th.thread_id))),
  )
}

/** 收集当前会话列表缓存里命中 ids 的会话（去重，用于计算未读增量） */
export function findCachedThreads(qc: QueryClient, ids: Set<string>): ThreadListItem[] {
  const found = new Map<string, ThreadListItem>()
  for (const [, data] of qc.getQueriesData({ queryKey: ['threads'] })) {
    mapThreadCache(data, (list) => {
      for (const th of list) {
        if (ids.has(th.thread_id) && !found.has(th.thread_id)) found.set(th.thread_id, th)
      }
      return list
    })
  }
  return [...found.values()]
}

/**
 * 按会话的未读封数调整各级未读角标。
 * delta 为每封未读邮件带来的增量：整条标为已读传 -1，标为未读时无从得知
 * 「本来有几封已读」，因此只有已读方向会调用这里（见 queries.ts 的说明）。
 */
export function applyThreadUnreadDelta(
  qc: QueryClient,
  threads: ThreadListItem[],
  delta: number,
): void {
  for (const th of threads) {
    if (th.unread <= 0) continue
    const n = th.unread * delta
    bumpAccountUnread(qc, th.account_id, n)
    bumpAggregateCount(qc, 'unread', n)
    bumpAggregateCount(qc, 'inbox', n)
    // 文件夹角标按会话拆不开（成员跨文件夹），交给 onSettled 的 invalidate 补
  }
}

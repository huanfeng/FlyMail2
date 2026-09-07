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
import type { Folder, MessageListItem } from '@/lib/types'

/** 可乐观修改的邮件字段 */
export type MessagePatch = Partial<Pick<MessageListItem, 'seen' | 'flagged'>>

/** 快照条目：[queryKey, 原数据]，回滚时原样写回 */
export type MailSnapshot = [QueryKey, unknown][]

/** 受邮件操作影响的所有缓存前缀（快照/回滚统一走这一份清单） */
const MAIL_QUERY_KEYS: QueryKey[] = [
  ['messages'],
  ['message'],
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
  for (const [, data] of qc.getQueriesData({ queryKey: ['messages'] })) {
    mapListCache(data, (list) => {
      for (const m of list) {
        if (ids.has(m.id) && !found.has(m.id)) found.set(m.id, m)
      }
      return list
    })
  }
  return [...found.values()]
}

/** 就地修改列表缓存中命中 ids 的邮件字段。 */
export function patchMessages(qc: QueryClient, ids: Set<number>, patch: MessagePatch): void {
  qc.setQueriesData({ queryKey: ['messages'] }, (old: unknown) =>
    mapListCache(old, (list) =>
      list.map((m) => (ids.has(m.id) ? { ...m, ...patch } : m)),
    ),
  )
}

/** 从所有列表缓存中移除若干邮件（删除/移动后本地立即消失）。 */
export function removeMessages(qc: QueryClient, ids: Set<number>): void {
  qc.setQueriesData({ queryKey: ['messages'] }, (old: unknown) =>
    mapListCache(old, (list) => list.filter((m) => !ids.has(m.id))),
  )
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
  return snapshotMail(qc)
}

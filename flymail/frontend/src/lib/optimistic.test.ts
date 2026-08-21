import { describe, expect, it } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import type { MessageListItem } from '@/lib/types'
import {
  applyUnreadDelta,
  bumpAggregateCount,
  findCachedMessages,
  patchMessageDetail,
  patchMessages,
  removeMessages,
  restoreMail,
  snapshotMail,
} from '@/lib/optimistic'

function msg(id: number, over: Partial<MessageListItem> = {}): MessageListItem {
  return {
    id,
    account_id: 1,
    folder_id: 4,
    uid: id,
    subject: `s${id}`,
    from_name: 'a',
    from_addr: 'a@x',
    date: '2026-01-01T00:00:00Z',
    seen: false,
    flagged: false,
    has_attachment: false,
    snippet: '',
    ...over,
  } as MessageListItem
}

/** 三种缓存形状各塞一份，验证工具函数都能处理 */
function seed(qc: QueryClient) {
  // 1. 普通数组（useMessages）
  qc.setQueryData(['messages', 4], [msg(1), msg(2, { seen: true })])
  // 2. 分页数组（useInfiniteMessages）
  qc.setQueryData(['messages', 9], { pages: [[msg(1)], [msg(3)]], pageParams: [0, 1] })
  // 3. 分页对象（聚合/搜索）
  qc.setQueryData(['messages', 'aggregate', 'unread'], {
    pages: [{ messages: [msg(1), msg(3)], next_cursor: null }],
    pageParams: [null],
  })
}

/** 从三种缓存里取出 id 对应的条目，便于断言 */
function collect(qc: QueryClient, id: number): MessageListItem[] {
  const out: MessageListItem[] = []
  const flat = qc.getQueryData(['messages', 4]) as MessageListItem[] | undefined
  const paged = qc.getQueryData(['messages', 9]) as { pages: MessageListItem[][] } | undefined
  const agg = qc.getQueryData(['messages', 'aggregate', 'unread']) as
    | { pages: { messages: MessageListItem[] }[] }
    | undefined
  for (const m of flat ?? []) if (m.id === id) out.push(m)
  for (const page of paged?.pages ?? []) for (const m of page) if (m.id === id) out.push(m)
  for (const page of agg?.pages ?? []) for (const m of page.messages) if (m.id === id) out.push(m)
  return out
}

describe('patchMessages', () => {
  it('把三种缓存形状里的同一封邮件一起改掉', () => {
    const qc = new QueryClient()
    seed(qc)
    patchMessages(qc, new Set([1]), { seen: true })
    const hits = collect(qc, 1)
    expect(hits).toHaveLength(3) // 数组 / 分页数组 / 分页对象 各一份
    expect(hits.every((m) => m.seen)).toBe(true)
  })

  it('不动未命中的邮件', () => {
    const qc = new QueryClient()
    seed(qc)
    patchMessages(qc, new Set([1]), { flagged: true })
    expect(collect(qc, 3).every((m) => !m.flagged)).toBe(true)
  })
})

describe('removeMessages', () => {
  it('从所有缓存形状里移除', () => {
    const qc = new QueryClient()
    seed(qc)
    removeMessages(qc, new Set([1]))
    expect(collect(qc, 1)).toHaveLength(0)
    expect(collect(qc, 3)).toHaveLength(2) // 其余邮件仍在
  })
})

describe('findCachedMessages', () => {
  it('跨缓存去重返回', () => {
    const qc = new QueryClient()
    seed(qc)
    const found = findCachedMessages(qc, new Set([1, 2]))
    expect(found.map((m) => m.id).sort()).toEqual([1, 2])
  })
})

describe('applyUnreadDelta', () => {
  it('同步下调文件夹/账户/聚合三处未读角标', () => {
    const qc = new QueryClient()
    qc.setQueryData(['folders', 1], [{ id: 4, unread_count: 3 }])
    qc.setQueryData(['account-unread'], { 1: 5 })
    qc.setQueryData(['aggregate-counts'], { inbox: 5, unread: 5, starred: 2 })

    applyUnreadDelta(qc, [msg(1)], -1)

    expect((qc.getQueryData(['folders', 1]) as { unread_count: number }[])[0].unread_count).toBe(2)
    expect((qc.getQueryData(['account-unread']) as Record<number, number>)[1]).toBe(4)
    expect((qc.getQueryData(['aggregate-counts']) as Record<string, number>).unread).toBe(4)
  })

  it('角标不会被减成负数', () => {
    const qc = new QueryClient()
    qc.setQueryData(['account-unread'], { 1: 0 })
    applyUnreadDelta(qc, [msg(1)], -1)
    expect((qc.getQueryData(['account-unread']) as Record<number, number>)[1]).toBe(0)
  })
})

describe('bumpAggregateCount', () => {
  it('delta 为 0 时不改动', () => {
    const qc = new QueryClient()
    qc.setQueryData(['aggregate-counts'], { inbox: 1, unread: 1, starred: 7 })
    bumpAggregateCount(qc, 'starred', 0)
    expect((qc.getQueryData(['aggregate-counts']) as Record<string, number>).starred).toBe(7)
  })
})

describe('snapshotMail / restoreMail', () => {
  it('回滚后缓存恢复原样', () => {
    const qc = new QueryClient()
    seed(qc)
    qc.setQueryData(['account-unread'], { 1: 5 })
    const snap = snapshotMail(qc)

    removeMessages(qc, new Set([1]))
    applyUnreadDelta(qc, [msg(1)], -1)
    expect(collect(qc, 1)).toHaveLength(0)

    restoreMail(qc, snap)
    expect(collect(qc, 1)).toHaveLength(3)
    expect((qc.getQueryData(['account-unread']) as Record<number, number>)[1]).toBe(5)
  })
})

describe('patchMessageDetail', () => {
  it('详情缓存存在时才改', () => {
    const qc = new QueryClient()
    qc.setQueryData(['message', 1], { id: 1, seen: false })
    patchMessageDetail(qc, 1, { seen: true })
    patchMessageDetail(qc, 99, { seen: true })
    expect((qc.getQueryData(['message', 1]) as { seen: boolean }).seen).toBe(true)
    expect(qc.getQueryData(['message', 99])).toBeUndefined()
  })
})

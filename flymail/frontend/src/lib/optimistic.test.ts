import { describe, it, expect, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import {
  applyThreadUnreadDelta,
  findCachedThreads,
  patchMessages,
  patchThreads,
  patchThreadsEach,
  removeMessages,
  removeThreads,
  snapshotMail,
  restoreMail,
} from '@/lib/optimistic'
import type { MessageListItem, ThreadListItem } from '@/lib/types'

function thread(id: string, over: Partial<ThreadListItem> = {}): ThreadListItem {
  return {
    thread_id: id,
    account_id: 1,
    count: 3,
    unread: 0,
    flagged: false,
    has_attachment: false,
    subject: 's',
    snippet: '',
    date: '2026-09-01T00:00:00Z',
    latest_id: 1,
    latest_folder_id: 1,
    participants: [],
    ...over,
  }
}

function msg(id: number, over: Partial<MessageListItem> = {}): MessageListItem {
  return {
    id,
    account_id: 1,
    folder_id: 1,
    uid: id,
    subject: 's',
    from_name: '',
    from_addr: 'a@b.com',
    to: [],
    date: '2026-09-01T00:00:00Z',
    size: 0,
    seen: false,
    flagged: false,
    has_attachment: false,
    snippet: '',
    ...over,
  }
}

/** 会话列表缓存的形状：useInfiniteQuery 的 { pages: ThreadPage[] } */
function threadPages(list: ThreadListItem[]) {
  return { pages: [{ threads: list, next_cursor: null }], pageParams: [null] }
}

let qc: QueryClient

beforeEach(() => {
  qc = new QueryClient()
})

describe('patchThreads / findCachedThreads', () => {
  it('只改命中的会话，其余原样', () => {
    qc.setQueryData(['threads', 1, 'none'], threadPages([thread('t1'), thread('t2')]))
    patchThreads(qc, new Set(['t1']), { flagged: true })
    const data = qc.getQueryData(['threads', 1, 'none']) as ReturnType<typeof threadPages>
    expect(data.pages[0].threads[0].flagged).toBe(true)
    expect(data.pages[0].threads[1].flagged).toBe(false)
  })

  it('跨多份会话缓存同时生效', () => {
    qc.setQueryData(['threads', 1, 'none'], threadPages([thread('t1')]))
    qc.setQueryData(['threads', 'aggregate', 'inbox', 'none'], threadPages([thread('t1')]))
    patchThreads(qc, new Set(['t1']), { unread: 0 })
    for (const key of [['threads', 1, 'none'], ['threads', 'aggregate', 'inbox', 'none']]) {
      const d = qc.getQueryData(key) as ReturnType<typeof threadPages>
      expect(d.pages[0].threads[0].unread).toBe(0)
    }
  })

  it('findCachedThreads 去重后返回命中的会话', () => {
    qc.setQueryData(['threads', 1, 'none'], threadPages([thread('t1', { unread: 2 }), thread('t2')]))
    qc.setQueryData(['threads', 'search', 'q', 'none'], threadPages([thread('t1', { unread: 9 })]))
    const got = findCachedThreads(qc, new Set(['t1']))
    expect(got.length).toBe(1)
    expect(got[0].thread_id).toBe('t1')
  })

  it('缓存里没有会话列表时不抛异常', () => {
    expect(() => patchThreads(qc, new Set(['t1']), { flagged: true })).not.toThrow()
    expect(findCachedThreads(qc, new Set(['t1']))).toEqual([])
  })
})

describe('patchThreadsEach', () => {
  it('逐条写入各不相同的值', () => {
    qc.setQueryData(
      ['threads', 1, 'none'],
      threadPages([thread('t1', { count: 3 }), thread('t2', { count: 7 }), thread('t3')]),
    )
    patchThreadsEach(qc, new Map([['t1', { unread: 3 }], ['t2', { unread: 7 }]]))
    const d = qc.getQueryData(['threads', 1, 'none']) as ReturnType<typeof threadPages>
    expect(d.pages[0].threads.map((t) => t.unread)).toEqual([3, 7, 0])
  })

  it('空 Map 不改动缓存对象', () => {
    const before = threadPages([thread('t1')])
    qc.setQueryData(['threads', 1, 'none'], before)
    patchThreadsEach(qc, new Map())
    expect(qc.getQueryData(['threads', 1, 'none'])).toBe(before)
  })
})

describe('removeThreads', () => {
  it('把会话从列表里摘掉', () => {
    qc.setQueryData(['threads', 1, 'none'], threadPages([thread('t1'), thread('t2')]))
    removeThreads(qc, new Set(['t1']))
    const d = qc.getQueryData(['threads', 1, 'none']) as ReturnType<typeof threadPages>
    expect(d.pages[0].threads.map((t) => t.thread_id)).toEqual(['t2'])
  })
})

describe('mapThreadCache 的形状安全', () => {
  // ⚠ 会话成员列表（['thread-messages', tid]）装的是裸 MessageListItem[]，
  // 与会话行列表完全不同。会话级的补丁函数只认 { pages: [{ threads }] }，
  // 碰到别的形状必须原样返回，不能把成员列表改坏或抹平。
  it('裸数组（成员列表形状）原样返回', () => {
    const members = [msg(1), msg(2)]
    qc.setQueryData(['threads'], members)
    patchThreads(qc, new Set(['t1']), { flagged: true })
    expect(qc.getQueryData(['threads'])).toBe(members)
  })

  it('pages 里不是 threads 的页原样返回', () => {
    const odd = { pages: [{ messages: [msg(1)] }], pageParams: [null] }
    qc.setQueryData(['threads', 'odd'], odd)
    patchThreads(qc, new Set(['t1']), { flagged: true })
    const d = qc.getQueryData(['threads', 'odd']) as typeof odd
    expect(d.pages[0]).toBe(odd.pages[0])
  })

  it('undefined / null 缓存不抛异常', () => {
    qc.setQueryData(['threads', 'empty'], null)
    expect(() => removeThreads(qc, new Set(['t1']))).not.toThrow()
  })
})

describe('applyThreadUnreadDelta', () => {
  it('按会话的未读封数调整聚合角标', () => {
    qc.setQueryData(['aggregate-counts'], { inbox: 10, unread: 10, starred: 0, inboxTotal: 50 })
    qc.setQueryData(['account-unread'], { 1: 10 })
    applyThreadUnreadDelta(qc, [thread('t1', { unread: 3 })], -1)
    expect(qc.getQueryData(['aggregate-counts'])).toMatchObject({ inbox: 7, unread: 7 })
    expect(qc.getQueryData(['account-unread'])).toEqual({ 1: 7 })
  })

  it('unread 为 0 的会话不动角标', () => {
    qc.setQueryData(['account-unread'], { 1: 10 })
    applyThreadUnreadDelta(qc, [thread('t1', { unread: 0 })], -1)
    expect(qc.getQueryData(['account-unread'])).toEqual({ 1: 10 })
  })

  it('角标不会被扣成负数', () => {
    qc.setQueryData(['account-unread'], { 1: 1 })
    applyThreadUnreadDelta(qc, [thread('t1', { unread: 5 })], -1)
    expect(qc.getQueryData(['account-unread'])).toEqual({ 1: 0 })
  })
})

describe('单封操作覆盖会话成员列表', () => {
  // 手风琴里的成员列表是裸数组、挂在 ['thread-messages'] 下；
  // 单封星标/已读/删除必须同时改到它，否则要等 refetch 才变。
  it('patchMessages 同时改到成员列表', () => {
    qc.setQueryData(['messages', 1, 'none'], { pages: [{ messages: [msg(1)] }], pageParams: [0] })
    qc.setQueryData(['thread-messages', 't1'], [msg(1), msg(2)])
    patchMessages(qc, new Set([1]), { flagged: true })
    const members = qc.getQueryData(['thread-messages', 't1']) as MessageListItem[]
    expect(members[0].flagged).toBe(true)
    expect(members[1].flagged).toBe(false)
  })

  it('removeMessages 同时从成员列表里摘掉', () => {
    qc.setQueryData(['thread-messages', 't1'], [msg(1), msg(2)])
    removeMessages(qc, new Set([2]))
    expect((qc.getQueryData(['thread-messages', 't1']) as MessageListItem[]).map((m) => m.id)).toEqual([1])
  })
})

describe('snapshotMail / restoreMail', () => {
  it('回滚把会话与成员列表一起还原', () => {
    qc.setQueryData(['threads', 1, 'none'], threadPages([thread('t1')]))
    qc.setQueryData(['thread-messages', 't1'], [msg(1)])
    const snap = snapshotMail(qc)

    removeThreads(qc, new Set(['t1']))
    removeMessages(qc, new Set([1]))
    restoreMail(qc, snap)

    const d = qc.getQueryData(['threads', 1, 'none']) as ReturnType<typeof threadPages>
    expect(d.pages[0].threads.length).toBe(1)
    expect((qc.getQueryData(['thread-messages', 't1']) as MessageListItem[]).length).toBe(1)
  })

  it('快照为 undefined 时是安全的空操作', () => {
    expect(() => restoreMail(qc, undefined)).not.toThrow()
  })
})

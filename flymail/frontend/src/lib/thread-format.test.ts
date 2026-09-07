import { describe, it, expect } from 'vitest'
import {
  commonAccountId,
  defaultExpanded,
  formatParticipants,
  pickAvatarParticipant,
  selectedThreads,
} from '@/lib/thread-format'
import type { Address, ThreadListItem } from '@/lib/types'

function addr(name: string, email: string): Address {
  return { name, email }
}

function thread(id: string, accountId: number): ThreadListItem {
  return {
    thread_id: id,
    account_id: accountId,
    count: 1,
    unread: 0,
    flagged: false,
    has_attachment: false,
    subject: 's',
    snippet: '',
    date: '2026-09-01T00:00:00Z',
    latest_id: 1,
    latest_folder_id: 1,
    participants: [],
  }
}

describe('formatParticipants', () => {
  it('单人只显示一人且没有「等 N 人」', () => {
    expect(formatParticipants([addr('张三', 'a@b.com')])).toEqual({ names: ['张三'], extra: 0 })
  })

  it('按邮箱去重，忽略大小写与空白', () => {
    const r = formatParticipants([
      addr('张三', 'A@b.com'),
      addr('Zhang San', ' a@B.com '),
      addr('李四', 'c@d.com'),
    ])
    expect(r).toEqual({ names: ['张三', '李四'], extra: 0 })
  })

  it('超过 3 人时截断并给出剩余人数', () => {
    const r = formatParticipants([
      addr('A', 'a@x.com'),
      addr('B', 'b@x.com'),
      addr('C', 'c@x.com'),
      addr('D', 'd@x.com'),
      addr('E', 'e@x.com'),
    ])
    expect(r.names).toEqual(['A', 'B', 'C'])
    expect(r.extra).toBe(2)
  })

  it('没有显示名时回落到邮箱', () => {
    expect(formatParticipants([addr('', 'a@b.com')]).names).toEqual(['a@b.com'])
  })

  it('没有邮箱时按名字去重', () => {
    const r = formatParticipants([addr('张三', ''), addr('张三', ''), addr('李四', '')])
    expect(r.names).toEqual(['张三', '李四'])
  })

  it('名字与邮箱都为空的条目直接丢弃', () => {
    expect(formatParticipants([addr('', ''), addr('张三', 'a@b.com')]).names).toEqual(['张三'])
  })

  it('空列表安全', () => {
    expect(formatParticipants([])).toEqual({ names: [], extra: 0 })
  })

  it('max 可调', () => {
    const r = formatParticipants([addr('A', 'a@x.com'), addr('B', 'b@x.com')], 1)
    expect(r).toEqual({ names: ['A'], extra: 1 })
  })
})

describe('selectedThreads', () => {
  it('按列表顺序取出选中项', () => {
    const items = [thread('t1', 1), thread('t2', 1), thread('t3', 1)]
    const got = selectedThreads(items, new Set(['t3', 't1']))
    expect(got.map((t) => t.thread_id)).toEqual(['t1', 't3'])
  })

  it('选中集合里有列表外的 id 时忽略', () => {
    expect(selectedThreads([thread('t1', 1)], new Set(['zzz'])).length).toBe(0)
  })
})

describe('commonAccountId', () => {
  it('同账户返回该账户 id', () => {
    expect(commonAccountId([thread('t1', 7), thread('t2', 7)])).toBe(7)
  })

  it('跨账户返回 null', () => {
    expect(commonAccountId([thread('t1', 7), thread('t2', 8)])).toBeNull()
  })

  it('空列表返回 null', () => {
    expect(commonAccountId([])).toBeNull()
  })
})

describe('defaultExpanded', () => {
  const msgs = [
    { id: 1, seen: true },
    { id: 2, seen: false },
    { id: 3, seen: true },
  ]

  it('展开范围内最新一封与所有未读', () => {
    expect([...defaultExpanded(msgs, 3)].sort()).toEqual([2, 3])
  })

  it('latestId 不在成员里且有未读时只展开未读', () => {
    expect([...defaultExpanded(msgs, 999)]).toEqual([2])
  })

  it('全部已读且 latestId 无效时退回最后一封', () => {
    const read = [{ id: 1, seen: true }, { id: 2, seen: true }]
    expect([...defaultExpanded(read, null)]).toEqual([2])
  })

  it('空成员列表返回空集合', () => {
    expect(defaultExpanded([], 1).size).toBe(0)
  })
})

describe('pickAvatarParticipant', () => {
  const self = new Set(['me@x.com'])

  it('跳过本人，取第一个他人', () => {
    const got = pickAvatarParticipant([addr('我', 'me@x.com'), addr('张三', 'a@b.com')], self)
    expect(got?.email).toBe('a@b.com')
  })

  it('本人邮箱大小写/空白不同也能跳过', () => {
    const got = pickAvatarParticipant([addr('Me', ' ME@X.com '), addr('张三', 'a@b.com')], self)
    expect(got?.email).toBe('a@b.com')
  })

  it('全是本人时退回第一个', () => {
    const got = pickAvatarParticipant([addr('我', 'me@x.com')], self)
    expect(got?.email).toBe('me@x.com')
  })

  it('本人集合为空时就是第一个', () => {
    const got = pickAvatarParticipant([addr('张三', 'a@b.com')], new Set())
    expect(got?.email).toBe('a@b.com')
  })

  it('没有参与者时返回 null', () => {
    expect(pickAvatarParticipant([], self)).toBeNull()
  })
})

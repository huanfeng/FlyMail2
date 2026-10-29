import { describe, it, expect } from 'vitest'
import { selectMessagesBy, selectThreadsBy, selectionState } from './select-by'
import type { MessageListItem, ThreadListItem } from './types'

const m = (id: number, seen: boolean, flagged: boolean) => ({ id, seen, flagged }) as MessageListItem
const th = (thread_id: string, unread: number, flagged: boolean) => ({ thread_id, unread, flagged }) as ThreadListItem

describe('selectMessagesBy', () => {
  const list = [m(1, true, false), m(2, false, true), m(3, false, false)]
  it.each([
    ['all', [1, 2, 3]],
    ['none', []],
    ['read', [1]],
    ['unread', [2, 3]],
    ['flagged', [2]],
    ['unflagged', [1, 3]],
  ] as const)('%s', (c, ids) => {
    expect(selectMessagesBy(list, c)).toEqual(ids)
  })
})

describe('selectThreadsBy', () => {
  const list = [th('a', 0, false), th('b', 2, true)]
  it('会话未读按 unread > 0 判定，返回字符串 id', () => {
    expect(selectThreadsBy(list, 'unread')).toEqual(['b'])
    expect(selectThreadsBy(list, 'read')).toEqual(['a'])
    expect(selectThreadsBy(list, 'flagged')).toEqual(['b'])
    expect(selectThreadsBy(list, 'unflagged')).toEqual(['a'])
  })
})

describe('selectionState', () => {
  it('三态', () => {
    expect(selectionState(0, 5)).toBe('none')
    expect(selectionState(2, 5)).toBe('some')
    expect(selectionState(5, 5)).toBe('all')
    expect(selectionState(0, 0)).toBe('none')
  })
})

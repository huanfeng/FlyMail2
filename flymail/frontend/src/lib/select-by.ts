// 列表「按条件选择」的判定。只作用于已加载的可见行——与「全选当前可见」同口径：
// 未翻到的页不在内存里，选中它们意味着对看不见的邮件做批量操作。
import type { MessageListItem, ThreadListItem } from './types'

export type SelectCondition = 'all' | 'none' | 'read' | 'unread' | 'flagged' | 'unflagged'

function match(c: SelectCondition, unread: boolean, flagged: boolean): boolean {
  switch (c) {
    case 'all': return true
    case 'none': return false
    case 'read': return !unread
    case 'unread': return unread
    case 'flagged': return flagged
    case 'unflagged': return !flagged
  }
}

export function selectMessagesBy(list: MessageListItem[], c: SelectCondition): number[] {
  return list.filter((x) => match(c, !x.seen, x.flagged)).map((x) => x.id)
}

/** 会话：任一成员未读即视为未读（与列表加粗口径一致） */
export function selectThreadsBy(list: ThreadListItem[], c: SelectCondition): string[] {
  return list.filter((x) => match(c, x.unread > 0, x.flagged)).map((x) => x.thread_id)
}

/** 全选框三态 */
export function selectionState(selected: number, visible: number): 'none' | 'some' | 'all' {
  if (selected === 0 || visible === 0) return 'none'
  return selected >= visible ? 'all' : 'some'
}

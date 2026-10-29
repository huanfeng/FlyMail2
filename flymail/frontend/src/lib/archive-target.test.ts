import { describe, it, expect } from 'vitest'
import { archiveFolderOf, archiveTargetForMessage, archiveTargetForThread } from './archive-target'
import type { Folder } from './types'

function f(id: number, account_id: number, type: string, selectable = true): Folder {
  return { id, account_id, path: String(id), display_name: String(id), type, selectable, total_count: 0, unread_count: 0, sort_order: id }
}

describe('archive-target', () => {
  const byAcct = new Map<number, Folder[]>([
    [1, [f(10, 1, 'inbox'), f(11, 1, 'archive')]],
    [2, [f(20, 2, 'inbox')]],
    [3, [f(30, 3, 'archive', false)]],
  ])

  it('取可选的归档文件夹，不可选的不算', () => {
    expect(archiveFolderOf(byAcct.get(1)!)?.id).toBe(11)
    expect(archiveFolderOf(byAcct.get(3)!)).toBeNull()
  })

  it('聚合视图：每行按自己的账户判断', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 1, folder_id: 10 })).toBe(11)
    expect(archiveTargetForMessage(byAcct, { account_id: 2, folder_id: 20 })).toBeNull()
  })

  it('已在归档里的单封不再提供归档', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 1, folder_id: 11 })).toBeNull()
  })

  it('账户文件夹尚未加载时不提供（缺省隐藏而不是点了报错）', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 9, folder_id: 1 })).toBeNull()
    expect(archiveTargetForThread(byAcct, { account_id: 9 })).toBeNull()
  })

  it('会话只看账户有无归档文件夹', () => {
    expect(archiveTargetForThread(byAcct, { account_id: 1 })).toBe(11)
    expect(archiveTargetForThread(byAcct, { account_id: 2 })).toBeNull()
  })
})

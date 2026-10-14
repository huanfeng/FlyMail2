import { describe, it, expect } from 'vitest'
import { splitFolders } from '@/lib/sidebar-folders'

const f = (id: number, type: string, unread = 0) => ({ id, type, unread_count: unread })

describe('splitFolders', () => {
  it('custom 少于阈值时不折叠', () => {
    const r = splitFolders([f(1, 'inbox'), f(2, 'custom'), f(3, 'custom')], null, false)
    expect(r.foldable).toBe(false)
    expect(r.visible.map((x) => x.id)).toEqual([1, 2, 3])
  })
  it('custom 达到阈值且未展开时折叠，系统文件夹常显', () => {
    const r = splitFolders([f(1, 'inbox'), f(2, 'trash'), f(3, 'custom', 2), f(4, 'custom'), f(5, 'custom', 5)], null, false)
    expect(r.foldable).toBe(true)
    expect(r.visible.map((x) => x.id)).toEqual([1, 2])
    expect(r.hiddenCount).toBe(3)
    expect(r.hiddenUnread).toBe(7)
  })
  it('激活的 custom 即使折叠也常显，且不计入隐藏数', () => {
    const r = splitFolders([f(1, 'inbox'), f(3, 'custom', 2), f(4, 'custom'), f(5, 'custom')], 4, false)
    expect(r.visible.map((x) => x.id)).toEqual([1, 4])
    expect(r.hiddenCount).toBe(2)
    expect(r.hiddenUnread).toBe(2)
  })
  it('展开时全部显示', () => {
    const r = splitFolders([f(1, 'inbox'), f(3, 'custom'), f(4, 'custom'), f(5, 'custom')], null, true)
    expect(r.visible.length).toBe(4)
    expect(r.foldable).toBe(true)
    expect(r.hiddenCount).toBe(0)
  })
})

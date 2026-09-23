import { describe, it, expect } from 'vitest'
import { moveItem } from '@/lib/reorder'

describe('moveItem', () => {
  const list = ['a', 'b', 'c'] as const

  it('上移与下移', () => {
    expect(moveItem(list, 1, -1)).toEqual(['b', 'a', 'c'])
    expect(moveItem(list, 1, 1)).toEqual(['a', 'c', 'b'])
  })

  it('跨多格移动', () => {
    expect(moveItem(list, 0, 2)).toEqual(['b', 'c', 'a'])
    expect(moveItem(list, 2, -2)).toEqual(['c', 'a', 'b'])
  })

  // 越界返回原数组本身（不是内容相等的副本），调用方据此判断「什么都没发生」
  // 从而不发请求。返回副本的话，首行点上移也会发一次把顺序原样写回去的请求。
  it('越界时原样返回入参，且可用 === 判定', () => {
    expect(moveItem(list, 0, -1)).toBe(list)
    expect(moveItem(list, 2, 1)).toBe(list)
    expect(moveItem(list, -1, 1)).toBe(list)
    expect(moveItem(list, 5, -1)).toBe(list)
    expect(moveItem([], 0, 1)).toEqual([])
    expect(moveItem(list, 1, 0)).toBe(list)
  })

  // 调用方拿返回值做乐观更新，而 react-query 缓存里那个数组不能就地改：
  // 改了之后 onError 回滚时，手里的「旧值」也已经是改过的了。
  it('不修改入参', () => {
    const src = ['a', 'b', 'c']
    const snapshot = [...src]
    moveItem(src, 0, 2)
    expect(src).toEqual(snapshot)
  })
})

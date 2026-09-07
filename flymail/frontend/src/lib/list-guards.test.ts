import { describe, it, expect } from 'vitest'
import { createAutoReadGate, shouldLoadMore } from '@/lib/list-guards'
import type { LoadMoreInput } from '@/lib/list-guards'

// list-guards.ts 单元测试
// 覆盖 React error #185（Maximum update depth exceeded）的两条自激路径。

describe('createAutoReadGate()', () => {
  it('打开未读邮件时放行一次', () => {
    const gate = createAutoReadGate()
    expect(gate.shouldSend(1, true)).toBe(true)
  })

  it('同一封连续判定只放行第一次（乐观更新尚未落地时不会重发）', () => {
    const gate = createAutoReadGate()
    expect(gate.shouldSend(1, true)).toBe(true)
    // 后续几次渲染里缓存仍是 seen=false —— 正是原来死循环的窗口
    expect(gate.shouldSend(1, true)).toBe(false)
    expect(gate.shouldSend(1, true)).toBe(false)
  })

  it('列表回填把 seen 刷回未读也不重发', () => {
    const gate = createAutoReadGate()
    gate.shouldSend(1, true)
    expect(gate.shouldSend(1, false)).toBe(false) // 乐观更新生效
    expect(gate.shouldSend(1, true)).toBe(false)  // refetch 把 seen 打回未读
  })

  it('已读邮件不发请求，且不占用闸门', () => {
    const gate = createAutoReadGate()
    expect(gate.shouldSend(1, false)).toBe(false)
    // 列表数据后到，这封其实是未读 —— 仍应放行
    expect(gate.shouldSend(1, true)).toBe(true)
  })

  it('换到另一封邮件重新放行', () => {
    const gate = createAutoReadGate()
    expect(gate.shouldSend(1, true)).toBe(true)
    expect(gate.shouldSend(2, true)).toBe(true)
  })

  it('关闭阅读器后复位，再打开同一封可重新放行', () => {
    const gate = createAutoReadGate()
    expect(gate.shouldSend(1, true)).toBe(true)
    expect(gate.shouldSend(null, false)).toBe(false)
    expect(gate.shouldSend(1, true)).toBe(true)
  })
})

describe('shouldLoadMore()', () => {
  const base: LoadMoreInput = {
    lastIndex: 45,
    rowCount: 50,
    messageCount: 50,
    lastLoadedCount: -1,
    hasNextPage: true,
    isFetchingNextPage: false,
  }

  it('接近底部且还有下一页时触发', () => {
    expect(shouldLoadMore(base)).toBe(true)
  })

  it('没有下一页时不触发', () => {
    expect(shouldLoadMore({ ...base, hasNextPage: false })).toBe(false)
  })

  it('已在请求中时不触发', () => {
    expect(shouldLoadMore({ ...base, isFetchingNextPage: true })).toBe(false)
  })

  it('离底部还远时不触发', () => {
    expect(shouldLoadMore({ ...base, lastIndex: 10 })).toBe(false)
  })

  it('上一轮翻页没带回新邮件时不再原地重试', () => {
    expect(shouldLoadMore({ ...base, lastLoadedCount: 50, messageCount: 50 })).toBe(false)
  })

  it('底层数据增长后允许再次翻页', () => {
    expect(shouldLoadMore({ ...base, lastLoadedCount: 50, messageCount: 100 })).toBe(true)
  })

  it('未读筛选下行数极少使接近底部恒成立，但同一批数据只翻一次', () => {
    // 100 封里只有 3 封未读 → rowCount=3、lastIndex=2，lastIndex >= rowCount-5 恒成立
    const unreadFiltered: LoadMoreInput = {
      ...base,
      lastIndex: 2,
      rowCount: 3,
      messageCount: 100,
      lastLoadedCount: -1,
    }
    expect(shouldLoadMore(unreadFiltered)).toBe(true)
    // 触发后记录 messageCount=100；新页尚未到达时的重渲染不得再次触发
    expect(shouldLoadMore({ ...unreadFiltered, lastLoadedCount: 100 })).toBe(false)
    // 新页到达（底层 +50）但一封未读都没有：rowCount 仍是 3，允许继续往后翻
    expect(shouldLoadMore({ ...unreadFiltered, lastLoadedCount: 100, messageCount: 150 })).toBe(true)
  })
})

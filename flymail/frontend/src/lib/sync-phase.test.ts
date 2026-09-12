import { describe, it, expect } from 'vitest'
import { isSyncActive } from '@/lib/types'
import type { SyncPhase } from '@/lib/types'

/**
 * 「同步还在进行吗」这个判据。
 *
 * 它同时决定三件事，所以判错一次会同时出三个症状：
 * 1. 账户行上的圆点转不转
 * 2. 进度条显不显示
 * 3. **每秒一次的轮询什么时候停**
 *
 * 第 3 点最要命：Shell 里把 `syncingAccountId` 置回 null 的 effect 只认 done / error，
 * 而这个判据决定进度条的可见性——两者对「进行中」的理解必须一致，
 * 否则要么进度条早早消失而轮询还在跑，要么反过来。
 */
describe('isSyncActive', () => {
  it('排队也算进行中', () => {
    // 后端在等全局同步名额时 phase 是 'queued'。这个值曾经根本不在前端的
    // SyncPhase 联合类型里，判据写的是 folders | messages——于是用户按下「同步」
    // 却排上队时，屏幕上什么都不发生。后端那边的注释写着「前端未识别按进行中展示」，
    // 那正是没有兑现的部分。
    expect(isSyncActive('queued')).toBe(true)
  })

  it('抓文件夹和抓邮件都算进行中', () => {
    expect(isSyncActive('folders')).toBe(true)
    expect(isSyncActive('messages')).toBe(true)
  })

  it('完成、失败、从未同步都不算', () => {
    expect(isSyncActive('done')).toBe(false)
    expect(isSyncActive('error')).toBe(false)
    expect(isSyncActive('none')).toBe(false)
  })

  it('状态还没拿到时不算', () => {
    // 刚触发、第一次轮询还没回来的那一刻。按「进行中」处理会让进度条闪一下又消失。
    expect(isSyncActive(undefined)).toBe(false)
  })

  it('覆盖了 SyncPhase 的每一个取值', () => {
    // 后端加了新阶段而这里忘了跟进时，新值会落进「不算进行中」那一支——
    // 症状是同步跑着而界面上毫无表示，正是第 7 条本身。
    // 这条测试不能自动发现新值（联合类型在运行时不存在），但把清单摆在这里，
    // 改 SyncPhase 时 tsc 会因为这个数组的类型标注而要求同步更新。
    const all: SyncPhase[] = ['none', 'queued', 'folders', 'messages', 'done', 'error']
    const active = all.filter(isSyncActive)
    expect(active).toEqual(['queued', 'folders', 'messages'])
  })
})

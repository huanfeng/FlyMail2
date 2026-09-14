import { describe, it, expect, beforeEach } from 'vitest'
import {
  LAYOUT_DEFAULTS,
  LAYOUT_LIMITS,
  LAYOUT_LS_KEY,
  clampWidth,
  loadLayoutWidths,
  saveLayoutWidths,
} from '@/lib/layout-prefs'

describe('布局宽度偏好', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('只写自己那份，不碰别人的', () => {
    // 这几个宽度有两个所有者：AppLayout 管 sidebar/list/slide，MailList 管 senderCol，
    // 各自带防抖。谁要是按自己手上的快照写整份对象，就会把另一方在这段窗口里的
    // 改动覆盖掉——可复现路径是先拖发件人列、在它的防抖到期前去拖侧栏，
    // 侧栏会在拖拽中途弹回旧宽度。
    saveLayoutWidths({ sidebar: 300, list: 400, slide: 800 })
    saveLayoutWidths({ senderCol: 200 })

    const w = loadLayoutWidths()
    expect(w.senderCol).toBe(200)
    expect(w.sidebar).toBe(300)
    expect(w.list).toBe(400)
    expect(w.slide).toBe(800)
  })

  it('广播出去的是合并后的完整宽度', () => {
    saveLayoutWidths({ sidebar: 300 })

    let got: unknown = null
    const onEvent = (e: Event) => {
      got = (e as CustomEvent).detail
    }
    window.addEventListener('flymail:layout-changed', onEvent)
    saveLayoutWidths({ senderCol: 111 })
    window.removeEventListener('flymail:layout-changed', onEvent)

    // 监听方（AppLayout / MailList）直接吸收 detail，缺字段会把对方的值抹成 undefined
    expect(got).toEqual({ ...LAYOUT_DEFAULTS, sidebar: 300, senderCol: 111 })
  })

  it('读回来的值夹在约束区间内', () => {
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify({ sidebar: 9999, list: -5 }))
    const w = loadLayoutWidths()
    expect(w.sidebar).toBe(LAYOUT_LIMITS.sidebar.max)
    expect(w.list).toBe(LAYOUT_LIMITS.list.min)
    // 没写进去的项回落默认值
    expect(w.senderCol).toBe(LAYOUT_DEFAULTS.senderCol)
  })

  it('存储损坏时回落默认值而不是抛出', () => {
    localStorage.setItem(LAYOUT_LS_KEY, '{ 这不是 JSON')
    expect(loadLayoutWidths()).toEqual(LAYOUT_DEFAULTS)
  })

  it('clampWidth 夹紧到两端', () => {
    expect(clampWidth(50, 100, 200)).toBe(100)
    expect(clampWidth(300, 100, 200)).toBe(200)
    expect(clampWidth(150, 100, 200)).toBe(150)
  })

  /**
   * ── 宽度必须是整数 ───────────────────────────────────────────────────────
   *
   * 分数缩放的显示器上 `ev.clientX` 带小数，拖拽是「状态 + dx」逐次累加的，
   * 于是宽度会变成 503.000003242。它不只是难看：设置里那一行把它原样渲染成
   * `503.000003242px`，把设置行撑宽、逼出横向滚动条（用户报的第 4 条）。
   *
   * 取整**不能**放进 clampWidth：那里是累加器的出口，小于 0.5px 的位移会被
   * 反复抹成 0，拖拽在分数缩放下彻底卡死。真正的修法是在 ResizeHandle 里只
   * 发整数增量、余量留到下次（见 ResizeHandle.test.tsx）。这里守的是另外两道：
   * 写入时取整、读取时自愈。
   */
  it('写进去的小数被取整', () => {
    saveLayoutWidths({ list: 503.000003242, sidebar: 247.6 })
    const raw = JSON.parse(localStorage.getItem(LAYOUT_LS_KEY)!)
    expect(raw.list, '小数落进了 localStorage').toBe(503)
    expect(raw.sidebar).toBe(248)
  })

  it('已被污染的旧值在读取时自愈', () => {
    // 用户手上那份 localStorage 已经是脏的了。只在写入侧取整的话，
    // 得等他重新拖一次才恢复正常——横向滚动条在那之前一直在。
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify({ list: 503.000003242 }))
    expect(loadLayoutWidths().list).toBe(503)
  })

  it('取整发生在夹紧之后，不会被顶出区间', () => {
    // 先取整再夹紧的话，上限是 680 而值是 680.4 → 取整成 680、没问题；
    // 但下限 300 遇到 299.6 会取整成 300 后再夹紧仍是 300——巧合正确。
    // 真正会出事的是 max 为奇数小数的情形，所以顺序固定成「夹紧 → 取整」。
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify({ list: 99999.7, sidebar: -0.4 }))
    const w = loadLayoutWidths()
    expect(w.list).toBe(LAYOUT_LIMITS.list.max)
    expect(w.sidebar).toBe(LAYOUT_LIMITS.sidebar.min)
    expect(Number.isInteger(w.list) && Number.isInteger(w.sidebar)).toBe(true)
  })
})

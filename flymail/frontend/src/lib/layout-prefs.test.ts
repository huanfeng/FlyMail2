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
})

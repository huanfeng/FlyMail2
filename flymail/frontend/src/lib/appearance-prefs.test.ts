import { describe, it, expect, beforeEach } from 'vitest'
import {
  APPEARANCE_DEFAULTS, getAppearance, setAppearance, resetAppearance,
  subscribeAppearance, applyAppearance,
} from './appearance-prefs'

const KEY = 'flymail_appearance'

describe('appearance-prefs', () => {
  beforeEach(() => { localStorage.clear(); setAppearance({}) })

  it('未存过返回默认值', () => {
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
  })

  it('非法值逐项回落默认，合法项保留', () => {
    localStorage.setItem(KEY, JSON.stringify({ uiScale: 175, textContrast: 'high', bodyZoom: 'x' }))
    setAppearance({})
    const a = getAppearance()
    expect(a.uiScale).toBe(100)
    expect(a.textContrast).toBe('high')
    expect(a.bodyZoom).toBe(100)
  })

  it('损坏的 JSON 回落默认而不抛', () => {
    localStorage.setItem(KEY, '{oops')
    setAppearance({})
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
  })

  it('快照稳定：未写入时两次读取是同一引用（useSyncExternalStore 契约）', () => {
    expect(getAppearance()).toBe(getAppearance())
  })

  it('写入后引用变化、通知订阅者、持久化', () => {
    const before = getAppearance()
    let n = 0
    const off = subscribeAppearance(() => n++)
    setAppearance({ bodyFontSize: 16 })
    off()
    expect(getAppearance()).not.toBe(before)
    expect(getAppearance().bodyFontSize).toBe(16)
    expect(n).toBe(1)
    expect(JSON.parse(localStorage.getItem(KEY)!).bodyFontSize).toBe(16)
  })

  it('resetAppearance 只重置指定键', () => {
    setAppearance({ bodyFontSize: 16, uiScale: 110 })
    resetAppearance(['bodyFontSize'])
    expect(getAppearance().bodyFontSize).toBe(14)
    expect(getAppearance().uiScale).toBe(110)
  })

  it('applyAppearance 写 data 属性与 zoom；默认值时不留残余', () => {
    const el = document.createElement('div')
    applyAppearance({ ...APPEARANCE_DEFAULTS, uiScale: 110, textContrast: 'high', motion: 'reduce' }, el)
    expect(el.style.zoom).toBe('1.1')
    expect(el.dataset.contrast).toBe('high')
    expect(el.dataset.motion).toBe('reduce')
    applyAppearance(APPEARANCE_DEFAULTS, el)
    expect(el.style.zoom).toBe('')
    expect(el.dataset.contrast).toBeUndefined()
    expect(el.dataset.motion).toBeUndefined()
  })
})

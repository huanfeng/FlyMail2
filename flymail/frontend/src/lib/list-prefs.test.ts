import { describe, it, expect, beforeEach } from 'vitest'
import { getListStyle, setListStyle } from '@/lib/list-prefs'

// list-prefs.ts 单元测试
// 环境：jsdom（vitest.config.ts 已配置 environment: 'jsdom'）

beforeEach(() => {
  localStorage.clear()
})

describe('getListStyle()', () => {
  it('localStorage 清空后返回默认值 compact', () => {
    expect(getListStyle()).toBe('compact')
  })

  it('非法值回落到 compact', () => {
    localStorage.setItem('flymail_list_style', 'invalid')
    expect(getListStyle()).toBe('compact')
  })

  it('空字符串回落到 compact', () => {
    localStorage.setItem('flymail_list_style', '')
    expect(getListStyle()).toBe('compact')
  })
})

describe('setListStyle()', () => {
  it('写入 compact 后读取返回 compact', () => {
    setListStyle('compact')
    expect(getListStyle()).toBe('compact')
  })

  it('写入 card 后读取返回 card', () => {
    setListStyle('card')
    expect(getListStyle()).toBe('card')
  })

  it('可以在 compact 和 card 之间切换', () => {
    setListStyle('card')
    expect(getListStyle()).toBe('card')
    setListStyle('compact')
    expect(getListStyle()).toBe('compact')
  })
})

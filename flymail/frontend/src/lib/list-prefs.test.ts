import { describe, it, expect, beforeEach } from 'vitest'
import {
  getAlwaysShowSelect,
  getConversationView,
  getListStyle,
  setAlwaysShowSelect,
  setConversationView,
  setListStyle,
} from '@/lib/list-prefs'

beforeEach(() => {
  localStorage.clear()
})

describe('getConversationView', () => {
  // 判据刻意写成「只有显式存过 'false' 才关」：没存过的老用户升级上来
  // 直接进会话视图，而不是被 localStorage 的空值判成关闭。
  it('从未存过时默认开启', () => {
    expect(getConversationView()).toBe(true)
  })

  it('存过 false 才关闭', () => {
    setConversationView(false)
    expect(getConversationView()).toBe(false)
  })

  it('再存回 true 就重新开启', () => {
    setConversationView(false)
    setConversationView(true)
    expect(getConversationView()).toBe(true)
  })

  it('存着无法识别的值时按开启处理', () => {
    localStorage.setItem('flymail_conversation_view', 'maybe')
    expect(getConversationView()).toBe(true)
  })

  it('空串不等于 false，仍然开启', () => {
    localStorage.setItem('flymail_conversation_view', '')
    expect(getConversationView()).toBe(true)
  })
})

describe('getListStyle', () => {
  it('默认紧凑', () => {
    expect(getListStyle()).toBe('compact')
  })

  it('存过的合法值原样读回', () => {
    setListStyle('card')
    expect(getListStyle()).toBe('card')
  })

  it('非法值回落到紧凑', () => {
    localStorage.setItem('flymail_list_style', 'huge')
    expect(getListStyle()).toBe('compact')
  })
})

describe('getAlwaysShowSelect', () => {
  // 与会话视图相反：这一项默认关，所以判据是「只有显式存过 'true' 才开」
  it('默认关闭', () => {
    expect(getAlwaysShowSelect()).toBe(false)
  })

  it('存过 true 才开启', () => {
    setAlwaysShowSelect(true)
    expect(getAlwaysShowSelect()).toBe(true)
  })
})

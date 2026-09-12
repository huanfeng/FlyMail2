import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  NOTIFY_DEFAULTS,
  getNotifyPrefs,
  setNotifyPrefs,
  subscribeNotifyPrefs,
  resetNotifyPrefsCache,
} from '@/lib/notify-prefs'

describe('浏览器通知偏好', () => {
  beforeEach(() => {
    localStorage.clear()
    resetNotifyPrefsCache()
  })

  it('默认不开桌面通知与声音，但开标签页角标', () => {
    const p = getNotifyPrefs()
    // 前两项要权限或会出声，不能替用户做主；角标什么都不需要，默认开着才有用
    expect(p.desktop).toBe(false)
    expect(p.sound).toBe(false)
    expect(p.titleBadge).toBe(true)
  })

  it('只写指定项，其余保持不变', () => {
    setNotifyPrefs({ sound: true })
    const p = getNotifyPrefs()
    expect(p.sound).toBe(true)
    expect(p.titleBadge).toBe(NOTIFY_DEFAULTS.titleBadge)
  })

  it('变更会通知订阅者', () => {
    const fn = vi.fn()
    const unsub = subscribeNotifyPrefs(fn)
    setNotifyPrefs({ desktop: true })
    expect(fn).toHaveBeenCalledTimes(1)
    unsub()
    setNotifyPrefs({ desktop: false })
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('存储损坏时回落默认值而不是抛出', () => {
    localStorage.setItem('flymail-notify-prefs-v1', '{ 这不是 JSON')
    resetNotifyPrefsCache()
    expect(getNotifyPrefs()).toEqual(NOTIFY_DEFAULTS)
  })

  it('localStorage 写不进去时，本次会话内的选择仍然生效', () => {
    // 隐私模式下 setItem 会抛。选择要么立刻生效、要么明确失败，
    // 不能出现「点了开关但界面没反应」这种第三种状态。
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceeded')
    })
    expect(() => setNotifyPrefs({ sound: true })).not.toThrow()
    expect(getNotifyPrefs().sound).toBe(true)
    spy.mockRestore()
  })
})

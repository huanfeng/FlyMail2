import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  getRemoteImageDefault,
  setRemoteImageDefault,
  subscribePrivacyPrefs,
} from '@/lib/privacy-prefs'

// 这个开关参与 useMessageDetail 的 query key，也是「打开邮件会不会向发件人发请求」
// 的总闸门。两件事必须成立：默认关闭（失败方向偏保守），以及改动能通知到订阅者
// （否则已挂载的详情查询不会换 key，开关看着生效实际没生效）。

describe('privacy-prefs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('默认关闭', () => {
    expect(getRemoteImageDefault()).toBe(false)
  })

  it('只有字符串 "true" 才算打开', () => {
    localStorage.setItem('flymail_load_remote_images', 'false')
    expect(getRemoteImageDefault()).toBe(false)
    localStorage.setItem('flymail_load_remote_images', '1')
    expect(getRemoteImageDefault()).toBe(false)
    localStorage.setItem('flymail_load_remote_images', 'true')
    expect(getRemoteImageDefault()).toBe(true)
  })

  it('写入后读得到', () => {
    setRemoteImageDefault(true)
    expect(getRemoteImageDefault()).toBe(true)
    setRemoteImageDefault(false)
    expect(getRemoteImageDefault()).toBe(false)
  })

  it('localStorage 读写抛异常时按关闭处理，且不把异常抛给调用方', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(getRemoteImageDefault()).toBe(false)
    expect(() => setRemoteImageDefault(true)).not.toThrow()
  })

  it('写入会通知订阅者，取消订阅后不再收到', () => {
    const seen: boolean[] = []
    const unsubscribe = subscribePrivacyPrefs(() => seen.push(getRemoteImageDefault()))

    setRemoteImageDefault(true)
    setRemoteImageDefault(false)
    expect(seen).toEqual([true, false])

    unsubscribe()
    setRemoteImageDefault(true)
    expect(seen).toEqual([true, false])
  })

  it('存储不可用导致写入失败时同样通知：本次会话内的选择仍要立刻生效', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    const fn = vi.fn()
    const unsubscribe = subscribePrivacyPrefs(fn)
    setRemoteImageDefault(true)
    expect(fn).toHaveBeenCalledTimes(1)
    unsubscribe()
  })
})

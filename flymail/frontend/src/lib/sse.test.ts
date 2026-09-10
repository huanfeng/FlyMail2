import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { RealtimeEvent } from '@/lib/types'

// ── mock @/lib/api（必须在 import 被测模块之前） ──
const post = vi.fn()
vi.mock('@/lib/api', () => ({
  default: {
    post: (url: string) => post(url),
  },
}))

// ── mock @/lib/auth ──
// sse.ts 现在**不该**再直接碰 auth：token 刷新已经统一交给 api 拦截器。
// 这个 mock 是哨兵——谁把 access token 拼回 SSE 的 URL，断言就会看到 TOK123。
vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK123', refresh: 'REFRESH' },
}))

import { connectRealtime } from '@/lib/sse'

/** 记录构造出来的 EventSource 实例，供测试驱动 onmessage / onerror */
class FakeEventSource {
  static instances: FakeEventSource[] = []
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onopen: (() => void) | null = null
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  close() {
    this.closed = true
  }
}

beforeEach(() => {
  FakeEventSource.instances = []
  post.mockReset()
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

/** 让取票的 Promise 与随后的 setTimeout 都跑完 */
async function settle(ms = 0) {
  await vi.advanceTimersByTimeAsync(ms)
}

describe('connectRealtime', () => {
  it('先取票，再用 ticket 连接；URL 里不出现 access token', async () => {
    post.mockResolvedValue({ data: { ticket: 'TK-1', expires_in: 60 } })
    const close = connectRealtime(() => {})
    await settle()

    expect(post).toHaveBeenCalledWith('/events/ticket')
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0].url).toBe('/api/v1/events?ticket=TK-1')
    expect(FakeEventSource.instances[0].url).not.toContain('access_token')
    expect(FakeEventSource.instances[0].url).not.toContain('TOK123')
    close()
  })

  it('票据里的特殊字符被转义', async () => {
    post.mockResolvedValue({ data: { ticket: 'a b&c', expires_in: 60 } })
    const close = connectRealtime(() => {})
    await settle()
    expect(FakeEventSource.instances[0].url).toBe('/api/v1/events?ticket=a%20b%26c')
    close()
  })

  it('解析 data 为 JSON 后回调；非 JSON（心跳）被忽略', async () => {
    post.mockResolvedValue({ data: { ticket: 'TK-1', expires_in: 60 } })
    const seen: RealtimeEvent[] = []
    const close = connectRealtime((ev) => seen.push(ev))
    await settle()

    const es = FakeEventSource.instances[0]
    es.onmessage?.({ data: '{"type":"new_mail"}' })
    es.onmessage?.({ data: ': ping' })
    expect(seen).toEqual([{ type: 'new_mail' }])
    close()
  })

  it('断线重连会重新取票——旧票在上次握手时已被后端核销', async () => {
    post
      .mockResolvedValueOnce({ data: { ticket: 'TK-1', expires_in: 60 } })
      .mockResolvedValueOnce({ data: { ticket: 'TK-2', expires_in: 60 } })
    const close = connectRealtime(() => {})
    await settle()

    FakeEventSource.instances[0].onopen?.() // 连上过，退避重置为 1s
    FakeEventSource.instances[0].onerror?.()
    await settle(1000)

    expect(post).toHaveBeenCalledTimes(2)
    expect(FakeEventSource.instances).toHaveLength(2)
    expect(FakeEventSource.instances[1].url).toBe('/api/v1/events?ticket=TK-2')
    close()
  })

  it('取票失败不连接，并按指数退避重试', async () => {
    post.mockRejectedValue(new Error('401'))
    const close = connectRealtime(() => {})
    await settle()
    expect(FakeEventSource.instances).toHaveLength(0)
    expect(post).toHaveBeenCalledTimes(1)

    await settle(1000)
    expect(post).toHaveBeenCalledTimes(2)
    // 第二次失败后的退避翻倍：1s 时还不该有第三次
    await settle(1000)
    expect(post).toHaveBeenCalledTimes(2)
    await settle(1000)
    expect(post).toHaveBeenCalledTimes(3)
    close()
  })

  it('取票期间被关闭则不再建立连接', async () => {
    let release: (v: unknown) => void = () => {}
    post.mockReturnValue(new Promise((r) => (release = r)))
    const close = connectRealtime(() => {})
    close()
    release({ data: { ticket: 'TK-1', expires_in: 60 } })
    await settle(60000)
    expect(FakeEventSource.instances).toHaveLength(0)
  })

  it('关闭后不再重连', async () => {
    post.mockResolvedValue({ data: { ticket: 'TK-1', expires_in: 60 } })
    const close = connectRealtime(() => {})
    await settle()
    const es = FakeEventSource.instances[0]
    close()
    expect(es.closed).toBe(true)

    es.onerror?.()
    await settle(60000)
    expect(FakeEventSource.instances).toHaveLength(1)
  })
})

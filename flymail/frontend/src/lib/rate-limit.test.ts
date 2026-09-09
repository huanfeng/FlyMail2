import { describe, it, expect } from 'vitest'
import { AxiosError, AxiosHeaders } from 'axios'
import { MAX_RETRY_AFTER_SEC, parseRetryAfter, retryAfterText } from '@/lib/rate-limit'

/** 造一个带响应体与响应头的 axios 错误 */
function axiosErr(data: unknown, headers: Record<string, string> = {}): AxiosError {
  const err = new AxiosError('rate limited')
  err.response = {
    status: 429,
    statusText: 'Too Many Requests',
    data,
    headers: new AxiosHeaders(headers),
    config: { headers: new AxiosHeaders() },
  }
  return err
}

describe('parseRetryAfter', () => {
  it('优先取响应体的 retry_after', () => {
    expect(parseRetryAfter(axiosErr({ error: 'x', retry_after: 300 }, { 'retry-after': '60' }))).toBe(300)
  })

  it('响应体没有时退回 Retry-After 头', () => {
    expect(parseRetryAfter(axiosErr({ error: 'x' }, { 'retry-after': '90' }))).toBe(90)
  })

  it('两处都取不到时返回 0（调用方据此走通用文案，而不是编造倒计时）', () => {
    expect(parseRetryAfter(axiosErr({ error: 'x' }))).toBe(0)
    expect(parseRetryAfter(axiosErr({ error: 'x', retry_after: 'soon' }))).toBe(0)
    expect(parseRetryAfter(axiosErr({ error: 'x', retry_after: -10 }))).toBe(0)
    expect(parseRetryAfter(new Error('boom'))).toBe(0)
    expect(parseRetryAfter(undefined)).toBe(0)
  })

  it('小数向上取整，异常大的值被截断', () => {
    expect(parseRetryAfter(axiosErr({ retry_after: 12.3 }))).toBe(13)
    expect(parseRetryAfter(axiosErr({ retry_after: 99_999_999 }))).toBe(MAX_RETRY_AFTER_SEC)
  })
})

describe('retryAfterText', () => {
  it('满一分钟按分钟说，并向上取整', () => {
    expect(retryAfterText(60)).toEqual({ unit: 'minutes', value: 1 })
    expect(retryAfterText(61)).toEqual({ unit: 'minutes', value: 2 })
    expect(retryAfterText(900)).toEqual({ unit: 'minutes', value: 15 })
  })

  it('不足一分钟按秒说', () => {
    expect(retryAfterText(59)).toEqual({ unit: 'seconds', value: 59 })
    expect(retryAfterText(1)).toEqual({ unit: 'seconds', value: 1 })
  })

  it('0 与负数归到 1 秒，界面上不出现「请 0 秒后再试」', () => {
    expect(retryAfterText(0)).toEqual({ unit: 'seconds', value: 1 })
    expect(retryAfterText(-5)).toEqual({ unit: 'seconds', value: 1 })
    expect(retryAfterText(NaN)).toEqual({ unit: 'seconds', value: 1 })
  })
})

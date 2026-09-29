import { describe, it, expect } from 'vitest'
import { healthKind } from '@/lib/account-health'

describe('healthKind', () => {
  it('needs_reauth 优先于 breaker（与后端口径一致）', () => {
    expect(healthKind({ status: 'needs_reauth', health: { needs_reauth: true, breaker_open: true, failures: 5 } })).toBe('needs_reauth')
  })
  it('仅熔断返回 breaker', () => {
    expect(healthKind({ status: 'ok', health: { needs_reauth: false, breaker_open: true, failures: 5 } })).toBe('breaker')
  })
  it('健康账户返回 null', () => {
    expect(healthKind({ status: 'ok', health: undefined })).toBe(null)
    expect(healthKind({ status: 'ok', health: { needs_reauth: false, breaker_open: false, failures: 0 } })).toBe(null)
  })
  it('health 缺省但 status 为 needs_reauth 时仍判定（兼容旧缓存数据）', () => {
    expect(healthKind({ status: 'needs_reauth', health: undefined })).toBe('needs_reauth')
  })
})

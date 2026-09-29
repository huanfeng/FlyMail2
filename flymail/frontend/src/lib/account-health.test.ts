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

import { beforeEach } from 'vitest'
import { loadDismissed, saveDismissed, pruneDismissed } from '@/lib/account-health'

describe('关闭记忆', () => {
  beforeEach(() => sessionStorage.clear())

  it('load 空存储返回空集合', () => {
    expect(loadDismissed().size).toBe(0)
  })

  it('save 后 load 往返一致', () => {
    const s = new Set([1, 3])
    saveDismissed(s)
    expect(loadDismissed()).toEqual(new Set([1, 3]))
  })

  it('损坏的存储内容不抛错，回退空集合', () => {
    sessionStorage.setItem('flymail-health-dismissed', '{bad json')
    expect(loadDismissed().size).toBe(0)
  })

  it('恢复后从记忆清除：同会话再故障可重现横幅', () => {
    const dismissed = new Set([1, 2])
    const next = pruneDismissed(dismissed, new Set([2]))
    expect(next.has(1)).toBe(false)
    expect(next.has(2)).toBe(true)
  })
})

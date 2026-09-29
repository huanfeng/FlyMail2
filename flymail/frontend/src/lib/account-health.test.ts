import { describe, it, expect } from 'vitest'
import { healthKind, formatLastSyncRel } from '@/lib/account-health'
import { shouldAccountNotice } from '@/lib/browser-notify'

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

describe('formatLastSyncRel', () => {
  const NOW = Date.UTC(2026, 8, 28, 12, 0, 0)

  it('缺省/非法 iso 返回空串', () => {
    expect(formatLastSyncRel(undefined, NOW, 'zh-CN')).toBe('')
    expect(formatLastSyncRel('not-a-date', NOW, 'zh-CN')).toBe('')
  })
  it('1 分钟内归为「刚刚」', () => {
    const iso = new Date(NOW - 30_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('此刻')
  })
  it('分钟桶', () => {
    const iso = new Date(NOW - 5 * 60_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('5分钟前')
  })
  it('小时桶', () => {
    const iso = new Date(NOW - 3 * 3_600_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('3小时前')
  })
  it('天桶', () => {
    const iso = new Date(NOW - 2 * 86_400_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('前天')
  })
})

describe('shouldAccountNotice 10 秒去重', () => {
  it('同账户同事件 10s 内折叠', () => {
    const t0 = 1_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(7, 'sync_failed', t0 + 9_999)).toBe(false)
    expect(shouldAccountNotice(7, 'sync_failed', t0 + 10_001)).toBe(true)
  })
  it('不同账户互不折叠', () => {
    const t0 = 2_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(8, 'sync_failed', t0)).toBe(true)
  })
  it('同账户不同事件互不折叠', () => {
    const t0 = 3_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(7, 'account_status', t0)).toBe(true)
  })
})

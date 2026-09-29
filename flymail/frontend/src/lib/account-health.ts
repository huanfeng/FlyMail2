import type { Account, AccountRuntimeHealth } from './types'

export type HealthKind = 'needs_reauth' | 'breaker'

/** 账户故障判定：needs_reauth 优先于 breaker（与后端 watcher 口径一致）。
 * health 字段缺省时回退 status 判定（兼容旧缓存数据）。 */
export function healthKind(a: Pick<Account, 'status'> & { health?: AccountRuntimeHealth }): HealthKind | null {
  if (a.health?.needs_reauth || a.status === 'needs_reauth') return 'needs_reauth'
  if (a.health?.breaker_open) return 'breaker'
  return null
}

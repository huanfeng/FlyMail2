import type { Account, AccountRuntimeHealth } from './types'

export type HealthKind = 'needs_reauth' | 'breaker'

/** 账户故障判定：needs_reauth 优先于 breaker（与后端 watcher 口径一致）。
 * health 字段缺省时回退 status 判定（兼容旧缓存数据）。 */
export function healthKind(a: Pick<Account, 'status'> & { health?: AccountRuntimeHealth }): HealthKind | null {
  if (a.health?.needs_reauth || a.status === 'needs_reauth') return 'needs_reauth'
  if (a.health?.breaker_open) return 'breaker'
  return null
}

const DISMISSED_KEY = 'flymail-health-dismissed'

/** 「知道了」的记忆：本会话内已关闭的故障账户 id。恢复时由 pruneDismissed 清除。 */
export function loadDismissed(): Set<number> {
  try {
    return new Set(JSON.parse(sessionStorage.getItem(DISMISSED_KEY) ?? '[]') as number[])
  } catch {
    return new Set()
  }
}

export function saveDismissed(s: Set<number>): void {
  try {
    sessionStorage.setItem(DISMISSED_KEY, JSON.stringify([...s]))
  } catch {
    // sessionStorage 不可用（隐私模式等）：记忆失效可接受，横幅行为回退为每次出现
  }
}

/** 恢复即清记忆：把 dismissed 里已不在 faultyIds 中的 id 移除。
 * 没有这一步，同会话内「恢复→再故障」不会重现横幅。 */
export function pruneDismissed(dismissed: Set<number>, faultyIds: Set<number>): Set<number> {
  const out = new Set<number>()
  dismissed.forEach((id) => {
    if (faultyIds.has(id)) out.add(id)
  })
  return out
}

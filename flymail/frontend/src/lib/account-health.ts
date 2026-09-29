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

/** 「上次同步」相对时间：纯展示。iso 缺省或非法返回空串。
 * 分桶：1 分钟内「刚刚」，60 分钟内分钟桶，24 小时内小时桶，
 * 30 天内天桶，更久回退本地日期。 */
export function formatLastSyncRel(
  iso: string | undefined,
  now: number,
  locale: string,
): string {
  if (!iso) return ''
  const ms = new Date(iso).getTime()
  if (Number.isNaN(ms)) return ''
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const minutes = Math.floor((now - ms) / 60000)
  if (minutes < 1) return rtf.format(0, 'minute')
  if (minutes < 60) return rtf.format(-minutes, 'minute')
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return rtf.format(-hours, 'hour')
  const days = Math.floor(hours / 24)
  if (days < 30) return rtf.format(-days, 'day')
  return new Date(iso).toLocaleDateString(locale)
}

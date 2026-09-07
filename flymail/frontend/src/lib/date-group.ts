// 按日期分组工具函数：将条目按本地时间归入日期分组
//
// 分组语义与显示文本分离：内部一律用语义 key 归组，显示文本最后由 GroupLabeler 渲染。
// 这样既能接 i18n，也避免了「两个不同语义被翻译成同一字符串后误合并」——
// 若直接拿译文当 Map 的 key，某种语言里 today 与 thisWeek 恰好同形就会串组。

/** 固定分组的语义标识（历史月份分组不在其列，见 monthKey） */
export type DateGroupKind = 'today' | 'yesterday' | 'week' | 'month' | 'earlier'

export interface DateGroup<T> {
  /** 分组语义标识：固定分组为 DateGroupKind，历史月份为 `m:YYYY-M` */
  key: string
  /** 显示文本，由 GroupLabeler 渲染 */
  label: string
  items: T[]
}

/** 分组标题的渲染器：把分组语义翻译成显示文本 */
export interface GroupLabeler {
  /** 固定分组标题 */
  fixed(kind: DateGroupKind): string
  /** 历史月份标题；month 为 1-12（非 Date 的 0-11） */
  month(year: number, month: number): string
}

/** 默认（中文）标签器。作为 groupByDate 的缺省值，非 i18n 场景与单测直接可用。 */
export const zhLabeler: GroupLabeler = {
  fixed(kind) {
    switch (kind) {
      case 'today': return '今天'
      case 'yesterday': return '昨天'
      case 'week': return '本周'
      case 'month': return '本月'
      case 'earlier': return '更早'
    }
  },
  month(year, month) {
    return `${year}年${month}月`
  },
}

/** 历史月份分组的 key */
function monthKey(year: number, month: number): string {
  return `m:${year}-${month}`
}

/**
 * 将条目按日期归入有序分组：今天 / 昨天 / 本周 / 本月 / YYYY年M月（更早）
 *
 * @param items   条目数组（调用方保证已按时间降序排列）
 * @param getDate 从条目中提取 ISO 日期字符串
 * @param now     当前时间（默认 new Date()），便于测试传入固定值
 * @param labeler 分组标题渲染器（默认中文），接 i18n 时传入对应实现
 */
export function groupByDate<T>(
  items: T[],
  getDate: (t: T) => string,
  now?: Date,
  labeler: GroupLabeler = zhLabeler,
): DateGroup<T>[] {
  const base = now ?? new Date()

  // 计算各分组的起始零点（本地时间）
  const todayStart = localDayStart(base)
  const yesterdayStart = addDays(todayStart, -1)
  // 本周：周一为起点（ISO 惯例）
  const weekStart = localWeekStart(todayStart)
  // 本月：当月第一天零点
  const monthStart = new Date(todayStart.getFullYear(), todayStart.getMonth(), 1)

  // 有序分组 key 列表（更早的月份分组按需追加）
  const fixedKinds: DateGroupKind[] = ['today', 'yesterday', 'week', 'month']
  // 使用 Map 保证插入顺序；key 为语义标识，value 附带渲染标题所需的信息
  const groupMap = new Map<string, { label: string; items: T[] }>()

  function push(key: string, label: string, item: T) {
    const g = groupMap.get(key)
    if (g) g.items.push(item)
    else groupMap.set(key, { label, items: [item] })
  }

  for (const item of items) {
    const d = new Date(getDate(item))

    // 非法日期归入「更早」（与历史月份分组区分开，不与任何真实月份混同）
    if (Number.isNaN(d.getTime())) {
      push('earlier', labeler.fixed('earlier'), item)
      continue
    }

    const kind = resolveKind(d, todayStart, yesterdayStart, weekStart, monthStart)
    if (kind) {
      push(kind, labeler.fixed(kind), item)
    } else {
      const y = d.getFullYear()
      const m = d.getMonth() + 1
      push(monthKey(y, m), labeler.month(y, m), item)
    }
  }

  // 将 Map 转换为有序数组：先固定分组，再历史月份（按出现顺序）
  const result: DateGroup<T>[] = []

  // 固定顺序：今天 → 昨天 → 本周 → 本月
  for (const kind of fixedKinds) {
    const g = groupMap.get(kind)
    if (g) result.push({ key: kind, label: g.label, items: g.items })
  }

  // 历史月份分组 + 非法日期的「更早」（Map 中除固定 kind 外按插入顺序）
  const fixedSet = new Set<string>(fixedKinds)
  for (const [key, g] of groupMap.entries()) {
    if (!fixedSet.has(key)) {
      result.push({ key, label: g.label, items: g.items })
    }
  }

  return result
}

// ─────────────────────────────────────────────────────────────────────────────
// 内部辅助函数
// ─────────────────────────────────────────────────────────────────────────────

/** 本地时间当天零点 */
function localDayStart(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

/** 往前/后 N 天 */
function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}

/** 本周周一零点（ISO 周，周一为第一天） */
function localWeekStart(todayStart: Date): Date {
  const day = todayStart.getDay() // 0=周日
  const diff = day === 0 ? -6 : 1 - day // 调整到周一
  return addDays(todayStart, diff)
}

/**
 * 将日期 d 映射到固定分组；不属于任何固定分组（即「更早」）时返回 null，
 * 由调用方按年月归组。
 */
function resolveKind(
  d: Date,
  todayStart: Date,
  yesterdayStart: Date,
  weekStart: Date,
  monthStart: Date,
): DateGroupKind | null {
  const dTime = localDayStart(d).getTime()

  if (dTime >= todayStart.getTime()) return 'today'
  if (dTime >= yesterdayStart.getTime()) return 'yesterday'
  if (dTime >= weekStart.getTime()) return 'week'
  if (dTime >= monthStart.getTime()) return 'month'
  return null
}

// 按日期分组工具函数：将条目按本地时间归入中文日期分组

export interface DateGroup<T> {
  label: string
  items: T[]
}

/**
 * 将条目按日期归入有序分组：今天 / 昨天 / 本周 / 本月 / YYYY年M月（更早）
 *
 * @param items   条目数组（调用方保证已按时间降序排列）
 * @param getDate 从条目中提取 ISO 日期字符串
 * @param now     当前时间（默认 new Date()），便于测试传入固定值
 */
export function groupByDate<T>(
  items: T[],
  getDate: (t: T) => string,
  now?: Date,
): DateGroup<T>[] {
  const base = now ?? new Date()

  // 计算各分组的起始零点（本地时间）
  const todayStart = localDayStart(base)
  const yesterdayStart = addDays(todayStart, -1)
  // 本周：周一为起点（ISO 惯例）
  const weekStart = localWeekStart(todayStart)
  // 本月：当月第一天零点
  const monthStart = new Date(todayStart.getFullYear(), todayStart.getMonth(), 1)

  // 有序分组 label 列表（更早的月份分组按需追加）
  const fixedLabels = ['今天', '昨天', '本周', '本月'] as const
  // 使用 Map 保证插入顺序
  const groupMap = new Map<string, T[]>()

  for (const item of items) {
    const raw = getDate(item)
    const d = new Date(raw)

    // 非法日期归入"更早"中最旧的月份分组（用固定 label 占位）
    if (Number.isNaN(d.getTime())) {
      const fallback = '更早'
      if (!groupMap.has(fallback)) groupMap.set(fallback, [])
      groupMap.get(fallback)!.push(item)
      continue
    }

    const label = resolveLabel(d, todayStart, yesterdayStart, weekStart, monthStart)
    if (!groupMap.has(label)) groupMap.set(label, [])
    groupMap.get(label)!.push(item)
  }

  // 将 Map 转换为有序数组：先固定分组，再历史月份（按出现顺序）
  const result: DateGroup<T>[] = []

  // 固定顺序：今天 → 昨天 → 本周 → 本月
  for (const label of fixedLabels) {
    if (groupMap.has(label)) {
      result.push({ label, items: groupMap.get(label)! })
    }
  }

  // 历史月份分组（Map 中除固定 label 外按插入顺序）
  const fixedSet = new Set<string>(fixedLabels)
  for (const [label, groupItems] of groupMap.entries()) {
    if (!fixedSet.has(label)) {
      result.push({ label, items: groupItems })
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

/** 将日期 d 映射到分组 label */
function resolveLabel(
  d: Date,
  todayStart: Date,
  yesterdayStart: Date,
  weekStart: Date,
  monthStart: Date,
): string {
  const dStart = localDayStart(d)
  const dTime = dStart.getTime()

  if (dTime >= todayStart.getTime()) return '今天'
  if (dTime >= yesterdayStart.getTime()) return '昨天'
  if (dTime >= weekStart.getTime()) return '本周'
  if (dTime >= monthStart.getTime()) return '本月'

  // 更早：按"YYYY年M月"分组
  return `${d.getFullYear()}年${d.getMonth() + 1}月`
}

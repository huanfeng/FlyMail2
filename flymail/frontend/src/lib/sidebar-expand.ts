/**
 * 侧栏各账户的展开状态。
 *
 * ── 为什么要自己存，而不是跟着 URL 走 ───────────────────────────────────────
 *
 * 展开状态天生是**多选**的（可以同时展开好几个账户），而 URL 里的 account 是单值，
 * 表达不了。更要紧的是职责不同：URL 应当准确描述「现在在看什么」，而展开哪几个
 * 账户是这台设备上的个人习惯，换台设备、把链接发给别人都不该带过去。
 *
 * ── 原先的写法为什么是坏的 ──────────────────────────────────────────────────
 *
 * 原先是 `useState(() => { accounts.forEach((a, i) => init[a.id] = i < 2) })`。
 * useState 的初始化函数只在**首次渲染**执行一次，而那一刻账户列表还在请求中、
 * 是个空数组——所以「默认展开前两个」从来没有生效过，侧栏永远全部折叠。
 * 而且刷新一次就忘光。
 */

const KEY = 'flymail.sidebar.expanded'

export type ExpandMap = Record<number, boolean>

/** 从 localStorage 读展开状态；读不到或格式不对时返回空表。 */
export function loadExpanded(): ExpandMap {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const out: ExpandMap = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      const id = Number(k)
      // 键必须是账户 id；值只认布尔，别的一律丢掉
      if (Number.isInteger(id) && id > 0 && typeof v === 'boolean') out[id] = v
    }
    return out
  } catch {
    // 隐私模式、存储被禁用、写坏的 JSON——一律当作「没存过」
    return {}
  }
}

/** 写回 localStorage；失败不抛（存储不可用时展开状态只是不记忆，不该影响使用）。 */
export function saveExpanded(map: ExpandMap): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(map))
  } catch {
    /* 忽略 */
  }
}

/**
 * 决定某个账户初始展不展开。
 *
 * 存过就按存的来（哪怕是 false——用户显式折叠过的不能又给他展开）；
 * 没存过的按「前 N 个默认展开」，这样第一次用的人不必逐个点开。
 */
export function initialExpanded(accountIds: number[], stored: ExpandMap, defaultCount = 2): ExpandMap {
  const out: ExpandMap = {}
  accountIds.forEach((id, i) => {
    out[id] = Object.prototype.hasOwnProperty.call(stored, id) ? stored[id] : i < defaultCount
  })
  return out
}

/** 是否至少有一个账户是展开的（决定要不要显示「全部折叠」）。 */
export function anyExpanded(map: ExpandMap): boolean {
  return Object.values(map).some(Boolean)
}

/** 全部折叠。 */
export function collapseAll(map: ExpandMap): ExpandMap {
  const out: ExpandMap = {}
  for (const k of Object.keys(map)) out[Number(k)] = false
  return out
}

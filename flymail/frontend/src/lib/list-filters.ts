// 列表筛选条件：三个独立开关，彼此 AND 叠加。
//
// 与旧版的互斥 chip（全部/未读/星标 三选一）不同，这里每个维度是独立开关，
// 可以同时激活——「未读 + 有附件」是一次真实的检索意图，三选一表达不了。
//
// 与后端 message.Filter 的映射不是恒等：前端只提供「只看 X」的正向开关，
// unread 开关对应后端的 seen=false。不做「只看已读」——没有对应的使用场景，
// 多一个开关只会让工具栏变宽。

/** 列表筛选条件。全 false 即不筛选。 */
export interface ListFilter {
  /** 只看未读 → 后端 seen=false */
  unread: boolean
  /** 只看星标 → 后端 flagged=true */
  flagged: boolean
  /** 只看有附件 → 后端 has_attachment=true */
  attachment: boolean
}

/** 可切换的筛选维度名 */
export type FilterKey = keyof ListFilter

/** 不筛选。作为常量复用，避免每次渲染造新对象打断 query key 相等性判断。 */
export const EMPTY_FILTER: ListFilter = { unread: false, flagged: false, attachment: false }

/** 是否有任一维度生效 */
export function isFilterActive(f: ListFilter): boolean {
  return f.unread || f.flagged || f.attachment
}

/** 切换单个维度，返回新对象（保持不可变，便于用作 query key 依赖） */
export function toggleFilter(f: ListFilter, key: FilterKey): ListFilter {
  return { ...f, [key]: !f[key] }
}

/**
 * 把筛选条件写进查询串。三个列表接口（folder / aggregate / search）参数名一致，
 * 因此这里只有一份映射。未激活的维度不写参数——后端把缺省视为「该维度不筛选」，
 * 显式传 seen=true 会变成「只看已读」，语义完全不同。
 */
export function applyFilterParams(params: URLSearchParams, f: ListFilter): void {
  if (f.unread) params.set('seen', 'false')
  if (f.flagged) params.set('flagged', 'true')
  if (f.attachment) params.set('has_attachment', 'true')
}

/**
 * 稳定的短标识，用作 react-query 的 query key 片段与 MailList 的 sourceKey 片段。
 *
 * 必须进 query key：否则切换筛选后 react-query 认为是同一个查询，
 * 直接返回上一份缓存，界面纹丝不动。
 */
export function filterKey(f: ListFilter): string {
  let s = ''
  if (f.unread) s += 'u'
  if (f.flagged) s += 'f'
  if (f.attachment) s += 'a'
  return s || 'none'
}

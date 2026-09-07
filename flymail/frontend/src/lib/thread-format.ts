// 会话行的展示格式化与选择集合换算。
//
// 抽出来是因为这几段都是纯数据变换，放在组件里就只能靠肉眼验证；
// 参与者去重尤其容易在「同一个人不同显示名」上翻车。

import type { Address, ThreadListItem } from '@/lib/types'

/** 会话行上参与者的展示形态 */
export interface ParticipantsDisplay {
  /** 去重后按原顺序取前 max 个的显示名 */
  names: string[]
  /** 未展示的人数；0 表示全部都展示了，不显示「等 N 人」 */
  extra: number
}

/**
 * 参与者去重 + 截断。
 *
 * 按邮箱（忽略大小写与首尾空白）去重而不是按显示名：同一个人在不同邮件里
 * 可能一次带名字一次只有地址，按名字去重会让同一个人出现两遍。
 * 没有邮箱的（少数畸形头）退回按名字去重，至少不会把两个人合成一个。
 */
export function formatParticipants(list: Address[], max = 3): ParticipantsDisplay {
  const seen = new Set<string>()
  const names: string[] = []
  let total = 0
  for (const p of list) {
    const email = (p.email ?? '').trim().toLowerCase()
    const name = (p.name ?? '').trim()
    const key = email || `name:${name.toLowerCase()}`
    if (key === 'name:' || seen.has(key)) continue
    seen.add(key)
    total++
    if (names.length < max) names.push(name || p.email.trim())
  }
  return { names, extra: Math.max(0, total - names.length) }
}

/**
 * 会话行头像该显示谁。
 *
 * 取第一个不是本人的参与者：自己发起的讨论里 participants[0] 永远是自己，
 * 一列会话行挂满同一个首字母，等于没有信息。全是自己（发给自己、草稿）
 * 时退回第一个，总比没有头像好。
 *
 * @param selfAddrs 本人邮箱集合，需已 trim + 转小写
 */
export function pickAvatarParticipant(
  participants: Address[],
  selfAddrs: Set<string>,
): Address | null {
  if (participants.length === 0) return null
  const other = participants.find((p) => !selfAddrs.has((p.email ?? '').trim().toLowerCase()))
  return other ?? participants[0]
}

/** 取出当前列表里被选中的会话（顺序与列表一致） */
export function selectedThreads(items: ThreadListItem[], selected: Set<string>): ThreadListItem[] {
  return items.filter((it) => selected.has(it.thread_id))
}

/**
 * 一组条目的共同账户；跨账户或为空时返回 null。
 *
 * 批量移动的目标文件夹必须属于同一个账户——后端对跨账户移动直接报错，
 * 与其让用户点完才失败，不如在跨账户时就把移动入口禁掉。
 */
export function commonAccountId(items: { account_id: number }[]): number | null {
  if (items.length === 0) return null
  const first = items[0].account_id
  return items.every((it) => it.account_id === first) ? first : null
}

/**
 * 会话手风琴的默认展开集合：范围内最新一封 + 所有未读。
 *
 * latestId 来自列表行的 `latest_id`，可能并不是成员里时间最晚的那一封——
 * 「范围内最新」看的是当前文件夹/搜索结果，而成员列表是跨文件夹的全量。
 * 取不到时（深链进来、列表里没这条）退回成员里的最后一封。
 */
export function defaultExpanded(
  messages: { id: number; seen: boolean }[],
  latestId: number | null,
): Set<number> {
  const out = new Set<number>()
  for (const m of messages) {
    if (!m.seen) out.add(m.id)
  }
  if (latestId != null && messages.some((m) => m.id === latestId)) out.add(latestId)
  else if (out.size === 0 && messages.length > 0) out.add(messages[messages.length - 1].id)
  return out
}

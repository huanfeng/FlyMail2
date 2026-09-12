/**
 * 账户识别色。
 *
 * 聚合收件箱与搜索结果把多个账户的邮件混在一列里，而所有头像底色都是同一个
 * `var(--accent)`——看不出哪封是工作邮箱、哪封是私人邮箱，这是聚合视图最缺的
 * 一层信息。给每个账户配一个稳定的颜色，在头像角上点一个小圆点即可。
 *
 * 颜色按账户在列表中的次序取，而不是按 id 取模：id 是数据库自增的，
 * 删掉一个账户再加一个，剩下账户的颜色会集体跳变。
 */

/** 调色板。首位用主题强调色，其余是与九套色调都能共存的中性高饱和色。 */
const PALETTE = ['var(--accent)', '#4ade80', '#f59e0b', '#a78bfa', '#f87171', '#38bdf8', '#fb923c']

/** 按次序取色 */
export function accountColor(index: number): string {
  return PALETTE[((index % PALETTE.length) + PALETTE.length) % PALETTE.length]
}

/**
 * 由账户列表建一张 id → 颜色的查询表。
 *
 * @param accounts 账户列表，顺序即配色顺序
 */
export function accountColorMap(accounts: { id: number }[]): Map<number, string> {
  return new Map(accounts.map((a, i) => [a.id, accountColor(i)]))
}

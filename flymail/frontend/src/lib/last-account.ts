/**
 * 最后使用过的账户。
 *
 * ── 为什么不从 URL 取 ────────────────────────────────────────────────────────
 *
 * URL 应当准确描述「现在在看什么」。聚合视图（所有收件箱/未读/星标）是**跨账户**的，
 * 那里根本没有「当前账户」这回事，URL 里再挂一个 account 就是在说一件不成立的事，
 * 还会让人以为列表被那个账户过滤了。
 *
 * 但有些地方确实需要一个账户：点「写邮件」要有默认发件人、打开草稿箱要知道看谁的。
 * 那是**上下文偏好**，不是当前状态——它属于这台设备，换台设备、把链接发给别人都
 * 不该带过去。所以存在本地，与 URL 分开。
 */

const KEY = 'flymail.lastAccount'

/** 记住当前账户；传 null 不做任何事（聚合视图下不该把记忆冲掉）。 */
export function rememberAccount(id: number | null): void {
  if (id == null || !Number.isInteger(id) || id <= 0) return
  try {
    localStorage.setItem(KEY, String(id))
  } catch {
    /* 存储不可用时只是不记忆，不影响使用 */
  }
}

/** 读回上次用过的账户；没有或不可用时返回 null。 */
export function loadLastAccount(): number | null {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return null
    const id = Number(raw)
    return Number.isInteger(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

/**
 * 决定「写邮件 / 草稿箱」该用哪个账户。
 *
 * 优先当前 URL 上的账户（正在看某个账户的文件夹时，用它最符合预期）；
 * 聚合视图下没有当前账户，退到记忆；记忆也没有（或那个账户已被删）就用第一个。
 */
export function resolveContextAccount(
  current: number | null,
  available: number[],
  remembered: number | null = loadLastAccount(),
): number | null {
  if (current != null && available.includes(current)) return current
  if (remembered != null && available.includes(remembered)) return remembered
  return available[0] ?? null
}

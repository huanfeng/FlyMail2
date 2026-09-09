// 发件人下拉的扁平化：一个账户可以配多个发信地址，撰写器里它们是并列的选项。
//
// 为什么不做成"先选账户、再选别名"两级：写信的人心里想的是"我要用哪个地址发"，
// 不是"我要用哪个 IMAP 连接发"。两级下拉把实现细节抬到了用户面前，
// 而扁平列表里 `sales@x.com` 和 `admin@x.com` 就是两个平等的选项——这才是心智模型。
//
// 纯函数：不碰 React、不碰网络，别名列表怎么来的由调用方负责。

import type { Account, Alias } from '@/lib/types'

export interface FromOption {
  /** `<accountId>|<email>`，用于 select 的 value；email 里不会出现 `|` */
  key: string
  accountId: number
  /**
   * 发送时写进 payload 的 `from_alias`。
   * 空串 = 用账户主地址，此时 payload 不带该字段（后端据此走历史路径）。
   */
  alias: string
  email: string
  displayName: string
  /** 下拉里显示的整行文案 */
  label: string
}

/** 账户主地址那一项 */
function accountOption(a: Account): FromOption {
  return {
    key: `${a.id}|${a.email}`,
    accountId: a.id,
    alias: '',
    email: a.email,
    displayName: a.name ?? '',
    label: a.name ? `${a.name} <${a.email}>` : a.email,
  }
}

function aliasOption(a: Account, al: Alias): FromOption {
  return {
    key: `${a.id}|${al.email}`,
    accountId: a.id,
    alias: al.email,
    email: al.email,
    displayName: al.display_name ?? '',
    label: al.display_name ? `${al.display_name} <${al.email}>` : al.email,
  }
}

/**
 * 账户 × 别名 → 扁平选项列表。
 *
 * 账户主地址永远排在该账户的第一位：别名可以被删掉，主地址不会，
 * 把它固定在首位，用户切账户时的落点才是稳定的。
 * 与主地址同名的别名会被跳过（后端理论上不该允许，但重复选项对用户是纯噪声）。
 */
export function buildFromOptions(
  accounts: Account[],
  aliasesByAccount: Record<number, Alias[] | undefined>,
): FromOption[] {
  const out: FromOption[] = []
  for (const a of accounts) {
    out.push(accountOption(a))
    const list = aliasesByAccount[a.id] ?? []
    for (const al of list) {
      if (!al.email) continue
      if (al.email.toLowerCase() === (a.email ?? '').toLowerCase()) continue
      out.push(aliasOption(a, al))
    }
  }
  return out
}

/** 找到 `(accountId, from_alias)` 对应的那一项；找不到返回 null（老草稿里的别名可能已被删） */
export function findFromOption(
  options: FromOption[],
  accountId: number | null,
  alias: string | undefined | null,
): FromOption | null {
  if (accountId == null) return null
  const want = (alias ?? '').toLowerCase()
  const hit = options.find((o) => o.accountId === accountId && o.alias.toLowerCase() === want)
  return hit ?? null
}

/**
 * 某账户默认选中的发件项。
 *
 * 有 `is_default` 别名时选它，否则用主地址——这就是"设为默认"这个开关的全部作用。
 */
export function defaultFromOption(
  options: FromOption[],
  accountId: number | null,
  aliasesByAccount: Record<number, Alias[] | undefined>,
): FromOption | null {
  if (accountId == null) return null
  const mine = options.filter((o) => o.accountId === accountId)
  if (mine.length === 0) return null
  const def = (aliasesByAccount[accountId] ?? []).find((a) => a.is_default)
  if (def) {
    const hit = mine.find((o) => o.alias.toLowerCase() === def.email.toLowerCase())
    if (hit) return hit
  }
  return mine[0]
}

/**
 * 打开草稿 / 回复时决定选中哪一项。
 *
 * 优先级：草稿里存的别名 → 该账户的默认项 → 列表首项。
 * 老草稿 `from_alias` 为空，会落到"默认项"这一档；但如果账户配了默认别名，
 * 空串同样能精确命中主地址那一项（alias === ''），所以老草稿的行为不变。
 */
export function pickFromOption(
  options: FromOption[],
  accountId: number | null,
  alias: string | undefined | null,
  aliasesByAccount: Record<number, Alias[] | undefined>,
): FromOption | null {
  if (alias) {
    const exact = findFromOption(options, accountId, alias)
    if (exact) return exact
  } else if (alias === '') {
    const main = findFromOption(options, accountId, '')
    if (main) return main
  }
  return defaultFromOption(options, accountId, aliasesByAccount) ?? options[0] ?? null
}

// 设置页的页面 ID，以及「从任意位置打开设置并定位到某一页」的请求通道。
//
// ── 为什么是一个订阅通道而不是层层传回调 ─────────────────────────────────
//
// 想打开设置的地方散在组件树深处（阅读器工具栏里「翻译」按钮在 AI 未配置时
// 直接带用户去配置），而设置弹框由 Shell 持有。把一个 onOpenSettings 从 Shell
// 一路传到 Reader / ThreadReader / 工具栏，要改五六层 props，只为一次点击。
// 这里只有一个动作、一个接收方，用最小的订阅通道即可。

/** 设置页面 ID。改名时记得在 LEGACY_PAGE_IDS 里留一条旧名映射。 */
export type SettingsPageId =
  | 'profile'
  | 'appearance'
  | 'shortcuts'
  | 'accounts'
  | 'reading'
  | 'compose'
  | 'filters'
  | 'sync'
  | 'notify'
  | 'ai'
  | 'oauth'
  | 'server'
  | 'monitoring'
  | 'about'

export const DEFAULT_SETTINGS_PAGE: SettingsPageId = 'appearance'

const PAGE_IDS: ReadonlySet<string> = new Set<SettingsPageId>([
  'profile', 'appearance', 'shortcuts', 'accounts', 'reading', 'compose', 'filters',
  'sync', 'notify', 'ai', 'oauth', 'server', 'monitoring', 'about',
])

/**
 * 重组前的页面 ID → 现在所在的页。
 *
 * 留着它是因为 ID 可能被写在别处（书签里的深链、旧版本存下的偏好、文档里的说明），
 * 旧 ID 落到一个不存在的页上，用户看到的是一个空白的设置面板。
 */
const LEGACY_PAGE_IDS: Readonly<Record<string, SettingsPageId>> = {
  general: 'appearance',
  security: 'profile',
  privacy: 'reading',
  mail: 'sync',
  signature: 'compose',
  aliases: 'compose',
  rules: 'filters',
  blocklist: 'filters',
}

/** 把任意输入归一成一个存在的页面 ID；认不出来就回到默认页。 */
export function resolveSettingsPage(id: string | null | undefined): SettingsPageId {
  if (id == null) return DEFAULT_SETTINGS_PAGE
  if (PAGE_IDS.has(id)) return id as SettingsPageId
  return LEGACY_PAGE_IDS[id] ?? DEFAULT_SETTINGS_PAGE
}

type Listener = (page: SettingsPageId) => void
const listeners = new Set<Listener>()

/** 请求打开设置并定位到某一页（由 Shell 接收）。 */
export function requestOpenSettings(page: SettingsPageId): void {
  for (const fn of Array.from(listeners)) fn(page)
}

/** 订阅「打开设置」请求；返回取消订阅函数。 */
export function onOpenSettingsRequest(fn: Listener): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

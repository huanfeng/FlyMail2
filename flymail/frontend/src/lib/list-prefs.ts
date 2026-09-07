// 列表样式偏好：存取 localStorage 中的显示样式设置
const STORAGE_KEY = 'flymail_list_style'

export type ListStyle = 'compact' | 'card'

/** 读取列表样式偏好，默认返回 'compact' */
export function getListStyle(): ListStyle {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === 'compact' || raw === 'card') return raw
  return 'compact'
}

/** 持久化列表样式偏好 */
export function setListStyle(style: ListStyle): void {
  localStorage.setItem(STORAGE_KEY, style)
}

// ── 行内选择框是否常显 ──────────────────────────────────────────────────────
// 默认关闭：选择框只在进入选择模式后出现，避免每行都挂一个复选框显得嘈杂。
// 习惯频繁批量操作的用户可在「外观」里打开，省掉每次先点选择开关的一步。
const ALWAYS_SELECT_KEY = 'flymail_always_show_select'

export function getAlwaysShowSelect(): boolean {
  return localStorage.getItem(ALWAYS_SELECT_KEY) === 'true'
}

export function setAlwaysShowSelect(on: boolean): void {
  localStorage.setItem(ALWAYS_SELECT_KEY, String(on))
}

// ── 会话视图（M10）─────────────────────────────────────────────────────────
// 默认开启：一条 10 封的讨论在列表里占 10 行是旧邮箱的做法，会话折叠才是现在的预期。
// 判据写成「只有显式存过 'false' 才关」，未存过的老用户升级上来直接进会话视图；
// 关掉后一切回到单封列表，没有任何能力依赖会话视图独有。
const CONVERSATION_KEY = 'flymail_conversation_view'

export function getConversationView(): boolean {
  return localStorage.getItem(CONVERSATION_KEY) !== 'false'
}

export function setConversationView(on: boolean): void {
  localStorage.setItem(CONVERSATION_KEY, String(on))
}

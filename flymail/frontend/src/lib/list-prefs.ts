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

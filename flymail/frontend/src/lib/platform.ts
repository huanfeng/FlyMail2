// 平台检测工具：用于按操作系统显示不同的快捷键提示。

/** 是否为 macOS / iOS（决定快捷键修饰符显示为 ⌘ 还是 Ctrl）。 */
export function isMac(): boolean {
  if (typeof navigator === 'undefined') return false
  const p = navigator.platform || navigator.userAgent || ''
  return /mac|iphone|ipad|ipod/i.test(p)
}

/** 搜索快捷键提示文本：mac 显示 ⌘K，其余显示 Ctrl K。 */
export function searchShortcutHint(): string {
  return isMac() ? '⌘K' : 'Ctrl K'
}

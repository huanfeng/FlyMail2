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

/** Wails v2 运行时注入到页面上的 window.runtime（仅桌面端存在）。 */
interface WailsRuntime {
  BrowserOpenURL?: (url: string) => void
}

/** 是否运行在 Wails 桌面壳内（据 runtime 注入判定）。 */
export function isDesktop(): boolean {
  if (typeof window === 'undefined') return false
  return typeof (window as { runtime?: WailsRuntime }).runtime?.BrowserOpenURL === 'function'
}

/**
 * 用系统默认浏览器打开外部链接。
 *
 * 桌面端走 Wails 的 BrowserOpenURL 交给操作系统；若在 WebView2 内直接导航，
 * 页面会被外站整个替换掉（应用"变成"了那个网站，且回不去）。
 * 浏览器环境下退化为新标签页打开。
 *
 * 只放行 http/https/mailto —— 其余协议（file:、javascript: 等）交给系统等于把
 * 邮件里的任意 URI 直接喂给 shell，是明确的攻击面。
 */
export function openExternal(url: string): void {
  const scheme = url.slice(0, url.indexOf(':') + 1).toLowerCase()
  if (scheme !== 'http:' && scheme !== 'https:' && scheme !== 'mailto:') return

  const rt = (window as { runtime?: WailsRuntime }).runtime
  if (typeof rt?.BrowserOpenURL === 'function') {
    rt.BrowserOpenURL(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

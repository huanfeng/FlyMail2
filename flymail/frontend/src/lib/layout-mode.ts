// 邮件布局模式偏好（持久化到 localStorage）。
// - three:     传统三栏并排（侧栏 | 列表 | 阅读）
// - two-slide: 双栏 + 右侧滑入浮动阅读面板（不占列表空间，可关闭）

export type LayoutMode = 'three' | 'two-slide'

const KEY = 'flymail_layout_mode'

export function getLayoutMode(): LayoutMode {
  return localStorage.getItem(KEY) === 'two-slide' ? 'two-slide' : 'three'
}

export function setLayoutMode(mode: LayoutMode): void {
  localStorage.setItem(KEY, mode)
}

/** 亮色/暗色模式 */
export type ThemeMode = 'light' | 'dark'

/** 9 套色调 ID */
export type ToneId =
  | 'slate'
  | 'warm'
  | 'sky'
  | 'rose'
  | 'mint'
  | 'lavender'
  | 'coral'
  | 'butter'
  | 'aqua'

/** 主题偏好（模式 + 色调） */
export interface ThemePref {
  mode: ThemeMode
  tone: ToneId
}

/** 所有色调的元数据（供 UI 渲染色块） */
export const TONES: { id: ToneId; nameKey: string; swatch: string }[] = [
  { id: 'slate',    nameKey: 'settings.general.tone.slate',    swatch: '#5b6470' },
  { id: 'warm',     nameKey: 'settings.general.tone.warm',     swatch: '#b5886b' },
  { id: 'sky',      nameKey: 'settings.general.tone.sky',      swatch: '#4a86c2' },
  { id: 'rose',     nameKey: 'settings.general.tone.rose',     swatch: '#d6628a' },
  { id: 'mint',     nameKey: 'settings.general.tone.mint',     swatch: '#3d9970' },
  { id: 'lavender', nameKey: 'settings.general.tone.lavender', swatch: '#8d6cc4' },
  { id: 'coral',    nameKey: 'settings.general.tone.coral',    swatch: '#d27a63' },
  { id: 'butter',   nameKey: 'settings.general.tone.butter',   swatch: '#d4a72c' },
  { id: 'aqua',     nameKey: 'settings.general.tone.aqua',     swatch: '#2ba9b5' },
]

/** localStorage 键名 */
const KEY_MODE = 'flymail_theme_mode'
const KEY_TONE = 'flymail_theme_tone'

/** 合法色调集合（用于校验） */
const VALID_TONES = new Set<string>(TONES.map((t) => t.id))

/**
 * 读取主题偏好
 * - 从 localStorage 读取 mode/tone，校验合法值
 * - 缺省返回 { mode: 'light', tone: 'slate' }
 */
export function getTheme(): ThemePref {
  const rawMode = localStorage.getItem(KEY_MODE)
  const rawTone = localStorage.getItem(KEY_TONE)

  const mode: ThemeMode =
    rawMode === 'dark' || rawMode === 'light' ? rawMode : 'light'

  const tone: ToneId = VALID_TONES.has(rawTone ?? '') ? (rawTone as ToneId) : 'slate'

  return { mode, tone }
}

/**
 * 应用主题到 documentElement 并持久化
 * - 设置 data-theme 属性（触发 CSS 色调令牌覆盖）
 * - 设置 data-mode 属性（供 MailMaster CSS 选择器 [data-mode="dark"] 使用）
 * - 切换 .dark class（供 shadcn 组件 @custom-variant dark 使用）
 * - 写入 localStorage
 */
export function applyTheme(t: ThemePref): void {
  document.documentElement.dataset.theme = t.tone
  document.documentElement.dataset.mode = t.mode
  document.documentElement.classList.toggle('dark', t.mode === 'dark')
  localStorage.setItem(KEY_MODE, t.mode)
  localStorage.setItem(KEY_TONE, t.tone)
}

/**
 * 初始化主题（入口调用）
 * 读取持久化偏好并立即应用，避免闪烁
 */
export function initTheme(): void {
  applyTheme(getTheme())
}

// ── 向后兼容：旧签名（Theme = 'light' | 'dark'）已废弃，保留类型别名避免外部编译报错 ──
/** @deprecated 请改用 ThemePref / ThemeMode */
export type Theme = ThemeMode

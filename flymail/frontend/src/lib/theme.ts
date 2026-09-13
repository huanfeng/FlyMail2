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

/** 所有色调的元数据（供 UI 渲染主题卡）。
 *  这里刻意**不带颜色值**：色板只在 index.css 的 [data-theme][data-mode] 里定义一份，
 *  界面要显示某套主题长什么样，就把那两个属性挂到元素上让令牌自己生效。 */
export const TONES: { id: ToneId; nameKey: string }[] = [
  { id: 'slate',    nameKey: 'settings.general.tone.slate' },
  { id: 'warm',     nameKey: 'settings.general.tone.warm' },
  { id: 'sky',      nameKey: 'settings.general.tone.sky' },
  { id: 'rose',     nameKey: 'settings.general.tone.rose' },
  { id: 'mint',     nameKey: 'settings.general.tone.mint' },
  { id: 'lavender', nameKey: 'settings.general.tone.lavender' },
  { id: 'coral',    nameKey: 'settings.general.tone.coral' },
  { id: 'butter',   nameKey: 'settings.general.tone.butter' },
  { id: 'aqua',     nameKey: 'settings.general.tone.aqua' },
]

/** localStorage 键名 */
const KEY_MODE = 'flymail_theme_mode'
const KEY_TONE = 'flymail_theme_tone'

/** 合法色调集合（用于校验） */
const VALID_TONES = new Set<string>(TONES.map((t) => t.id))

/** 系统是否偏好暗色。matchMedia 在非浏览器环境（测试、SSR）下可能没有。 */
function prefersDark(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)').matches
    : false
}

/**
 * 读取主题偏好
 * - 从 localStorage 读取 mode/tone，校验合法值
 * - 缺省返回 { mode: 'light', tone: 'slate' }
 */
export function getTheme(): ThemePref {
  const rawMode = localStorage.getItem(KEY_MODE)
  const rawTone = localStorage.getItem(KEY_TONE)

  // 没存过偏好就跟随系统。一律默认亮色会让系统用暗色的人每次打开都被闪一下，
  // 而这个判断在 index.html 的内联引导脚本里有一份提前量——两处必须一致。
  const mode: ThemeMode =
    rawMode === 'dark' || rawMode === 'light' ? rawMode : prefersDark() ? 'dark' : 'light'

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

// ── 当前模式的外部存储（useSyncExternalStore 用）──────────────────────────────
//
// 有些地方要在**模式变化时重新渲染**（正文 iframe 的暗化样式是随 srcDoc 一次性
// 注入的，不重渲染就不会更新）。做成订阅而不是让 applyTheme 挨个通知：
// 盯 DOM 的结果比要求每个写入点都记得广播可靠——applyTheme 之外还有
// initTheme，将来可能还有别的入口。

/** 当前生效的亮/暗模式。直接读 applyTheme 写进去的那个属性，不会与它分叉。 */
export function getThemeMode(): ThemeMode {
  return document.documentElement.dataset.mode === 'dark' ? 'dark' : 'light'
}

/** 订阅模式变化；返回取消订阅函数（useSyncExternalStore 的 subscribe 契约）。 */
export function subscribeThemeMode(onChange: () => void): () => void {
  const ob = new MutationObserver(onChange)
  ob.observe(document.documentElement, { attributes: true, attributeFilter: ['data-mode'] })
  return () => ob.disconnect()
}

// ── 向后兼容：旧签名（Theme = 'light' | 'dark'）已废弃，保留类型别名避免外部编译报错 ──
/** @deprecated 请改用 ThemePref / ThemeMode */
export type Theme = ThemeMode

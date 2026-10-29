// 外观与正文阅读偏好：只存本设备（与栏宽一致——大屏的设置不该污染手机）。
//
// 一个 JSON 存全部键，而不是每项一个 localStorage 键：这些偏好总是成组读写
// （外观预览要同时读五项），一个键也让「恢复默认」与损坏回落只有一处逻辑。
//
// 快照必须缓存：useSyncExternalStore 用 Object.is 比较 getSnapshot 的返回值，
// 每次 JSON.parse 出新对象会让订阅组件无限重渲染（React error #185）。

const STORAGE_KEY = 'flymail_appearance'

export interface Appearance {
  uiScale: 90 | 100 | 110 | 120
  textContrast: 'standard' | 'high'
  motion: 'system' | 'reduce'
  bodyFontSize: 13 | 14 | 15 | 16 | 18
  bodyLineHeight: 1.4 | 1.6 | 1.8
  readingWidth: 'original' | 'comfort' | 'wide'
  bodyZoom: 90 | 100 | 110 | 125 | 150
}

export const APPEARANCE_DEFAULTS: Appearance = {
  uiScale: 100,
  textContrast: 'standard',
  motion: 'system',
  bodyFontSize: 14,
  bodyLineHeight: 1.6,
  readingWidth: 'original',
  bodyZoom: 100,
}

export const APPEARANCE_OPTIONS: { [K in keyof Appearance]: readonly Appearance[K][] } = {
  uiScale: [90, 100, 110, 120],
  textContrast: ['standard', 'high'],
  motion: ['system', 'reduce'],
  bodyFontSize: [13, 14, 15, 16, 18],
  bodyLineHeight: [1.4, 1.6, 1.8],
  readingWidth: ['original', 'comfort', 'wide'],
  bodyZoom: [90, 100, 110, 125, 150],
}

const listeners = new Set<() => void>()
let snapshot: Appearance | null = null

function read(): Appearance {
  let raw: Record<string, unknown> = {}
  try {
    const s = localStorage.getItem(STORAGE_KEY)
    if (s) {
      const v: unknown = JSON.parse(s)
      if (v && typeof v === 'object') raw = v as Record<string, unknown>
    }
  } catch {
    /* 存储不可用或 JSON 损坏：整体回落默认 */
  }
  const out = { ...APPEARANCE_DEFAULTS }
  for (const k of Object.keys(APPEARANCE_DEFAULTS) as (keyof Appearance)[]) {
    const opts = APPEARANCE_OPTIONS[k] as readonly unknown[]
    if (opts.includes(raw[k])) (out as Record<string, unknown>)[k] = raw[k]
  }
  return out
}

export function getAppearance(): Appearance {
  if (snapshot == null) snapshot = read()
  return snapshot
}

function write(next: Appearance) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    /* 不持久化，本次会话内仍生效 */
  }
  snapshot = next
  applyAppearance(next)
  for (const fn of Array.from(listeners)) fn()
}

/** 合并写入。空 patch 用于从存储重新载入（测试与跨标签页同步用） */
export function setAppearance(patch: Partial<Appearance>): void {
  snapshot = null
  write({ ...getAppearance(), ...patch })
}

export function resetAppearance(keys: (keyof Appearance)[]): void {
  const patch: Partial<Appearance> = {}
  for (const k of keys) (patch as Record<string, unknown>)[k] = APPEARANCE_DEFAULTS[k]
  setAppearance(patch)
}

export function subscribeAppearance(fn: () => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

/**
 * 把界面级偏好落到根元素上。默认值不写属性：没改过设置的用户 DOM 与以前完全一致。
 * 正文级偏好（body*、readingWidth）不在这里——它们注入邮件 iframe，见 mail-frame.ts。
 */
export function applyAppearance(a: Appearance, root: HTMLElement = document.documentElement): void {
  root.style.zoom = a.uiScale === 100 ? '' : String(a.uiScale / 100)
  if (a.textContrast === 'high') root.dataset.contrast = 'high'
  else delete root.dataset.contrast
  if (a.motion === 'reduce') root.dataset.motion = 'reduce'
  else delete root.dataset.motion
}

export function initAppearance(): void {
  applyAppearance(getAppearance())
  // 另一个标签页改了设置：重新载入并通知本页订阅者
  window.addEventListener('storage', (e) => {
    if (e.key === STORAGE_KEY) setAppearance({})
  })
}

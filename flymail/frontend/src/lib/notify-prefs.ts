// 浏览器通知偏好。
//
// 存 localStorage 而不是后端：这几项本来就是**按浏览器/设备**成立的选择——
// 通知权限是浏览器授予当前源的，在公司电脑上开了声音不等于手机上也想要。
// 与 privacy-prefs 同一套路（含跨组件同步的订阅机制）。

export interface NotifyPrefs {
  /** 浏览器原生桌面通知。默认关：它要权限，不能替用户做主 */
  desktop: boolean
  /** 新邮件提示音 */
  sound: boolean
  /** 标签页标题与图标上的未读角标。不需要任何权限，默认开 */
  titleBadge: boolean
}

const LS_KEY = 'flymail-notify-prefs-v1'

export const NOTIFY_DEFAULTS: NotifyPrefs = {
  desktop: false,
  sound: false,
  titleBadge: true,
}

type Listener = () => void
const listeners = new Set<Listener>()

let cache: NotifyPrefs | null = null

export function getNotifyPrefs(): NotifyPrefs {
  if (cache) return cache
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (raw) {
      const p = JSON.parse(raw) as Partial<NotifyPrefs>
      cache = {
        desktop: p.desktop ?? NOTIFY_DEFAULTS.desktop,
        sound: p.sound ?? NOTIFY_DEFAULTS.sound,
        titleBadge: p.titleBadge ?? NOTIFY_DEFAULTS.titleBadge,
      }
      return cache
    }
  } catch {
    /* 隐私模式下 localStorage 可能抛错，回落默认值即可 */
  }
  cache = { ...NOTIFY_DEFAULTS }
  return cache
}

export function setNotifyPrefs(patch: Partial<NotifyPrefs>): NotifyPrefs {
  const next = { ...getNotifyPrefs(), ...patch }
  cache = next
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(next))
  } catch {
    /* 存不下也要让本次会话内的选择生效，所以 cache 先写 */
  }
  for (const fn of listeners) fn()
  return next
}

/** 订阅偏好变化（useSyncExternalStore 用）。 */
export function subscribeNotifyPrefs(fn: Listener): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

/** 供测试重置模块级缓存。 */
export function resetNotifyPrefsCache(): void {
  cache = null
}

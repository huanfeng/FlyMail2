// 浏览器原生通知与提示音。
//
// 两件事都只在**标签页不可见**时做：页面就在眼前时，列表已经自己刷新了，
// 再弹一个系统通知只是噪音（Gmail / Slack 也都是这个判据）。

export type NotifyPermission = 'unsupported' | 'default' | 'granted' | 'denied'

/** 当前通知权限。'unsupported' 表示这个浏览器/上下文根本没有 Notification。 */
export function notifyPermission(): NotifyPermission {
  if (typeof window === 'undefined' || !('Notification' in window)) return 'unsupported'
  return Notification.permission as NotifyPermission
}

/**
 * 请求通知权限。
 *
 * 必须由用户的点击触发：页面一加载就弹权限框，多数浏览器会直接拒绝或折叠，
 * 而且一旦被拒就再也问不了（denied 是粘住的）。所以设置页里的开关才是入口。
 */
export async function requestNotifyPermission(): Promise<NotifyPermission> {
  if (notifyPermission() === 'unsupported') return 'unsupported'
  try {
    return (await Notification.requestPermission()) as NotifyPermission
  } catch {
    // 老 Safari 只有回调式签名，拿不到结果时按当前状态处理
    return notifyPermission()
  }
}

/** 标签页此刻是否不可见（被切走、被遮住、窗口最小化）。 */
export function pageHidden(): boolean {
  return typeof document !== 'undefined' && document.visibilityState === 'hidden'
}

export interface MailNotice {
  title: string
  body: string
  /** 单封邮件时非 0：点击通知直达那封信 */
  messageId: number
  accountId: number
  onOpen?: (messageId: number) => void
}

/**
 * 弹一条新邮件通知。
 *
 * 按账户分 tag：同一账户的连续来信会**替换**掉上一条而不是堆成一摞——
 * 一次同步带回十几封时，用户要的是「有新邮件」这一个提示，不是十几个弹窗。
 * 跨账户仍然各弹各的（不同账户是不同的事）。
 */
export function showMailNotice(n: MailNotice): Notification | null {
  if (notifyPermission() !== 'granted') return null
  try {
    const notice = new Notification(n.title, {
      body: n.body,
      tag: `flymail-mail-${n.accountId}`,
      // 不用 renotify：它要求配合 tag 使用，且部分浏览器会因此再响一次系统音，
      // 与我们自己的提示音叠在一起
    })
    notice.onclick = () => {
      // 先把窗口拉到前面，否则用户点了通知却什么也没看见
      window.focus()
      if (n.messageId > 0) n.onOpen?.(n.messageId)
      notice.close()
    }
    return notice
  } catch {
    // 部分浏览器在非安全上下文或权限被撤销时会直接抛
    return null
  }
}

// ── 提示音 ───────────────────────────────────────────────────────────────────
//
// 用 Web Audio 合成而不是带一个音频文件：省掉一个二进制资源与它的加载失败分支，
// 也免了「点开邮件时才发现音频 404」这种只在生产才暴露的问题。

let audioCtx: AudioContext | null = null

function ctx(): AudioContext | null {
  if (typeof window === 'undefined') return null
  const Ctor = window.AudioContext ?? (window as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!Ctor) return null
  if (!audioCtx) audioCtx = new Ctor()
  return audioCtx
}

/**
 * 跨标签页抢一次提示音的资格。
 *
 * 通知本身有 `tag` 管着（同源多标签弹出的会互相替换，桌面上只看到一条），
 * **但声音没有 tag 这回事**：开着两个 FlyMail 窗口然后去干别的，
 * 同一批新邮件会听到两声叠在一起。
 *
 * 这是「减少重复」不是「严格互斥」：localStorage 的读-改-写不是原子的，
 * 两个标签页恰好同时读到旧值时仍会各响一声。SSE 推送到各标签有微小时差，
 * 实际撞上的概率很低，而为一声提示音上真正的锁不值得。
 * 存不进去（隐私模式）就各响各的——总比一声不响好。
 */
const CHIME_LOCK_KEY = 'flymail-chime-at'
const CHIME_WINDOW_MS = 3000

export function claimChime(): boolean {
  try {
    const last = Number(localStorage.getItem(CHIME_LOCK_KEY) ?? 0)
    const now = Date.now()
    if (Number.isFinite(last) && now - last < CHIME_WINDOW_MS) return false
    localStorage.setItem(CHIME_LOCK_KEY, String(now))
    return true
  } catch {
    return true
  }
}

/**
 * 两声短促的提示音（上行小三度）。
 *
 * 音量刻意压得很低（0.06）：这是后台提醒，不是闹钟。
 * 新邮件那条路请走 claimChime() 把关；设置页的「试听」直接调这里，
 * 它是用户主动要求的，不该被跨标签页的去重窗口挡住。
 */
export function playChime(): void {
  const ac = ctx()
  if (!ac) return
  try {
    // 浏览器的自动播放策略会让 AudioContext 停在 suspended，
    // 直到页面有过用户交互。resume 失败就安静地算了，不值得为提示音报错。
    if (ac.state === 'suspended') void ac.resume()
    const now = ac.currentTime
    for (const [i, freq] of [880, 1046.5].entries()) {
      const osc = ac.createOscillator()
      const gain = ac.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      const start = now + i * 0.12
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(0.06, start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.11)
      osc.connect(gain).connect(ac.destination)
      osc.start(start)
      osc.stop(start + 0.12)
    }
  } catch {
    /* 音频不是关键路径，失败就静音 */
  }
}

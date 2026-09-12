import { useEffect } from 'react'
import { useSyncExternalStore } from 'react'
import { getNotifyPrefs, subscribeNotifyPrefs } from '@/lib/notify-prefs'

/**
 * 把未读数写到标签页标题与站点图标上。
 *
 * 这是**不需要任何权限**的那一层提醒，也是用户切到别的标签页后唯一还能看见的
 * 东西——桌面通知要授权、还可能被系统免打扰拦掉，而标签页标题一直在那儿。
 *
 * 图标是现画的：用 canvas 画底图再叠一个角标，省掉「准备两套 ico」以及
 * 「未读数变化时去换哪一张」的维护。画不出来（SSR / canvas 被禁）就只改标题。
 */

const BASE_TITLE = 'FlyMail'

/**
 * 画一枚带未读角标的图标，返回 data URL；画不出来时返回 null。
 *
 * 整段包 try/catch 不是防御性冗余：`ctx.roundRect` 要 Chrome 99 / Safari 16.4 /
 * Firefox 112 才有，更老的浏览器上它是 `undefined`。这个函数在 effect 里同步调用，
 * 抛出来会一路冒到 React，被最外层的 ErrorBoundary 接住——
 * **整个应用会因为一枚 favicon 变成错误页**。
 * 这条路径测试也覆盖不到：jsdom 的 getContext 返回 null，下面这些一行都不会执行。
 */
function drawFavicon(unread: number): string | null {
  if (typeof document === 'undefined') return null
  try {
    return paintFavicon(unread)
  } catch {
    return null
  }
}

function paintFavicon(unread: number): string | null {
  const canvas = document.createElement('canvas')
  canvas.width = 64
  canvas.height = 64
  const ctx = canvas.getContext('2d')
  if (!ctx) return null

  // 底图：圆角方块 + 字母 F。与侧栏的 .brand-mark 是同一个意象
  ctx.fillStyle = '#2f6df6'
  ctx.beginPath()
  ctx.roundRect(2, 2, 60, 60, 14)
  ctx.fill()
  ctx.fillStyle = '#ffffff'
  ctx.font = 'bold 40px system-ui, -apple-system, "Segoe UI", sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText('F', 32, 35)

  if (unread > 0) {
    // 角标压在右上角，带一圈底色描边，免得与深色底图糊在一起
    const label = unread > 99 ? '99+' : String(unread)
    const r = 21
    const cx = 64 - r - 1
    const cy = r + 1
    ctx.beginPath()
    ctx.arc(cx, cy, r, 0, Math.PI * 2)
    ctx.fillStyle = '#e5484d'
    ctx.fill()
    ctx.lineWidth = 4
    ctx.strokeStyle = '#ffffff'
    ctx.stroke()
    ctx.fillStyle = '#ffffff'
    ctx.font = `bold ${label.length > 2 ? 18 : 24}px system-ui, -apple-system, sans-serif`
    ctx.fillText(label, cx, cy + 1)
  }
  return canvas.toDataURL('image/png')
}

function applyFavicon(href: string) {
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.type = 'image/png'
  link.href = href
}

/**
 * @param unread 未读总数（跨账户）
 */
export function useUnreadBadge(unread: number): void {
  const prefs = useSyncExternalStore(subscribeNotifyPrefs, getNotifyPrefs, getNotifyPrefs)
  const enabled = prefs.titleBadge

  useEffect(() => {
    const n = enabled ? Math.max(0, Math.floor(unread)) : 0
    document.title = n > 0 ? `(${n > 99 ? '99+' : n}) ${BASE_TITLE}` : BASE_TITLE
    const href = drawFavicon(n)
    if (href) applyFavicon(href)
  }, [unread, enabled])
}

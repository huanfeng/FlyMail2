import { auth } from '@/lib/auth'
import type { RealtimeEvent } from '@/lib/types'

/**
 * 连接 SSE 实时推送流。
 * - 使用 query-param 方式传递 access_token（浏览器 EventSource 不支持自定义请求头）。
 * - 连接断开时自动尝试刷新 token，再按指数退避重连（最长 30 秒）。
 * - 返回关闭函数，供组件卸载时调用。
 */
export function connectRealtime(onEvent: (ev: RealtimeEvent) => void): () => void {
  let es: EventSource | null = null
  let closed = false
  let backoff = 1000

  /** 尝试用 refresh_token 换取新的 access_token */
  async function refreshToken(): Promise<boolean> {
    const rt = auth.refresh
    if (!rt) return false
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: rt }),
      })
      if (!res.ok) return false
      const data = (await res.json()) as { access_token: string; refresh_token: string }
      auth.set(data.access_token, data.refresh_token)
      return true
    } catch {
      return false
    }
  }

  /** 建立 EventSource 连接 */
  function open() {
    if (closed) return
    const token = auth.access
    if (!token) return

    es = new EventSource(`/api/v1/events?access_token=${encodeURIComponent(token)}`)

    es.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data) as RealtimeEvent)
      } catch {
        // 忽略心跳或非 JSON 数据
      }
    }

    es.onerror = () => {
      es?.close()
      es = null
      if (closed) return
      // 可能是 token 过期，先尝试刷新，再退避重连
      void refreshToken().finally(() => {
        if (closed) return
        setTimeout(open, backoff)
        backoff = Math.min(backoff * 2, 30000)
      })
    }

    es.onopen = () => {
      // 连接成功后重置退避时间
      backoff = 1000
    }
  }

  open()

  return () => {
    closed = true
    es?.close()
    es = null
  }
}

import api from '@/lib/api'
import type { RealtimeEvent } from '@/lib/types'

/**
 * 连接状态。
 *
 * 只有两个值，因为调用方要回答的只有一个问题：**此刻还收得到推送吗**。
 * 「正在退避等待下一次重连」与「正在取票」对用户没有区别，都归 'connecting'。
 */
export type RealtimeState = 'connecting' | 'open'

/** 票据签发端点的响应 */
interface StreamTicket {
  ticket: string
  expires_in: number
}

/**
 * 向后端换取一张一次性 SSE 连接票据。
 *
 * 走 api 实例而不是裸 fetch：401 时它会用 refresh token 静默换新并重试
 * （见 lib/api.ts 的响应拦截器），刷新彻底失败才清登录态跳登录页。
 * 这里因此不需要自己再实现一遍 token 刷新——重连时重新取票，刷新自然一并发生。
 */
async function fetchTicket(): Promise<string | null> {
  try {
    const res = await api.post<StreamTicket>('/events/ticket')
    return res.data.ticket || null
  } catch {
    return null
  }
}

/**
 * 连接 SSE 实时推送流。
 *
 * 鉴权用一次性票据而不是 access token：浏览器原生 EventSource 无法设置请求头，
 * 凭据只能写在 URL 上，而 URL 会落进反向代理与浏览器的访问日志、也留在历史记录里。
 * 放在那里的必须是「只能换一条连接、60 秒作废、握手即核销」的票，
 * 而不是一个能开整个账号的长期凭据（KI-2）。
 *
 * 断开后按指数退避（最长 30 秒）重连，每次重连都**重新取票**——旧票在上次握手时
 * 就已被后端核销，复用它只会换来一次 401。
 *
 * 断开期间**收不到任何推送**——退避最长 30 秒，那段时间里新邮件既不会让列表刷新，
 * 也不会弹通知。所以状态要能外露给界面（`onState`），否则合盖唤醒、后端重启之后
 * UI 表现为「安静地不再收信」，与「确实没有新邮件」在用户眼里完全一样。
 *
 * 返回关闭函数，供组件卸载时调用。
 */
export function connectRealtime(
  onEvent: (ev: RealtimeEvent) => void,
  onState?: (s: RealtimeState) => void,
): () => void {
  let es: EventSource | null = null
  let closed = false
  let backoff = 1000

  /** 断开后安排下一次重连（退避后重新取票再连） */
  function scheduleReconnect() {
    if (closed) return
    onState?.('connecting')
    setTimeout(open, backoff)
    backoff = Math.min(backoff * 2, 30000)
  }

  /** 取票并建立 EventSource 连接 */
  function open() {
    if (closed) return
    void fetchTicket().then((ticket) => {
      // 取票是异步的，期间调用方可能已经卸载组件：连上去就没人关了。
      if (closed) return
      if (!ticket) {
        scheduleReconnect()
        return
      }

      es = new EventSource(`/api/v1/events?ticket=${encodeURIComponent(ticket)}`)

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
        scheduleReconnect()
      }

      es.onopen = () => {
        // 连接成功后重置退避时间
        backoff = 1000
        onState?.('open')
      }
    })
  }

  onState?.('connecting')
  open()

  return () => {
    closed = true
    es?.close()
    es = null
  }
}

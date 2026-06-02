import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { connectRealtime } from '@/lib/sse'

/**
 * 订阅 SSE 实时推送。
 * 收到 new_mail 事件后，使 folders 和 messages 查询缓存失效，
 * TanStack Query 将自动在后台重新请求，从而刷新未读数和邮件列表。
 */
export function useRealtimeSync(): void {
  const qc = useQueryClient()
  useEffect(() => {
    const close = connectRealtime((ev) => {
      if (ev.type === 'new_mail') {
        void qc.invalidateQueries({ queryKey: ['folders'] })
        void qc.invalidateQueries({ queryKey: ['messages'] })
      }
    })
    return close
  }, [qc])
}

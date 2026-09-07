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
        // 会话视图下列表数据来自 ['threads']：新邮件既可能新开一条会话，
        // 也可能只是让某条已有会话的封数/未读数变化，两种都要重取。
        void qc.invalidateQueries({ queryKey: ['threads'] })
        // 新邮件可能正落在用户此刻展开的那条会话里
        void qc.invalidateQueries({ queryKey: ['thread-messages'] })
        void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
        void qc.invalidateQueries({ queryKey: ['account-unread'] })
        // 新邮件会产生站内通知，刷新铃铛角标与通知列表
        void qc.invalidateQueries({ queryKey: ['notifications-unread'] })
        void qc.invalidateQueries({ queryKey: ['notifications'] })
      }
    })
    return close
  }, [qc])
}

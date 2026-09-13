import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/components/ui/Toast'
import { useSyncStatus, useTriggerSync } from '@/lib/queries'
import { writeSyncStatus } from '@/lib/sync-cache'
import { apiErrorMessage } from '@/lib/api'
import { isSyncActive, type SyncStatus } from '@/lib/types'

/**
 * 手动触发一次账户同步，并跟踪它到终态。
 *
 * ── 为什么要把这段抽出来 ────────────────────────────────────────────────────
 *
 * 「点同步 → 轮询状态 → 到终态收手」这套逻辑原先有两份：侧栏（Shell）一份、
 * 设置面板的账户卡一份。两份都踩了同一批坑，而且它们共用同一个查询键
 * `['sync-status', id]`，互相还能看见对方写进缓存的东西。
 *
 * ── 三个必须按顺序做对的地方 ────────────────────────────────────────────────
 *
 * 1. **先等触发成功，再开轮询。** 停用的账户会 500、已有同步在跑会 409。
 *    原先不管成没成功都把轮询打开，而这两种情况下后端根本没有建立状态记录，
 *    phase 永远到不了 done/error——于是每秒一个请求一直发到用户刷新页面为止，
 *    屏幕上还一点提示都没有。
 *
 * 2. **开轮询之前必须覆盖缓存里上一轮的 'done'。** 查询键带账户 id，上一轮同步
 *    结束后缓存里留着 `{phase:'done'}`。不覆盖的话，开轮询的那一刻 react-query
 *    会**同步**返回它，下面那个终态 effect 当场触发、立刻又收手——同一个账户的
 *    第二次及以后的同步，转圈、进度条、计数全都不会出现。
 *
 *    写成 `queued` 是如实的而非乐观猜测：后端 `Service.Trigger` 在返回 202 之前
 *    就已经同步调用了 `status.begin(accountID, PhaseQueued)`。
 *
 * 3. **'none' 也是终态。** 后端的状态是内存态（重启丢失），status 端点在没有记录时
 *    返回 200 `{"phase":"none"}`，账户被删了也照样是这个。既然第 1 条保证了
 *    「开轮询时状态记录一定存在」，那之后再读到 'none' 就只意味着记录没了——
 *    不认它的话，同步中重启后端 / 删掉账户又是一条轮询永不停止的路径。
 */
export interface AccountSync {
  /** 正在跟踪的账户 id；null 表示当前没有由本会话触发的同步 */
  accountId: number | null
  /** 该账户的最新状态快照；没有在跟踪时为 null */
  status: SyncStatus | null
  /** 是否正在同步（排队也算） */
  syncing: boolean
  /** 触发一次同步。失败只弹提示，不会开轮询 */
  start: (id: number) => void
}

export function useAccountSync(): AccountSync {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const { toast } = useToast()
  const [accountId, setAccountId] = useState<number | null>(null)
  const { data } = useSyncStatus(accountId, accountId != null)
  const trigger = useTriggerSync()

  // 不在跟踪时一律给 null：查询键随 accountId 变，停下之后 data 本就会变成
  // 上一个键的残留或 undefined，把它透出去只会让调用方读到过期状态。
  const status = accountId == null ? null : (data ?? null)
  const phase = status?.phase

  useEffect(() => {
    if (accountId == null || phase == null) return
    if (isSyncActive(phase)) return

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setAccountId(null)

    if (phase === 'done') {
      // 抓完了刷新文件夹（未读数/新文件夹）、列表与聚合计数
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['threads'] })
      void qc.invalidateQueries({ queryKey: ['thread-messages'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
      return
    }
    if (phase === 'error') {
      toast(t('sync.error', { error: status?.error ?? '' }))
      return
    }
    // 'none'：状态记录没了（后端重启，或账户已被删除）
    toast(t('sync.lost'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId, phase])

  const start = useCallback(
    (id: number) => {
      trigger.mutate(id, {
        onSuccess: () => {
          // 走同一个单调写入口。播种没有 updated_at，于是它总能覆盖上一轮
          // 遗留的 done——那正是要的：用户刚按下按钮，这一刻服务端确实是 queued。
          writeSyncStatus(qc, id, { account_id: id, phase: 'queued' })
          setAccountId(id)
        },
        onError: (err) => {
          toast(apiErrorMessage(err, t('sync.triggerFailed')))
        },
      })
    },
    [trigger, qc, toast, t],
  )

  return { accountId, status, syncing: isSyncActive(phase), start }
}

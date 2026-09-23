import { useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { CLIENT_ID } from '@/lib/client-id'
import { reconcileSyncStatus } from '@/lib/queries'
import { connectRealtime, type RealtimeState } from '@/lib/sse'
import { writeSyncStatus } from '@/lib/sync-cache'
import type { SyncStatus } from '@/lib/types'
import { getNotifyPrefs } from '@/lib/notify-prefs'
import { claimChime, pageHidden, playChime, showMailNotice } from '@/lib/browser-notify'

/** 新邮件通知里「点开那封信」的回调。由 Shell 传入。 */
export interface RealtimeOptions {
  onOpenMessage?: (messageId: number) => void
  /** 供读屏播报的一行文字（常驻 live region 消费）。 */
  onAnnounce?: (text: string) => void
}

/**
 * 订阅 SSE 实时推送。
 *
 * 两类事件分工不同，不能混：
 *
 * - `new_mail` —— 「有变化，去重新拉」。对基线导入、archive / junk 一律会发，
 *   所以只用来让缓存失效，**绝不**拿它弹通知：用户首次添加账户导入几千封
 *   历史邮件时会被淹没。
 * - `notify` —— 「值得打扰用户的一件事」。后端在 emit 那一侧已经过了三道闸门
 *   （文件夹类型、非基线未读、跨文件夹去重），标题正文也拼好了。
 */

/**
 * 连续多久连不上才告诉用户。
 *
 * 判据不能是「此刻是不是 connecting」——重连很频繁（合盖唤醒、切换网络、后端重启
 * 都会断一下），而绝大多数在一两秒内就成功了，照实显示只会让提示条不停闪。
 */
const OFFLINE_AFTER_MS = 6000

/**
 * 多久之内的 mail_state 合成一次失效。
 *
 * 这条事件不携带信息，只说「手里那份不作数」，所以晚半秒失效与立刻失效对用户
 * 没有任何区别；而不合并的代价是实打实的：`invalidateQueries` 对活动查询立刻重取，
 * **不受标签页是否可见影响**（focusManager 只管 refetchOnWindowFocus 与 refetchInterval）。
 * 用户在一个标签页里按住 j 连读，每封一条事件、每条 6+ 个请求，另一个标签页就会
 * 以每秒十几个请求的速度打后端——而那边屏幕上根本没人在看。
 */
const MAIL_STATE_DEBOUNCE_MS = 400

/**
 * 把「邮件视图」相关的缓存全部标脏。
 *
 * new_mail 与 mail_state 收到后要做的事完全一样：两者都只说明「本地数据变了，
 * 手里这份不作数」，差别仅在变化是谁引起的。
 */
function invalidateMailViews(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: ['folders'] })
  void qc.invalidateQueries({ queryKey: ['messages'] })
  // 会话视图下列表数据来自 ['threads']：变化既可能新开一条会话，
  // 也可能只是让某条已有会话的封数/未读数变化，两种都要重取。
  void qc.invalidateQueries({ queryKey: ['threads'] })
  // 变化可能正落在用户此刻展开的那条会话里
  void qc.invalidateQueries({ queryKey: ['thread-messages'] })
  void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
  void qc.invalidateQueries({ queryKey: ['account-unread'] })
}

/**
 * 订阅 SSE 的结果。
 *
 * 两个字段**不是**同一件事的两种说法，界面上也由两个不同的元件消费：
 *
 * - `offline` 是「连不上已经持续了一会儿」（≥ OFFLINE_AFTER_MS），给那条横幅用。
 *   它带防抖，因为横幅是打扰性的，为一次一秒的重连弹出来只会烦人。
 * - `state` 是**此刻**的原始状态，给标题栏那颗常驻状态灯用。它不防抖：
 *   状态灯本来就常驻在那儿、不打扰任何人，反而应该如实反映短暂的重连，
 *   否则断开的头六秒里界面上没有任何迹象（横幅还没到阈值）——
 *   而那六秒里新邮件既不刷新列表也不弹通知。
 */
export interface RealtimeStatus {
  offline: boolean
  state: RealtimeState
}

/** 订阅 SSE，返回连接状态（见 RealtimeStatus）。 */
export function useRealtimeSync(opts: RealtimeOptions = {}): RealtimeStatus {
  const qc = useQueryClient()
  // 回调每次渲染都是新引用，放进依赖会让 SSE 连接反复重建（每次都要重新取票）。
  // 同步放在 effect 里而不是 render 期赋值：后者正是 react-hooks/refs 拦的东西，
  // 而这里也不需要在 render 期读——SSE 回调只会在 effect 跑完之后才触发。
  const optsRef = useRef(opts)
  useEffect(() => {
    optsRef.current = opts
  })

  const [offline, setOffline] = useState(false)
  // 初值取 'connecting'：connectRealtime 建连前就会先报一次 connecting，
  // 但那要等到 effect 跑完。用 'open' 做初值会让首帧闪一下绿灯。
  const [state, setState] = useState<RealtimeState>('connecting')

  useEffect(() => {
    // 计时器在 effect 作用域内，与连接同生共死：卸载时一并清掉，
    // 不会出现「连接没了而计时器还在把 offline 置真」。
    let offlineTimer: ReturnType<typeof setTimeout> | null = null
    const clearOfflineTimer = () => {
      if (offlineTimer != null) {
        clearTimeout(offlineTimer)
        offlineTimer = null
      }
    }

    // mail_state 的合并计时器，同样与连接同生共死。
    let mailStateTimer: ReturnType<typeof setTimeout> | null = null
    const scheduleMailStateRefresh = () => {
      if (mailStateTimer != null) clearTimeout(mailStateTimer)
      mailStateTimer = setTimeout(() => {
        mailStateTimer = null
        invalidateMailViews(qc)
        // 别的界面改的可能正是此刻打开的这封：星标、标为未读、删除都会。
        // 只在这条分支补，不放进 invalidateMailViews——new_mail 不影响已打开的详情，
        // 给它加上就是每来一封新邮件白发一个请求。
        void qc.invalidateQueries({ queryKey: ['message'] })
      }, MAIL_STATE_DEBOUNCE_MS)
    }

    const close = connectRealtime((ev) => {
      if (ev.type === 'sync_status') {
        // 写进与轮询同一个缓存键。
        //
        // 这样「谁在同步」只有一份真相：手动触发那一路在轮询这个键，侧栏的每个
        // 账户行在**观察**这个键（enabled:false，只读缓存不发请求），后台自动同步
        // 则由这里推进来。三方读同一份，不会出现「转圈的是 A、进度是 B」。
        // 剥掉 type：它是信封而不是状态。留着的话缓存里的 SyncStatus 会多出一个
        // 来路不明的字段，下一个人分不清它是后端给的还是前端塞的。
        const status: SyncStatus = { ...ev }
        delete (status as { type?: string }).type
        writeSyncStatus(qc, ev.account_id, status)
        return
      }

      if (ev.type === 'new_mail') {
        invalidateMailViews(qc)
        return
      }

      if (ev.type === 'mail_state') {
        // 自己那次操作的回声：onSettled 已经失效过一轮，再来一轮纯属白打请求
        // （一次标已读要带出 folders ×账户数 + 两个计数接口）。
        //
        // ⚠ 这一句成立的前提：**后端 mailStateNotify 那一组路由，对应的每个前端
        // mutation 都失效了 invalidateMailCaches 的那 6 个 key**（见 lib/queries.ts）。
        // 往那组路由里加新接口时必须一并满足，否则发起方既不自己刷、又认掉回声，
        // 表现是「只有做操作的那个标签页角标不动、别的都对」——所有排列里最难归因的一种。
        if (ev.origin === CLIENT_ID) return
        scheduleMailStateRefresh()
        return
      }

      if (ev.type === 'notify') {
        // 铃铛角标与通知列表跟着这条走（它才是「产生了一条通知」的准确时刻）
        void qc.invalidateQueries({ queryKey: ['notifications-unread'] })
        void qc.invalidateQueries({ queryKey: ['notifications'] })

        if (ev.event !== 'mail_new') return

        // 读屏播报：视觉用户看得见未读徽标跳变，读屏用户此前完全无感知
        optsRef.current.onAnnounce?.(`${ev.title} ${ev.body}`.trim())

        // 桌面通知与提示音只在标签页不可见时给：页面就在眼前时列表已经自己
        // 刷新了，再弹一个系统通知只是噪音。
        if (!pageHidden()) return
        const prefs = getNotifyPrefs()
        if (prefs.desktop) {
          showMailNotice({
            title: ev.title,
            body: ev.body,
            messageId: ev.message_id,
            accountId: ev.account_id,
            onOpen: (id) => optsRef.current.onOpenMessage?.(id),
          })
        }
        // 抢一次：开着多个 FlyMail 标签页时，同一批新邮件不该按窗口数叠加着响
        if (prefs.sound && claimChime()) playChime()
      }
    },
    (next) => {
      setState(next)
      if (next === 'open') {
        clearOfflineTimer()
        setOffline(false)
        void reconcileSyncStatus(qc)
        return
      }
      // connecting：可能一秒内就好了，先不报。已经在倒计时就别重置——
      // 退避会让 connecting 反复触发，每次都重置计时器的话永远到不了阈值。
      if (offlineTimer == null) {
        offlineTimer = setTimeout(() => setOffline(true), OFFLINE_AFTER_MS)
      }
    })
    return () => {
      clearOfflineTimer()
      if (mailStateTimer != null) clearTimeout(mailStateTimer)
      close()
    }
  }, [qc])

  return { offline, state }
}

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import { CLIENT_ID } from '@/lib/client-id'
import { setNotifyPrefs, resetNotifyPrefsCache } from '@/lib/notify-prefs'
import type { RealtimeEvent } from '@/lib/types'

// 捕获 connectRealtime 注册的两个回调，直接把事件与状态喂进去，不必真起一条 SSE
let emit: ((ev: RealtimeEvent) => void) | null = null
let emitState: ((s: 'connecting' | 'open') => void) | null = null
const closeSpy = vi.fn()
vi.mock('@/lib/sse', () => ({
  connectRealtime: (
    cb: (ev: RealtimeEvent) => void,
    onState?: (s: 'connecting' | 'open') => void,
  ) => {
    emit = cb
    emitState = onState ?? null
    return closeSpy
  },
}))

const showMailNotice = vi.fn()
const playChime = vi.fn()
let hidden = true
let chimeClaimed = true
vi.mock('@/lib/browser-notify', () => ({
  showMailNotice: (...a: unknown[]) => showMailNotice(...a),
  playChime: () => playChime(),
  claimChime: () => chimeClaimed,
  pageHidden: () => hidden,
  notifyPermission: () => 'granted',
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const NEW_MAIL: RealtimeEvent = { type: 'new_mail', account_id: 1, folder_id: 2, new_count: 500 }
const NOTIFY: RealtimeEvent = {
  type: 'notify',
  event: 'mail_new',
  account_id: 1,
  message_id: 42,
  title: '新邮件 · 张三',
  body: '报销单据',
}

describe('useRealtimeSync', () => {
  let container: HTMLDivElement
  let root: Root

  function Harness(props: Parameters<typeof useRealtimeSync>[0]) {
    useRealtimeSync(props)
    return null
  }

  async function mount(props: Parameters<typeof useRealtimeSync>[0] = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <Harness {...props} />
        </QueryClientProvider>,
      )
    })
    return qc
  }

  async function fire(ev: RealtimeEvent) {
    await act(async () => emit?.(ev))
  }

  beforeEach(() => {
    emit = null
    emitState = null
    hidden = true
    chimeClaimed = true
    showMailNotice.mockClear()
    playChime.mockClear()
    localStorage.clear()
    resetNotifyPrefsCache()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('new_mail 只刷新缓存，绝不弹通知', async () => {
    // 这是整个设计的地基：new_mail 对基线导入、archive / junk 一律会发。
    // 拿它弹通知的话，用户首次添加账户导入几千封历史邮件时会被弹窗淹没。
    setNotifyPrefs({ desktop: true, sound: true })
    const qc = await mount()
    const spy = vi.spyOn(qc, 'invalidateQueries')

    await fire(NEW_MAIL)

    expect(showMailNotice).not.toHaveBeenCalled()
    expect(playChime).not.toHaveBeenCalled()
    // 缓存该失效的还是要失效
    const keys = spy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey))
    expect(keys.some((k) => k?.includes('messages'))).toBe(true)
    expect(keys.some((k) => k?.includes('aggregate-counts'))).toBe(true)
  })

  it('别的界面改了邮件状态 → 计数、列表与已打开的详情全部重取', async () => {
    // 这条事件存在的全部理由：同时开着多个界面时，标已读此前不发任何风声，
    // 而计数的三条自愈路径（轮询、focus 重取、SSE）当时一条都不通，
    // 未读角标能一直停在错值上到用户按 F5。
    vi.useFakeTimers()
    try {
      const qc = await mount()
      const spy = vi.spyOn(qc, 'invalidateQueries')

      await act(async () => emit?.({ type: 'mail_state', origin: 'another-tab' }))
      await act(async () => { vi.advanceTimersByTime(500) })

      const keys = spy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey))
      for (const k of ['folders', 'messages', 'threads', 'thread-messages', 'aggregate-counts', 'account-unread']) {
        expect(keys.some((key) => key?.includes(k)), `缺少 ${k}`).toBe(true)
      }
      // 别的界面改的可能正是此刻打开的那封（星标/标未读/删除），阅读窗格不能留旧值
      expect(keys.some((key) => key === JSON.stringify(['message']))).toBe(true)
      // 它只是「去重新拉」，与打扰用户无关
      expect(showMailNotice).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('连读刷屏时合并成一次失效', async () => {
    // invalidateQueries 对活动查询是立刻重取的，**不看标签页可见性**。
    // 用户在另一个标签页按住 j 连读，不合并的话这边就以每秒十几个请求打后端，
    // 而这边屏幕上根本没人在看。
    vi.useFakeTimers()
    try {
      const qc = await mount()
      const spy = vi.spyOn(qc, 'invalidateQueries')

      for (let i = 0; i < 10; i++) {
        await act(async () => emit?.({ type: 'mail_state', origin: 'another-tab' }))
        await act(async () => { vi.advanceTimersByTime(50) })
      }
      expect(spy, '防抖窗口内不该有任何失效').not.toHaveBeenCalled()

      await act(async () => { vi.advanceTimersByTime(500) })
      // 十条事件合成一轮：6 个列表/计数 key + 1 个详情 key
      expect(spy).toHaveBeenCalledTimes(7)
    } finally {
      vi.useRealTimers()
    }
  })

  it('自己那次操作的回声不再重取一遍', async () => {
    // 发起方在 mutation 的 onSettled 里已经失效过一轮。不认自己的话，
    // 每点一封邮件就要白打一轮请求：folders ×账户数 + 两个计数接口。
    vi.useFakeTimers()
    try {
      const qc = await mount()
      const spy = vi.spyOn(qc, 'invalidateQueries')

      await act(async () => emit?.({ type: 'mail_state', origin: CLIENT_ID }))
      await act(async () => { vi.advanceTimersByTime(500) })

      expect(spy).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('notify 且标签页不可见时，按偏好弹通知与提示音', async () => {
    setNotifyPrefs({ desktop: true, sound: true })
    const onOpenMessage = vi.fn()
    await mount({ onOpenMessage })

    await fire(NOTIFY)

    expect(showMailNotice).toHaveBeenCalledTimes(1)
    const arg = showMailNotice.mock.calls[0][0] as Record<string, unknown>
    expect(arg.title).toBe('新邮件 · 张三')
    expect(arg.body).toBe('报销单据')
    expect(arg.messageId).toBe(42)
    expect(playChime).toHaveBeenCalledTimes(1)
  })

  it('标签页就在眼前时不打扰', async () => {
    // 页面可见时列表已经自己刷新了，再弹一个系统通知只是噪音
    setNotifyPrefs({ desktop: true, sound: true })
    hidden = false
    await mount()

    await fire(NOTIFY)

    expect(showMailNotice).not.toHaveBeenCalled()
    expect(playChime).not.toHaveBeenCalled()
  })

  it('偏好关着就不弹', async () => {
    setNotifyPrefs({ desktop: false, sound: false })
    await mount()

    await fire(NOTIFY)

    expect(showMailNotice).not.toHaveBeenCalled()
    expect(playChime).not.toHaveBeenCalled()
  })

  it('不论偏好如何，读屏播报都要给', async () => {
    // 视觉用户看得见未读徽标跳变，读屏用户此前对新邮件完全无感知。
    // 播报不弹窗、不出声，没有理由跟着桌面通知的开关走。
    setNotifyPrefs({ desktop: false, sound: false })
    const onAnnounce = vi.fn()
    hidden = false
    await mount({ onAnnounce })

    await fire(NOTIFY)

    expect(onAnnounce).toHaveBeenCalledWith('新邮件 · 张三 报销单据')
  })

  it('别的标签页已经响过了，这个就不再响', async () => {
    // 通知本身有 tag 管着（多标签弹出的互相替换），但**声音没有 tag 这回事**：
    // 开着两个 FlyMail 窗口然后去干别的，同一批新邮件会听到两声叠在一起。
    setNotifyPrefs({ desktop: true, sound: true })
    chimeClaimed = false
    await mount()

    await fire(NOTIFY)

    // 通知照弹（它自己会按 tag 合并），只是不再重复出声
    expect(showMailNotice).toHaveBeenCalledTimes(1)
    expect(playChime).not.toHaveBeenCalled()
  })

  it('非新邮件的通知只刷新铃铛，不弹提醒', async () => {
    setNotifyPrefs({ desktop: true, sound: true })
    const onAnnounce = vi.fn()
    await mount({ onAnnounce })

    await fire({ ...NOTIFY, event: 'sync_failed', title: '同步失败', body: 'IMAP 连接超时' } as RealtimeEvent)

    expect(showMailNotice).not.toHaveBeenCalled()
    expect(onAnnounce).not.toHaveBeenCalled()
  })

  // ── 连接状态 ───────────────────────────────────────────────────────────
  //
  // 断开期间**收不到任何推送**：新邮件既不会让列表刷新，也不会弹通知。
  // 而「安静地不再收信」与「确实没有新邮件」在用户眼里完全一样——
  // 这正是 ui-audit 第 7 条里「合盖唤醒 / 后端重启后 UI 静默停止收信」那一句。

  it('刚断开时先不报——绝大多数重连一两秒就好了', async () => {
    vi.useFakeTimers()
    try {
      const seen: boolean[] = []
      function Probe() {
        const { offline } = useRealtimeSync()
        seen.push(offline)
        return null
      }
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      await act(async () => {
        root.render(
          <QueryClientProvider client={qc}>
            <Probe />
          </QueryClientProvider>,
        )
      })
      await act(async () => emitState?.('connecting'))
      await act(async () => {
        vi.advanceTimersByTime(3000)
      })
      expect(seen.at(-1), '才断了 3 秒就报离线，提示条会随着每次重连闪').toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('连不上超过阈值才报离线', async () => {
    vi.useFakeTimers()
    try {
      const seen: boolean[] = []
      function Probe() {
        const { offline } = useRealtimeSync()
        seen.push(offline)
        return null
      }
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      await act(async () => {
        root.render(
          <QueryClientProvider client={qc}>
            <Probe />
          </QueryClientProvider>,
        )
      })
      await act(async () => emitState?.('connecting'))
      await act(async () => {
        vi.advanceTimersByTime(8000)
      })
      expect(seen.at(-1)).toBe(true)

      // 连上了就立刻收回
      await act(async () => emitState?.('open'))
      expect(seen.at(-1)).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('退避期间反复 connecting 不会把倒计时一直往后推', async () => {
    // 指数退避会让 connecting 反复触发（1s、2s、4s…）。每次都重置计时器的话
    // 阈值永远到不了，离线提示一辈子不出现——而那恰恰是断得最久的情况。
    vi.useFakeTimers()
    try {
      const seen: boolean[] = []
      function Probe() {
        const { offline } = useRealtimeSync()
        seen.push(offline)
        return null
      }
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      await act(async () => {
        root.render(
          <QueryClientProvider client={qc}>
            <Probe />
          </QueryClientProvider>,
        )
      })
      for (let i = 0; i < 5; i++) {
        await act(async () => emitState?.('connecting'))
        await act(async () => {
          vi.advanceTimersByTime(2000)
        })
      }
      expect(seen.at(-1), '重连尝试把倒计时一直往后推了').toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  // ── 同步进度推送 ──────────────────────────────────────────────────────────
  describe('sync_status', () => {
    it('写进与轮询同一个缓存键，不走 invalidate', async () => {
      // 同一个键有三个写入方（触发时的乐观播种、轮询、这里的推送）和多个读者
      // （手动触发那一路、侧栏每个账户行）。写同一份是「谁在同步」只有一份真相的前提；
      // 走 invalidate 则会让每条事件都触发一次 HTTP，后台同步推几十条就是几十个请求。
      const qc = await mount()
      const spy = vi.spyOn(qc, 'invalidateQueries')

      await fire({
        type: 'sync_status',
        account_id: 7,
        phase: 'messages',
        folders_total: 12,
        folders_done: 3,
        current_folder: '收件箱',
      })

      expect(qc.getQueryData(['sync-status', 7])).toEqual({
        account_id: 7,
        phase: 'messages',
        folders_total: 12,
        folders_done: 3,
        current_folder: '收件箱',
      })
      expect(spy, 'sync_status 不该触发任何 invalidate').not.toHaveBeenCalled()
    })

    it('缓存里不留 type 字段——它是信封而不是状态', async () => {
      // 留着的话 SyncStatus 会多出一个来路不明的字段，而下一个人分不清
      // 它是后端给的还是前端塞的。
      const qc = await mount()
      await fire({ type: 'sync_status', account_id: 1, phase: 'folders' })
      expect(qc.getQueryData(['sync-status', 1])).not.toHaveProperty('type')
    })

    it('按账户分键，不会把 A 的进度写到 B 上', async () => {
      const qc = await mount()
      await fire({ type: 'sync_status', account_id: 1, phase: 'messages', folders_done: 2 })
      await fire({ type: 'sync_status', account_id: 2, phase: 'done' })
      expect((qc.getQueryData(['sync-status', 1]) as { phase: string }).phase).toBe('messages')
      expect((qc.getQueryData(['sync-status', 2]) as { phase: string }).phase).toBe('done')
    })
  })
})

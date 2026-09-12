import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import { setNotifyPrefs, resetNotifyPrefsCache } from '@/lib/notify-prefs'
import type { RealtimeEvent } from '@/lib/types'

// 捕获 connectRealtime 注册的回调，直接把事件喂进去，不必真起一条 SSE
let emit: ((ev: RealtimeEvent) => void) | null = null
const closeSpy = vi.fn()
vi.mock('@/lib/sse', () => ({
  connectRealtime: (cb: (ev: RealtimeEvent) => void) => {
    emit = cb
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
})

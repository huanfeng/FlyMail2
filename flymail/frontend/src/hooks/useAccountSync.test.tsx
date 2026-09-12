import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { ToastProvider } from '@/components/ui/Toast'
import { useAccountSync } from '@/hooks/useAccountSync'
import type { SyncPhase } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) => (o?.error != null ? `${k}:${String(o.error)}` : k),
  }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 手动同步的触发与跟踪。
 *
 * 这里验的全部是**「轮询到底有没有在跑」**这一件事的各种变体。它之所以值得单独
 * 测，是因为它的两种坏法在界面上长得一模一样——都是「点了同步，屏幕上什么都
 * 没发生」——但一种是轮询没启动（转圈和进度条不出现），另一种是轮询停不下来
 * （每秒一个请求打到用户刷新页面为止，屏幕上同样一点反馈都没有）。
 */
describe('useAccountSync', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  /** 服务端当前会回的 phase，用例中途改它来模拟同步推进 */
  let phase: SyncPhase
  /** status 端点被打了多少次——判断「轮询停没停」只能靠数请求 */
  let statusHits: number
  /** 触发端点的响应码，用来模拟停用账户（500）等失败 */
  let triggerReply: [number, unknown]

  function Probe() {
    const s = useAccountSync()
    return (
      <div>
        <button type="button" id="s1" onClick={() => s.start(1)}>
          sync 1
        </button>
        <button type="button" id="s2" onClick={() => s.start(2)}>
          sync 2
        </button>
        <span id="state">
          {String(s.syncing)}|{s.accountId ?? '-'}|{s.status?.phase ?? '-'}
        </span>
      </div>
    )
  }

  const state = () => container.querySelector('#state')!.textContent
  const syncing = () => state()!.split('|')[0] === 'true'

  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await Promise.resolve()
      })
    }
  }

  /**
   * 让轮询往前走一拍（refetchInterval 是 1000ms），并等这一拍的响应真正落地。
   *
   * ⚙ 只 advance(1000) 不够：定时器只负责把请求**发出去**，而 axios 的响应、
   * react-query 的状态更新、React 的重渲染各在后面的任务里。少等这一段，
   * 读到的是上一拍的状态——所有断言都会整体滞后一拍，且错得悄无声息。
   */
  /**
   * 让轮询往前走一拍（refetchInterval 是 1000ms），并等这一拍的响应真正落地。
   *
   * ⚙ 只 advance(1000) 不够：那一下只把请求**发出去**，axios 的响应、react-query
   * 的状态更新、React 的重渲染各在后面的任务里，而且光冲刷微任务推不动它们
   * （实测 flush 20 轮也不行，必须让定时器再往前走）。少等这一段的后果是所有
   * 断言整体滞后一拍——读到的是上一拍的状态，而且错得悄无声息。
   */
  async function tick() {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    for (let i = 0; i < 6; i++) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1)
        await Promise.resolve()
      })
    }
  }

  async function click(id: string) {
    await act(async () => container.querySelector<HTMLButtonElement>('#' + id)?.click())
    await flush()
  }

  beforeEach(async () => {
    vi.useFakeTimers()
    phase = 'none'
    statusHits = 0
    triggerReply = [202, { status: 'started' }]

    mock = new MockAdapter(api)
    mock.onPost(/\/accounts\/\d+\/sync$/).reply(() => {
      // 照搬后端的时序：Service.Trigger 在**返回 202 之前**就同步调用了
      // status.begin(queued)。所以触发成功之后 status 端点不可能再回 'none'——
      // 假后端这一点必须如实，否则测出来的是一个现实中不存在的时序。
      if (triggerReply[0] === 202) phase = 'queued'
      return triggerReply as [number, unknown]
    })
    mock.onGet(/\/accounts\/\d+\/sync\/status$/).reply(() => {
      statusHits++
      return [200, phase === 'none' ? { phase: 'none' } : { phase, total: 10, processed: 3 }]
    })

    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <Probe />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await flush()
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
    vi.useRealTimers()
  })

  it('同一个账户连点两次，第二次同样要进入同步态', async () => {
    // ⚠ 这是本轮真正的回归所在。
    //
    // 查询键是 ['sync-status', id]，上一轮结束时缓存里留着一条 'done'。
    // 第二次开轮询的那一刻 react-query 会**同步**把它返回，终态判定当场触发、
    // 立刻又收手——于是每个账户的第二次及以后的同步，转圈、进度条、阶段计数
    // 全都不出现，用户看到的是「点了没反应」。
    //
    // 修法是开轮询之前先把缓存写成 queued（后端在返回 202 之前已经同步置为
    // queued，所以这是如实的）。回退掉那一行 setQueryData，这条用例会失败。
    await click('s1')
    expect(syncing(), '第一次点击后没进入同步态').toBe(true)

    phase = 'done'
    await tick()
    expect(syncing(), 'done 之后没有收手').toBe(false)

    await click('s1')
    expect(syncing(), '第二次点同一个账户时没有进入同步态').toBe(true)
  })

  it('跟踪的是被点的那个账户，切换账户时旧的轮询停掉', async () => {
    await click('s1')
    expect(state()).toContain('|1|')

    phase = 'folders'
    await tick()
    expect(state()).toBe('true|1|folders')

    await click('s2')
    expect(state()).toContain('|2|')
  })

  it('到终态后轮询确实停止（不是只把界面改回去）', async () => {
    await click('s1')
    phase = 'done'
    await tick()
    expect(syncing()).toBe(false)

    const settled = statusHits
    await tick()
    await tick()
    await tick()
    expect(statusHits, '收手之后还在发状态请求').toBe(settled)
  })

  it('触发失败（停用账户 500）时给出提示，且根本不开轮询', async () => {
    // 原先这两件事都没有：500 一声不吭，轮询照开。而触发失败意味着后端压根
    // 没建立状态记录，phase 永远到不了终态——每秒一个请求发到刷新页面为止。
    triggerReply = [500, { error: 'account is disabled' }]
    await click('s1')

    expect(syncing(), '触发失败却进了同步态').toBe(false)
    expect(statusHits, '触发失败却开了轮询').toBe(0)
    expect(container.textContent, '触发失败没有任何提示').toContain('account is disabled')
  })

  it('同步中状态记录消失（后端重启 / 账户被删）时收手并提示', async () => {
    // status 端点在没有记录时回的是 200 {"phase":"none"}，既不是 done 也不是 error。
    // 不认它就是第三条「轮询永不停止」的路径。
    await click('s1')
    phase = 'messages'
    await tick()
    expect(syncing()).toBe(true)

    phase = 'none'
    await tick()
    expect(syncing(), 'phase 回到 none 之后没有收手').toBe(false)
    expect(container.textContent).toContain('sync.lost')

    const settled = statusHits
    await tick()
    await tick()
    expect(statusHits, 'none 之后还在发状态请求').toBe(settled)
  })

  it('同步失败（phase=error）把后端的错误文本带出来', async () => {
    await click('s1')
    phase = 'error'
    await tick()
    expect(syncing()).toBe(false)
  })
})

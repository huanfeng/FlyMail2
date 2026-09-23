import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { useSettings } from '@/lib/queries'
import type { AppSettings } from '@/lib/types'

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 推送正文上限的解析。
 *
 * 这条盯的是一个极易写出来的 bug：其它数字设置都写成
 * `Number(raw ?? 180) || 180`，照抄到这里就会**把 0 吞掉**——
 * 而 0 在这一项里是合法取值（表示「用内置默认」）。
 * 后果是用户存了 0，界面却显示 8000，看着像没保存成功，
 * 再点一次保存又把 8000 真的写进了库。
 */
describe('notify_body_runes 的解析', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let seen: AppSettings | null = null

  function Harness() {
    seen = useSettings().data ?? null
    return null
  }

  async function read(raw: Record<string, string>): Promise<AppSettings | null> {
    mock.onGet('/settings').reply(200, { settings: raw })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <Harness />
        </QueryClientProvider>,
      )
    })
    for (let i = 0; i < 4; i++) {
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
    return seen
  }

  beforeEach(() => {
    seen = null
    mock = new MockAdapter(api)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('0 原样保留，不被当成缺失值', async () => {
    const s = await read({ notify_body_runes: '0' })
    expect(s?.notify_body_runes).toBe(0)
  })

  it('正常值原样读出', async () => {
    const s = await read({ notify_body_runes: '3000' })
    expect(s?.notify_body_runes).toBe(3000)
  })

  it('缺失时回落到默认', async () => {
    const s = await read({})
    expect(s?.notify_body_runes).toBe(8000)
  })

  it('非法值回落到默认而不是 NaN', async () => {
    // NaN 会一路渗到 input 的 value 上，把输入框变成空白且无法输入
    const s = await read({ notify_body_runes: 'abc' })
    expect(s?.notify_body_runes).toBe(8000)
  })

  it('负数回落到默认', async () => {
    const s = await read({ notify_body_runes: '-1' })
    expect(s?.notify_body_runes).toBe(8000)
  })
})

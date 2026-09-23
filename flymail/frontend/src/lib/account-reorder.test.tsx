import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { useReorderAccounts } from '@/lib/queries'
import type { Account } from '@/lib/types'

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function acct(id: number, name: string): Account {
  return {
    id, name, email: `${name}@example.com`, auth_type: 'password',
    imap_host: 'i', imap_port: 993, imap_security: 'ssl',
    smtp_host: 's', smtp_port: 465, smtp_security: 'ssl',
    status: 'ok', enabled: true,
  }
}

/**
 * 账户排序的**乐观更新契约**。
 *
 * 排序是连续操作：把一个账户从第 4 位挪到第 1 位要点三次箭头。每点一次都等一个
 * 网络来回，列表就会在原地闪三次、而且中间那两次点击落在还没更新的列表上。
 * 所以这里钉的是「点下去列表立刻就变」，以及失败时必须弹回去——
 * 乐观更新最危险的不是不够快，是失败了还停在那个假状态上。
 */
describe('useReorderAccounts 的乐观更新', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let qc: QueryClient
  let fire: ((ids: number[]) => void) | null = null

  const initial = [acct(1, 'a'), acct(2, 'b'), acct(3, 'c')]

  function Harness() {
    const m = useReorderAccounts()
    fire = (ids) => m.mutate(ids)
    return null
  }

  function cachedIDs(): number[] {
    return (qc.getQueryData<Account[]>(['accounts']) ?? []).map((a) => a.id)
  }

  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
  }

  beforeEach(async () => {
    fire = null
    mock = new MockAdapter(api)
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData<Account[]>(['accounts'], initial)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <Harness />
        </QueryClientProvider>,
      )
    })
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('缓存在请求回来之前就已是新顺序', async () => {
    // 永不 resolve：这样断言到的必然是乐观写入，而不是响应回来后的重取结果
    mock.onPut('/accounts/order').reply(() => new Promise(() => {}))

    await act(async () => { fire!([3, 1, 2]) })
    expect(cachedIDs()).toEqual([3, 1, 2])
  })

  it('提交的是完整 ID 顺序', async () => {
    mock.onPut('/accounts/order').reply(200, { status: 'ok' })
    await act(async () => { fire!([2, 3, 1]) })
    await flush()

    const put = mock.history.find((h) => h.method === 'put')
    expect(put?.url).toBe('/accounts/order')
    expect(JSON.parse(put!.data)).toEqual({ ids: [2, 3, 1] })
  })

  it('失败时回滚到原顺序', async () => {
    // 409 = 后端认为提交的列表与库中账户集合对不上（另一个标签页动过账户）
    mock.onPut('/accounts/order').reply(409, { error: '排序列表与当前账户不一致' })
    mock.onGet('/accounts').reply(200, initial)

    await act(async () => { fire!([3, 2, 1]) })
    await flush(6)

    expect(cachedIDs()).toEqual([1, 2, 3])
  })

  // ids 理论上与缓存同集合。真要对不上（缓存里没有这个 id），宁可少一行，
  // 也不能让 undefined 混进数组——那会在渲染 account.name 时整个列表崩掉。
  it('缓存里找不到的 id 被丢弃，而不是留下 undefined', async () => {
    mock.onPut('/accounts/order').reply(() => new Promise(() => {}))

    await act(async () => { fire!([3, 999, 1, 2]) })
    const cached = qc.getQueryData<Account[]>(['accounts']) ?? []
    expect(cached.every((a) => a != null)).toBe(true)
    expect(cached.map((a) => a.id)).toEqual([3, 1, 2])
  })
})

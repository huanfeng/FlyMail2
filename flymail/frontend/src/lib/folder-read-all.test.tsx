import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { useFolderReadAll } from '@/lib/queries'

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 文件夹级「全部标为已读」的**网络契约**。
 *
 * 这条钉的不是"能不能标成已读"（那在后端 TestMarkFolderRead* 里验），
 * 而是**前端不许自己去凑 id 列表**。
 *
 * 最容易被写出来的实现是「先 GET 一遍未读、拿到 ids、再 POST /batch/read」。
 * 它在开发者自己的测试账户上完全正常（几十封），在真实收件箱上有两个后果：
 *
 *   1. 几万个 id 在网络上来回传两趟；
 *   2. 更糟——**只会标记前端分页拿到的那些**。用户点了「全部标为已读」，
 *      角标从 8000 掉到 7950，看起来像是坏了，实际是"全部"二字根本没兑现。
 *
 * 所以判据是：**一次 POST，零次 GET**。
 */
describe('useFolderReadAll 的网络契约', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let fire: ((folderId: number) => void) | null = null
  let lastResult: { marked: number } | null = null

  function Harness() {
    const m = useFolderReadAll()
    fire = (id: number) => m.mutate(id, { onSuccess: (r) => { lastResult = r } })
    return null
  }

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  beforeEach(async () => {
    fire = null
    lastResult = null
    mock = new MockAdapter(api)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
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

  it('一次 POST 到文件夹端点，不预先拉取任何 id', async () => {
    // ⚠ 具体 handler 注册在通配之前：axios-mock-adapter 按注册顺序匹配，
    // 反了的话 POST 会被通配吃掉，下面的断言变成对着空历史做检查。
    mock.onPost('/folders/7/read-all').reply(200, { status: 'ok', marked: 843 })
    mock.onAny(/.*/).reply(200, {})

    await act(async () => fire?.(7))
    await flush()

    expect(mock.history.post.map((r) => r.url)).toEqual(['/folders/7/read-all'])
    expect(
      mock.history.get.length,
      '前端自己去拉 id 了——真实收件箱上只会标记分页拿到的那部分',
    ).toBe(0)
  })

  it('请求体是空的——文件夹 id 在路径里，不该再塞一份 id 列表', async () => {
    mock.onPost('/folders/7/read-all').reply(200, { status: 'ok', marked: 1 })
    mock.onAny(/.*/).reply(200, {})

    await act(async () => fire?.(7))
    await flush()

    const body = mock.history.post[0].data
    expect(body == null || body === '' || body === '{}').toBe(true)
  })

  it('把后端报的封数交回调用方', async () => {
    // 调用方要拿它显示「已标记 843 封为已读」。后端分批执行时即使中途失败
    // 也会回报已完成数，所以这个值不能被丢掉。
    mock.onPost('/folders/7/read-all').reply(200, { status: 'ok', marked: 843 })
    mock.onAny(/.*/).reply(200, {})

    await act(async () => fire?.(7))
    await flush()

    expect(lastResult?.marked).toBe(843)
  })
})

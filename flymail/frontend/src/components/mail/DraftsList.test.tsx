import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { DraftsList } from '@/components/mail/DraftsList'

// i18n 换成「原样返回 key」：这里验的是故障与空态的分流，文案由 locales.test.ts 兜住。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('DraftsList', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  /** 把挂起的宏任务跑完（axios → react-query → React 重渲染分属不同队列）。 */
  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0))
      })
    }
  }

  let qcRef: QueryClient | null = null

  async function mount() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qcRef = qc
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <DraftsList accountId={1} onOpenDraft={vi.fn()} />
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  beforeEach(() => {
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

  it('请求失败时显示错误态，而不是把故障谎报成空草稿箱', async () => {
    mock.onGet('/accounts/1/drafts').reply(500)

    await mount()

    expect(container.textContent).toContain('list.loadErrorTitle')
    // 关键：绝不能落进空态分支
    expect(container.textContent).not.toContain('compose.noDrafts')
    const retry = [...container.querySelectorAll('button')].find((b) => b.textContent === 'app.retry')
    expect(retry).toBeDefined()
  })

  it('点重试重新取数，成功后正常显示草稿', async () => {
    mock.onGet('/accounts/1/drafts').replyOnce(500)
    await mount()

    mock.onGet('/accounts/1/drafts').reply(200, {
      drafts: [{ id: 7, account_id: 1, subject: '未写完的信', to: ['a@b.c'], cc: [], bcc: [], body: '' }],
    })
    const retry = [...container.querySelectorAll('button')].find((b) => b.textContent === 'app.retry')!
    await act(async () => {
      retry.click()
    })
    await flush()

    expect(container.textContent).toContain('未写完的信')
    expect(container.textContent).not.toContain('list.loadErrorTitle')
  })

  it('已有草稿时的后台重取失败不掀掉列表', async () => {
    mock.onGet('/accounts/1/drafts').replyOnce(200, {
      drafts: [{ id: 7, account_id: 1, subject: '未写完的信', to: [], cc: [], bcc: [], body: '' }],
    })
    await mount()
    expect(container.textContent).toContain('未写完的信')

    // 发信/删草稿都会 invalidate ['drafts']，重取失败时 isError 为真而数据还在
    mock.onGet('/accounts/1/drafts').reply(500)
    await act(async () => {
      await qcRef!.invalidateQueries({ queryKey: ['drafts'] })
    })
    await flush()

    expect(container.textContent).toContain('未写完的信')
    expect(container.textContent).not.toContain('list.loadErrorTitle')
  })

  it('真的没有草稿时仍走空态', async () => {
    mock.onGet('/accounts/1/drafts').reply(200, { drafts: [] })

    await mount()

    expect(container.textContent).toContain('compose.noDrafts')
    expect(container.textContent).not.toContain('list.loadErrorTitle')
  })
})

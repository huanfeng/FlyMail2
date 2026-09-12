import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { TrustedSendersSection } from '@/components/settings/TrustedSendersSection'
import { ConfirmProvider } from '@/components/ui/Confirm'

// i18n 在测试里换成「原样返回 key」：这里要验的是名单渲染与删除链路，
// 不是文案本身（文案由 locales.test.ts 的键集校验兜住）。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

// React 19 的 act 需要这个标志，否则会警告并跳过副作用刷新
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('TrustedSendersSection', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  /**
   * 把挂起的宏任务跑完。
   *
   * ⚙ 单转一次 await 不够：axios 的响应、react-query 的状态更新、React 的重渲染
   * 分属不同的任务队列，少等一轮拿到的就是首渲染的空态——
   * 而空态正好也是其中一个用例的期望值，错得悤无声息。
   */
  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0))
      })
    }
  }

  /** 挂载组件并等首次请求落地 */
  async function mount() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          {/* 删除走 useConfirm()，它在没有 Provider 时会抛——这是有意的，
              让「忘了挂 Provider」在开发期就暴露，而不是安静地退回 window.confirm。 */}
          <ConfirmProvider>
            <TrustedSendersSection />
          </ConfirmProvider>
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  /**
   * 在确认框上点「确定」或「取消」。
   *
   * 对话框走 radix 的 Portal，挂在 document.body 上而**不在 container 里**——
   * 从 container 查永远查不到，那会让断言静默落空成「没有按钮所以没点」。
   */
  async function answerConfirm(ok: boolean) {
    const dialog = document.querySelector('.confirm-dialog')
    expect(dialog, '确认框没有出现').not.toBeNull()
    // 按位置取：取消在前、确认在后。确认键的文案各处不同（删除 / 确定），
    // 按文案找会把这个 helper 绑死在某一处的措辞上。
    const btns = dialog!.querySelectorAll<HTMLButtonElement>('.confirm-actions button')
    expect(btns.length, '确认框应当有取消与确认两个按钮').toBe(2)
    expect(btns[0].textContent, '第一个按钮应是取消——默认落点不该是删除').toBe('common.cancel')
    const btn = btns[ok ? 1 : 0]
    await act(async () => btn!.click())
    await flush()
  }

  beforeEach(() => {
    mock = new MockAdapter(api)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
    mock.restore()
    vi.restoreAllMocks()
  })

  it('名单为空时显示空态，不渲染任何条目行', async () => {
    mock.onGet('/privacy/trusted-senders').reply(200, { senders: [] })
    await mount()
    expect(container.textContent).toContain('settings.privacy.trusted.none')
    expect(container.querySelectorAll('.settings-list-row').length).toBe(0)
  })

  it('渲染名单里的地址', async () => {
    mock.onGet('/privacy/trusted-senders').reply(200, {
      senders: [
        { id: 1, address: 'alice@example.com', created_at: '2026-09-01T10:00:00Z' },
        { id: 2, address: 'bob@example.com', created_at: '2026-09-02T10:00:00Z' },
      ],
    })
    await mount()
    expect(container.querySelectorAll('.settings-list-row').length).toBe(2)
    expect(container.textContent).toContain('alice@example.com')
    expect(container.textContent).toContain('bob@example.com')
  })

  it('确认后才发删除请求，取消则什么都不做', async () => {
    mock.onGet('/privacy/trusted-senders').reply(200, {
      senders: [{ id: 7, address: 'alice@example.com', created_at: '2026-09-01T10:00:00Z' }],
    })
    let deleted = 0
    mock.onDelete('/privacy/trusted-senders/7').reply(() => {
      deleted++
      return [200, { status: 'ok' }]
    })
    await mount()

    const btn = container.querySelector<HTMLButtonElement>('.settings-list-row button')
    expect(btn).not.toBeNull()

    // 取消：撤销信任是会影响后续所有邮件的动作，不该点一下就生效
    await act(async () => {
      btn?.click()
    })
    await flush()
    await answerConfirm(false)
    expect(deleted).toBe(0)

    await act(async () => {
      btn?.click()
    })
    await flush()
    await answerConfirm(true)
    expect(deleted).toBe(1)
  })

  it('删除失败时把后端文案显示出来，条目留在原地', async () => {
    mock.onGet('/privacy/trusted-senders').reply(200, {
      senders: [{ id: 7, address: 'alice@example.com', created_at: '2026-09-01T10:00:00Z' }],
    })
    mock.onDelete('/privacy/trusted-senders/7').reply(500, { error: '数据库忙' })
    await mount()

    await act(async () => {
      container.querySelector<HTMLButtonElement>('.settings-list-row button')?.click()
    })
    await flush()
    await answerConfirm(true)

    expect(container.textContent).toContain('数据库忙')
    expect(container.querySelectorAll('.settings-list-row').length).toBe(1)
  })
})

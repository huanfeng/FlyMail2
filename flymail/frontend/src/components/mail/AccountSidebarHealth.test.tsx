import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { ToastProvider } from '@/components/ui/Toast'
import { ConfirmProvider } from '@/components/ui/Confirm'
import { requestOpenSettings } from '@/lib/settings-nav'
import type { Account } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join('/')}` : k,
  }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

// 只换掉 requestOpenSettings，其余导出（页面 ID 表等）保持真实实现——
// 其它模块也可能从这个模块拿常量。
vi.mock('@/lib/settings-nav', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/settings-nav')>()),
  requestOpenSettings: vi.fn(),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function account(id: number, name: string, health?: Account['health']): Account {
  return {
    id,
    name,
    email: `${name}@example.com`,
    auth_type: 'password',
    imap_host: 'i',
    imap_port: 993,
    imap_security: 'ssl',
    smtp_host: 's',
    smtp_port: 465,
    smtp_security: 'ssl',
    status: health?.needs_reauth ? 'needs_reauth' : 'ok',
    enabled: true,
    health,
  }
}

const HEALTHY = account(1, 'a')
const REAUTH = account(2, 'b', { needs_reauth: true, breaker_open: false, failures: 0 })
const BREAKER = account(3, 'c', { needs_reauth: false, breaker_open: true, failures: 5 })

/** 健康徽标：账户行里 class 带 acct-health 的 span */
function badge(container: HTMLElement): HTMLElement | null {
  return container.querySelector('.acct-health')
}

describe('侧栏健康徽标点击直达账户管理', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  async function renderWith(accounts: Account[]) {
    const props = {
      accounts,
      activeAccountId: accounts[0]?.id ?? null,
      activeFolderId: null,
      notifOpen: false,
      settingsOpen: false,
      connState: 'open' as const,
      activeAgg: null,
      aggCounts: { inbox: 0, unread: 0, starred: 0 },
      onSelectAccount: vi.fn(),
      onSelectFolder: vi.fn(),
      onSync: vi.fn(),
      onToggleNotif: vi.fn(),
      onToggleSettings: vi.fn(),
      onSelectAgg: vi.fn(),
      onSelectAggregate: vi.fn(),
      onCompose: vi.fn(),
      onAddAccount: vi.fn(),
      onOpenDrafts: vi.fn(),
      draftsAccountId: null,
    }
    await act(async () => {
      root.render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <ConfirmProvider>
              <AccountSidebar {...props} />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    return props
  }

  beforeEach(() => {
    vi.mocked(requestOpenSettings).mockClear()
    mock = new MockAdapter(api)
    mock.onGet('/accounts/oauth/providers').reply(200, [])
    mock.onGet(/.*/).reply(200, {})
    mock.onPost(/.*/).reply(200, {})
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => { root.unmount() })
    container.remove()
    mock.restore()
  })

  it('needs_reauth 徽标点击打开账户管理，不触发账户展开', async () => {
    const props = await renderWith([HEALTHY, REAUTH])

    const el = badge(container)
    expect(el, 'needs_reauth 徽标应渲染').toBeTruthy()
    await act(async () => { el!.click() })

    expect(requestOpenSettings).toHaveBeenCalledWith('accounts')
    expect(props.onSelectAccount).not.toHaveBeenCalled()
  })

  it('熔断徽标点击同样打开账户管理', async () => {
    await renderWith([HEALTHY, BREAKER])

    const el = badge(container)
    expect(el, '熔断徽标应渲染').toBeTruthy()
    await act(async () => { el!.click() })

    expect(requestOpenSettings).toHaveBeenCalledWith('accounts')
  })

  it('健康账户无徽标', async () => {
    await renderWith([HEALTHY])
    expect(badge(container)).toBe(null)
  })
})

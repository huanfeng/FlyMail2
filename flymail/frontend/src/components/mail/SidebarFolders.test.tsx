import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { ToastProvider } from '@/components/ui/Toast'
import { ConfirmProvider } from '@/components/ui/Confirm'
import type { Account, Folder } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join('/')}` : k,
  }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function account(id: number, email: string): Account {
  return {
    id,
    name: email,
    email,
    auth_type: 'password',
    imap_host: 'imap.example.com',
    imap_port: 993,
    imap_security: 'ssl',
    smtp_host: 'smtp.example.com',
    smtp_port: 465,
    smtp_security: 'ssl',
    status: 'ok',
    enabled: true,
  }
}

function folder(id: number, accountID: number, path: string): Folder {
  return {
    id,
    account_id: accountID,
    path,
    display_name: path,
    type: path === 'INBOX' ? 'inbox' : 'custom',
    selectable: true,
    total_count: 1,
    unread_count: 0,
    sort_order: id,
  }
}

/**
 * 同时展开多个账户时，每个账户都要显示自己的文件夹。
 *
 * ── 缘起（2026-09-16 用户报「多个账户展开时，除本地草稿箱外的文件夹消失」） ───
 *
 * 侧栏原先只给**当前激活**账户传文件夹：
 *
 *	folders={acc.id === activeAccountId ? folders : []}
 *
 * 于是同时展开两个账户时，非激活的那个整棵树是空的，只剩「草稿箱（本地）」
 * 那一行——它是客户端渲染的，不依赖接口。而侧栏默认就展开前两个账户，
 * 一打开界面就能看到。
 *
 * 设计上从来没有「只能展开一个账户」这条限制，所以要补的是取数，不是加限制。
 */
describe('侧栏：多账户同时展开', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  /** 每个账户的文件夹接口被请求了几次 */
  let hits: Record<number, number>

  const props = {
    accounts: [account(1, 'a@example.com'), account(2, 'b@example.com')],
    activeAccountId: 1,
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
  }

  async function render() {
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
  }

  beforeEach(() => {
    hits = { 1: 0, 2: 0 }
    mock = new MockAdapter(api)
    mock.onGet(/\/accounts\/(\d+)\/folders/).reply((cfg) => {
      const id = Number(/\/accounts\/(\d+)\/folders/.exec(cfg.url ?? '')?.[1])
      hits[id] = (hits[id] ?? 0) + 1
      return [200, { folders: [folder(id * 10, id, 'INBOX'), folder(id * 10 + 1, id, `仅属于账户${id}`)] }]
    })
    // ⚠ 这个要在下面的兜底之前注册：账户对话框会拉 OAuth 提供方，
    // 兜底返回的 {} 会让它在 .filter 上炸掉，掩盖真正要测的东西。
    mock.onGet('/accounts/oauth/providers').reply(200, [])
    mock.onGet(/.*/).reply(200, {})
    mock.onPost(/.*/).reply(200, {})
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('两个账户默认都展开时，各自的文件夹都在', async () => {
    await render()

    // 侧栏默认展开前两个账户，所以两个账户的接口都该被请求
    expect(hits[1], '激活账户的文件夹没取').toBeGreaterThan(0)
    expect(hits[2], '非激活账户压根没发请求——它的文件夹树会是空的').toBeGreaterThan(0)

    const text = container.textContent ?? ''
    expect(text, '激活账户的文件夹没渲染').toContain('仅属于账户1')
    expect(text, '非激活账户展开着却没有文件夹，只剩本地草稿箱').toContain('仅属于账户2')
  })

  it('折叠的账户不发请求', async () => {
    // 三个账户时默认只展开前两个，第三个是折叠的
    const three = { ...props, accounts: [...props.accounts, account(3, 'c@example.com')] }
    await act(async () => {
      root.render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <ConfirmProvider>
              <AccountSidebar {...three} />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })

    expect(hits[3] ?? 0, '折叠的账户不该发请求：白白占着连接还要每 30 秒轮询一次').toBe(0)
  })
})

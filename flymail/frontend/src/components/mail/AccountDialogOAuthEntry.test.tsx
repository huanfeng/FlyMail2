// 「添加账户」里 OAuth 入口的显示规则。
//
// 这条规则此前错在一个很难自查的方向上：未配置凭据时 `.filter(p => p.configured)`
// 把入口整个删掉，界面上什么都不剩。用户看到的是「这个功能没做」，
// 而真相是「还差一步配置」——两者在界面上长得一模一样。
//
// 后端本来就是照「一并返回、由前端置灰」的契约写的（account.OAuthProviders 头注释），
// 所以这里钉住的是那份契约，不只是当前实现。

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AccountDialog } from '@/components/mail/AccountDialog'
import type { OAuthProviderInfo } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o?.provider ? `${k}:${String(o.provider)}` : k) }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function provider(over: Partial<OAuthProviderInfo> = {}): OAuthProviderInfo {
  return {
    id: 'google',
    name: 'Gmail',
    configured: true,
    device_code: false,
    imap_host: 'imap.gmail.com',
    smtp_host: 'smtp.gmail.com',
    redirect_uri: '',
    password_auth: true,
    ...over,
  }
}

describe('AccountDialog 的 OAuth 入口', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  /**
   * 等到条件成立，而不是数固定的刷新次数。
   *
   * 这里原来是 `await Promise.resolve()` 四次，单跑能过、全量跑会随机挂：
   * providers 请求的 resolve、react-query 的状态更新与 React 重渲染分属不同队列，
   * 只清微任务清不干净，机器一忙就差那么一两拍——而组件在数据回来前渲染的是
   * 「没有任何 OAuth 入口」，和「入口被删掉了」长得一模一样，于是断言假失败。
   */
  async function waitFor(cond: () => boolean, timeoutMs = 2000) {
    const deadline = Date.now() + timeoutMs
    while (!cond()) {
      if (Date.now() > deadline) throw new Error('等待超时：OAuth 入口始终没渲染出来')
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
  }

  async function mount(providers: OAuthProviderInfo[]) {
    mock.onGet('/accounts/oauth/providers').reply(200, providers)
    mock.onGet(/\/accounts$/).reply(200, [])
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <AccountDialog open onOpenChange={() => {}} account={null} />
        </QueryClientProvider>,
      )
    })
    // 四条用例都至少有一个提供方，所以「按钮出现」是个对全部用例都成立的稳定锚点。
    await waitFor(() => oauthButtons().length > 0)
  }

  /**
   * 取所有「用 X 登录」按钮。
   * 查 document.body 而不是 container：Radix Dialog 的内容渲染在 Portal 里，
   * 挂载点之下什么都没有。
   */
  function oauthButtons(): HTMLButtonElement[] {
    return [...document.body.querySelectorAll('button')].filter((b) =>
      (b.textContent ?? '').includes('account.oauthSignIn'),
    ) as HTMLButtonElement[]
  }

  beforeEach(() => {
    mock = new MockAdapter(api)
    // ⚠ 先清空 body。本文件的断言查的是 document.body（Dialog 内容在 Portal 里，
    // 挂载点之下什么都没有），而 Radix 的 Portal 节点在 unmount 后会残留——
    // 上一条用例的文案会漏进下一条，表现为「单跑这个文件全过、全量跑却挂一条」。
    document.body.innerHTML = ''
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('未配置凭据时入口仍在，只是置灰并说明去哪配', async () => {
    await mount([provider({ configured: false })])

    const buttons = oauthButtons()
    expect(buttons.length, '未配置时入口不该凭空消失').toBe(1)
    expect(buttons[0].disabled).toBe(true)
    // 光置灰不够：还得告诉用户这一步缺在哪，否则和「坏了」无从区分
    expect(document.body.textContent).toContain('account.oauthNotConfigured')
  })

  it('配好凭据后入口可点，且不再显示提示', async () => {
    await mount([provider({ configured: true })])

    const buttons = oauthButtons()
    expect(buttons.length).toBe(1)
    expect(buttons[0].disabled).toBe(false)
    expect(document.body.textContent).not.toContain('account.oauthNotConfigured')
  })

  // 「没配凭据」对两家服务商的后果不一样，提示也不能一样：
  // Gmail 还能用应用专用密码手填顶上，Outlook 个人账户 2024-09-16 起
  // 连基本认证都关了，压根没有那条退路。提示给反了，用户会去翻一个
  // 根本不存在的设置，翻半天然后断定是 FlyMail 坏了。

  it('Gmail 未配置时，额外告知可用应用专用密码手填', async () => {
    await mount([provider({ id: 'google', name: 'Gmail', configured: false, password_auth: true })])
    expect(document.body.textContent).toContain('account.oauthPasswordFallback')
  })

  it('Outlook 未配置时不给这条退路——它已经没有口令认证了', async () => {
    await mount([provider({ id: 'microsoft', name: 'Outlook', configured: false, password_auth: false })])
    expect(document.body.textContent).toContain('account.oauthNotConfigured')
    expect(document.body.textContent).not.toContain('account.oauthPasswordFallback')
  })
})

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { OAuthPanel } from '@/components/mail/OAuthPanel'
import type { OAuthProviderInfo } from '@/lib/types'

// i18n 换成「原样返回 key」：这里要验的是授权链路，不是文案（文案由 locales.test.ts 兜住）。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

// openExternal 在 jsdom 里会真的调 window.open，这里只关心它被调用时带了什么地址。
const openExternal = vi.fn()
vi.mock('@/lib/platform', () => ({
  openExternal: (url: string) => openExternal(url),
  isDesktop: () => false,
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const GOOGLE: OAuthProviderInfo = {
  id: 'google',
  name: 'Gmail',
  configured: true,
  device_code: false,
  imap_host: 'imap.gmail.com',
  smtp_host: 'smtp.gmail.com',
  // 空串 = 走 loopback（后端与浏览器同机），这些用例本来就是照 loopback 写的
  redirect_uri: '',
  password_auth: true,
}

describe('OAuthPanel', () => {
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

  async function mount(props: Partial<React.ComponentProps<typeof OAuthPanel>> = {}) {
    const onDone = vi.fn()
    const onCancel = vi.fn()
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <OAuthPanel provider={GOOGLE} onDone={onDone} onCancel={onCancel} {...props} />
        </QueryClientProvider>,
      )
    })
    await flush()
    return { onDone, onCancel }
  }

  beforeEach(() => {
    mock = new MockAdapter(api)
    openExternal.mockClear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('挂载即发起授权，并把授权地址交给系统浏览器', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f1',
      provider: 'google',
      mode: 'code',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      auth_url: 'https://accounts.google.com/o/oauth2/v2/auth?x=1',
    })
    mock.onGet('/accounts/oauth/flows/f1').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code', status: 'pending',
    })

    await mount()

    const startCalls = mock.history.post.filter((r) => r.url === '/accounts/oauth/start')
    expect(startCalls).toHaveLength(1)

    const button = [...container.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('account.oauthOpen'))
    expect(button).toBeTruthy()
    await act(async () => { button!.click() })
    expect(openExternal).toHaveBeenCalledWith('https://accounts.google.com/o/oauth2/v2/auth?x=1')
  })

  it('授权成功后自动落库，且只提交一次', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      auth_url: 'https://example.com/auth',
    })
    // 轮询直接返回成功：complete 应被触发。
    mock.onGet('/accounts/oauth/flows/f1').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code', status: 'success', email: 'me@gmail.com',
    })
    mock.onPost('/accounts/oauth/complete').reply(200, {
      id: 7, name: 'me', email: 'me@gmail.com', auth_type: 'oauth', oauth_provider: 'google',
      imap_host: 'imap.gmail.com', imap_port: 993, imap_security: 'ssl',
      smtp_host: 'smtp.gmail.com', smtp_port: 465, smtp_security: 'ssl',
      status: 'new', enabled: true,
    })

    const { onDone } = await mount()
    await flush(6)

    const completes = mock.history.post.filter((r) => r.url === '/accounts/oauth/complete')
    // 轮询与 mutation 分处不同队列，没有守卫就会重复提交、建出多个账户。
    expect(completes).toHaveLength(1)
    expect(JSON.parse(completes[0].data)).toMatchObject({ flow_id: 'f1', email: 'me@gmail.com' })
    expect(onDone).toHaveBeenCalledTimes(1)
    expect(onDone.mock.calls[0][0]).toMatchObject({ id: 7, email: 'me@gmail.com' })
  })

  it('授权失败时展示后端给出的原因，且不建号', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      auth_url: 'https://example.com/auth',
    })
    mock.onGet('/accounts/oauth/flows/f1').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code',
      status: 'failed', error: '授权被拒绝: 用户取消了授权',
    })

    const { onDone } = await mount()
    await flush(6)

    expect(container.textContent).toContain('用户取消了授权')
    expect(mock.history.post.filter((r) => r.url === '/accounts/oauth/complete')).toHaveLength(0)
    expect(onDone).not.toHaveBeenCalled()
  })

  it('发起失败时展示后端原因（如未配置凭据）', async () => {
    mock.onPost('/accounts/oauth/start').reply(501, { error: '未配置该提供方的 OAuth 客户端凭据' })

    await mount()
    await flush()

    expect(container.textContent).toContain('未配置该提供方的 OAuth 客户端凭据')
  })

  it('设备码流程展示用户码与验证地址', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f2', provider: 'microsoft', mode: 'device',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      user_code: 'ABCD-EFGH',
      verification_uri: 'https://microsoft.com/devicelogin',
    })
    mock.onGet('/accounts/oauth/flows/f2').reply(200, {
      flow_id: 'f2', provider: 'microsoft', mode: 'device', status: 'pending',
    })

    await mount({ mode: 'device' })

    expect(container.textContent).toContain('ABCD-EFGH')
    expect(container.textContent).toContain('https://microsoft.com/devicelogin')
    expect(JSON.parse(mock.history.post[0].data)).toMatchObject({ mode: 'device' })
  })

  it('取消进行中的流程时通知后端释放回调端口', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      auth_url: 'https://example.com/auth',
    })
    mock.onGet('/accounts/oauth/flows/f1').reply(200, {
      flow_id: 'f1', provider: 'google', mode: 'code', status: 'pending',
    })
    mock.onDelete('/accounts/oauth/flows/f1').reply(200, { status: 'ok' })

    const { onCancel } = await mount()
    const cancelButton = [...container.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('account.cancel'))
    await act(async () => { cancelButton!.click() })
    await flush()

    expect(mock.history.delete.filter((r) => r.url === '/accounts/oauth/flows/f1')).toHaveLength(1)
    expect(onCancel).toHaveBeenCalled()
  })

  it('重新授权时把账户 ID 一并带上，避免建成新账户', async () => {
    mock.onPost('/accounts/oauth/start').reply(200, {
      flow_id: 'f3', provider: 'google', mode: 'code',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      auth_url: 'https://example.com/auth',
    })
    mock.onGet('/accounts/oauth/flows/f3').reply(200, {
      flow_id: 'f3', provider: 'google', mode: 'code', status: 'pending',
    })

    await mount({ accountId: 42, hintEmail: 'me@gmail.com' })

    expect(JSON.parse(mock.history.post[0].data)).toMatchObject({
      provider: 'google', account_id: 42, email: 'me@gmail.com',
    })
  })
})

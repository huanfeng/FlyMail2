import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { OAuthSection } from '@/components/settings/OAuthSection'
import zh from '@/locales/zh.json'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('OAuthSection', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  // 用宏任务而不是 await Promise.resolve()：这里有两个并发请求（/settings 与
  // /accounts/oauth/providers），axios 的响应、react-query 的状态更新与 React 的
  // 重渲染分属不同队列，只清微任务会让后到的那个请求看着像「没数据」。
  async function flush(times = 6) {
    for (let i = 0; i < times; i++) {
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
  }

  async function mount(opts: {
    settings?: Record<string, string>
    redirectUri?: string
  } = {}) {
    mock.onGet('/settings').reply(200, { settings: opts.settings ?? {} })
    mock.onGet('/accounts/oauth/providers').reply(200, [
      {
        id: 'google', name: 'Gmail', configured: true, device_code: false,
        imap_host: 'imap.gmail.com', smtp_host: 'smtp.gmail.com',
        redirect_uri: opts.redirectUri ?? '', password_auth: true,
      },
    ])
    mock.onPut('/settings').reply(200, { status: 'ok' })

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <OAuthSection />
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  function byId<T extends HTMLElement>(id: string): T {
    const el = container.querySelector(`#${id}`)
    if (!el) throw new Error(`找不到 #${id}`)
    return el as T
  }

  function buttonWith(text: string): HTMLButtonElement {
    const btn = [...container.querySelectorAll('button')].find((b) =>
      (b.textContent ?? '').includes(text),
    )
    if (!btn) throw new Error(`找不到按钮 ${text}`)
    return btn as HTMLButtonElement
  }

  /** 取最后一次 PUT /settings 的请求体 */
  function lastPut(): Record<string, string> {
    const puts = mock.history.filter((h) => h.method === 'put')
    return JSON.parse(puts[puts.length - 1].data).settings
  }

  async function type(el: HTMLInputElement, value: string) {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(el, value)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    })
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

  it('只改 client_id 保存时，不把已存的 secret 连带抹掉', async () => {
    // 后端把空串当作「清除」。界面上那个空的密钥输入框意思是「不动它」，
    // 两者混为一谈的话，用户改个 client_id 就会把 secret 抹掉，
    // 而且毫无提示——下一次授权才会以 invalid_client 的形式爆出来。
    await mount({ settings: { oauth_google_client_secret_set: 'true' } })

    await type(byId<HTMLInputElement>('oauth-google-client-id'), 'new-id.apps.googleusercontent.com')
    await act(async () => buttonWith('settings.oauth.save').click())
    await flush()

    const body = lastPut()
    expect(body.oauth_google_client_id).toBe('new-id.apps.googleusercontent.com')
    expect('oauth_google_client_secret' in body, '没输入新密钥就不该提交这个字段').toBe(false)
  })

  it('输入了新密钥才随保存一起提交', async () => {
    await mount({ settings: { oauth_google_client_secret_set: 'true' } })

    await type(byId<HTMLInputElement>('oauth-google-client-secret'), 'GOCSPX-new')
    await act(async () => buttonWith('settings.oauth.save').click())
    await flush()

    expect(lastPut().oauth_google_client_secret).toBe('GOCSPX-new')
  })

  // ⚠ 每个用例一次 mount。同一个 it 里 unmount 再 mount 是不行的：
  // axios-mock-adapter 同一路径注册多次时**第一个 handler 优先**，第二次拿到的
  // 还是上一次的数据；而组件在数据回来前的空态又恰好和「没配」长得一样，
  // 于是断言会假通过——这条测试自己先踩过一次。

  it('未保存过密钥时不显示「清除」', async () => {
    await mount({ settings: { oauth_google_client_secret_set: 'false' } })
    expect(
      [...container.querySelectorAll('button')].some((b) => (b.textContent ?? '').includes('clearSecret')),
    ).toBe(false)
  })

  it('「清除」显式提交空串', async () => {
    await mount({ settings: { oauth_google_client_secret_set: 'true' } })
    await act(async () => buttonWith('settings.oauth.clearSecret').click())
    await flush()
    expect(lastPut().oauth_google_client_secret).toBe('')
  })

  it('配了对外地址就显示回调地址供复制', async () => {
    await mount({ redirectUri: 'https://mail.example.com/api/v1/accounts/oauth/callback' })
    expect(container.querySelector('code')?.textContent)
      .toBe('https://mail.example.com/api/v1/accounts/oauth/callback')
  })

  it('没配对外地址则给出该去哪补的提示，而不是一个空地址栏', async () => {
    await mount({ redirectUri: '' })
    expect(container.querySelector('code')).toBeNull()
    expect(container.textContent).toContain('settings.oauth.redirectUriMissing')
  })

  it('外部操作说明收在折叠里，默认不展开', async () => {
    // 这些步骤（去哪个后台、哪一步不做会 403）删不得，但也不该每次打开设置
    // 都占半屏——真正要看它的只有第一次配置那一遍。
    await mount()
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details!.open).toBe(false)
    expect(details!.textContent).toContain('settings.oauth.guideStep1')
  })

  it('每一步都给出控制台直达链接，且在新标签页安全打开', async () => {
    // 光写「进入 OAuth 同意屏幕」是不够的：控制台改过菜单名，
    // 「新建项目」更是压根不在任何菜单项下（在顶栏项目选择器的弹窗里）。
    // 没有直达地址，用户就会卡在「搜了一圈找不到入口」。
    await mount()
    const steps = [...container.querySelectorAll('.settings-guide li')]
    expect(steps.length, '指引不该是空的').toBeGreaterThan(0)

    for (const [i, li] of steps.entries()) {
      const a = li.querySelector('a')
      expect(a, `第 ${i + 1} 步缺少直达链接`).not.toBeNull()
      expect(a!.getAttribute('href')).toMatch(/^https:\/\/console\.cloud\.google\.com\//)
      expect(a!.getAttribute('target')).toBe('_blank')
      // 少了 noopener，新开的页面能拿到 window.opener 反向操纵本页（含设置页）
      expect(a!.getAttribute('rel') ?? '').toContain('noopener')
    }
  })

  it('步骤条数与语言包里的 guideStepN 对得上', async () => {
    // 对不上不会报错，只会把 "settings.oauth.guideStep6" 这样的原始 key
    // 直接渲染给用户看，或者静默吞掉最后一步。
    await mount()
    const rendered = container.querySelectorAll('.settings-guide li').length
    const declared = Object.keys(zh.settings.oauth).filter((k) => /^guideStep\d+$/.test(k)).length
    expect(rendered).toBe(declared)
  })
})

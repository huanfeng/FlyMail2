import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { LoginPage } from '@/pages/Login'

// 这里验的是表单的可访问性契约与消息通道，文案由 locales 的键测试兜住
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

const navigate = vi.fn()
// 登录页现在还要读 ?next=（通知链接把来路带过来），mock 里得有它
let searchParams = new URLSearchParams()
vi.mock('react-router', () => ({
  useNavigate: () => navigate,
  useSearchParams: () => [searchParams, vi.fn()] as const,
}))

const login = vi.fn()
vi.mock('@/lib/api', () => ({
  login: (...a: unknown[]) => login(...a),
  default: {},
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('LoginPage', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount() {
    await act(async () => {
      root.render(<LoginPage />)
    })
  }

  async function flush() {
    for (let i = 0; i < 3; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  const q = <T extends Element>(sel: string) => container.querySelector<T>(sel)

  beforeEach(() => {
    localStorage.clear()
    searchParams = new URLSearchParams()
    navigate.mockClear()
    login.mockReset()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('两个输入框都有真正关联上的 label', async () => {
    // 原先的 shadcn 版本靠写死的 id="username"/"password" 关联，改成 useId 之后
    // 一旦拼接写错就会变成指向不存在的 id——和没有 label 一样，但看起来是有的。
    await mount()
    for (const name of ['username', 'password']) {
      const label = [...container.querySelectorAll('label')].find((l) => l.textContent === `login.${name}`)
      expect(label, `缺少 ${name} 的 label`).toBeTruthy()
      const id = label?.getAttribute('for')
      expect(id).toBeTruthy()
      // useId 生成的 id 含冒号，不能直接拼进 #id 选择器（jsdom 也没有 CSS.escape）
      expect(container.querySelector(`[id="${id}"]`), `label for=${id} 指向不存在的元素`).toBeTruthy()
    }
  })

  it('密码可见性按钮说得出自己是什么，并且可聚焦', async () => {
    // 原先是 tabIndex={-1} 的裸 button，读屏只报「按钮」，键盘也够不着
    await mount()
    const eye = q<HTMLButtonElement>('.login-eye')
    expect(eye).toBeTruthy()
    expect(eye?.getAttribute('aria-label')).toBe('login.togglePassword')
    expect(eye?.getAttribute('tabindex')).toBeNull()
  })

  it('点一下明文显示，aria-pressed 跟着变', async () => {
    await mount()
    const eye = q<HTMLButtonElement>('.login-eye')
    const pass = q<HTMLInputElement>('input[autocomplete="current-password"]')
    expect(pass?.type).toBe('password')
    expect(eye?.getAttribute('aria-pressed')).toBe('false')

    await act(async () => eye?.click())
    expect(q<HTMLInputElement>('input[autocomplete="current-password"]')?.type).toBe('text')
    expect(q('.login-eye')?.getAttribute('aria-pressed')).toBe('true')
  })

  it('消息区常驻在 DOM 里，没有消息时也在', async () => {
    // live region 与内容同时插入 DOM 时读屏不播报（这是前几轮踩过的）。
    // 所以这条断言的是「空的时候它也在」，而不是「有错误时它出现」。
    await mount()
    const msg = q('.login-msg')
    expect(msg).toBeTruthy()
    expect(msg?.getAttribute('role')).toBe('status')
    expect(msg?.getAttribute('aria-live')).toBe('polite')
    expect(msg?.textContent).toBe('')
  })

  it('登录失败时错误落进那个常驻的消息区', async () => {
    login.mockRejectedValue({ response: { status: 401 } })
    await mount()
    const pass = q<HTMLInputElement>('input[autocomplete="current-password"]')
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(pass, 'x')
      pass?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      q<HTMLFormElement>('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })
    await flush()

    const msg = q('.login-msg')
    expect(msg?.textContent).toBe('login.errInvalid')
    expect(msg?.className).toContain('danger')
  })

  it('被限流时提交按钮直接禁用', async () => {
    // 限流期内连请求都不发：多打一次只会把后端的失败计数推得更远
    // parseRetryAfter 走的是 axios.isAxiosError，普通对象会被它直接判否——
    // 那样只会退回通用文案，这条就测不到倒计时那一支了
    login.mockRejectedValue({
      isAxiosError: true,
      response: { status: 429, headers: { 'retry-after': '90' }, data: {} },
    })
    await mount()
    const pass = q<HTMLInputElement>('input[autocomplete="current-password"]')
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(pass, 'x')
      pass?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      q<HTMLFormElement>('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })
    await flush()

    expect(q<HTMLButtonElement>('.login-submit')?.disabled).toBe(true)
    expect(q('.login-msg')?.textContent).toContain('login.errRateLimited')
  })

  /**
   * 登录成功后回到用户本来要去的地方。
   *
   * ⚠ 这条守的是通知链接在**另一台设备**上的主路径：飞书里看到提醒 → 手机浏览器
   * 点开 → 没有登录态 → 跳登录页。不把来路带过来并用上，URL 里的
   * account/folder/message 就在这一跳里没了，登录完落在默认收件箱——
   * 用户点了链接却没打开那封邮件，而且没有任何提示。
   */
  it('登录成功后回到 next 指向的页面', async () => {
    searchParams = new URLSearchParams('next=%2F%3Faccount%3D1%26folder%3D7%26message%3D42')
    login.mockResolvedValue({})
    await mount()
    await submitWith('x')

    expect(navigate).toHaveBeenCalledWith('/?account=1&folder=7&message=42', { replace: true })
  })

  it('没有来路时回首页', async () => {
    login.mockResolvedValue({})
    await mount()
    await submitWith('x')

    expect(navigate).toHaveBeenCalledWith('/', { replace: true })
  })

  /**
   * ⚠ 安全判据：站外地址必须被丢掉。
   *
   * 放行的话，`/login?next=https://evil.example.com` 就是一个挂在我们自己域名上的
   * 开放重定向——用户看到的是从可信站点跳过去的，钓鱼页再仿个登录框即可。
   */
  it('来路指向站外时丢掉，回首页', async () => {
    searchParams = new URLSearchParams('next=https%3A%2F%2Fevil.example.com%2F')
    login.mockResolvedValue({})
    await mount()
    await submitWith('x')

    expect(navigate).toHaveBeenCalledWith('/', { replace: true })
  })

  async function submitWith(password: string) {
    const pass = q<HTMLInputElement>('input[autocomplete="current-password"]')
    const user = q<HTMLInputElement>('input[autocomplete="username"]')
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
      setter?.call(user, 'admin')
      user?.dispatchEvent(new Event('input', { bubbles: true }))
      setter?.call(pass, password)
      pass?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      q<HTMLFormElement>('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })
    await flush()
  }

})

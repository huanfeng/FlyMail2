import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { ToastProvider } from '@/components/ui/Toast'
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

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function account(id: number, email: string): Account {
  return {
    id,
    name: `acct${id}`,
    email,
    auth_type: 'password',
    imap_host: `imap${id}.example.com`,
    imap_port: 993,
    imap_security: 'ssl',
    smtp_host: `smtp${id}.example.com`,
    smtp_port: 465,
    smtp_security: 'ssl',
    status: 'ok',
    enabled: true,
  }
}

/**
 * 账户对话框在关闭后**保留**填到一半的内容。
 *
 * ── 缘起 ─────────────────────────────────────────────────────────────────
 *
 * 原先这里是 `if (open) setForm(...)`——每次打开都重置。而 radix Dialog 默认
 * 点遮罩就关闭，于是「填到一半手滑点了对话框外面」= 全部白填。
 * 添加账户要填邮箱、密码、两组服务器地址端口，重填代价很高，
 * 而触发它只需要一次误点。
 *
 * ── 三条必须同时成立 ─────────────────────────────────────────────────────
 *
 * 1. 同一个目标重新打开 → 内容还在（用户报的那一条）
 * 2. 换成别的账户 → 必须重新初始化，**绝不能**把上一个账户的草稿串过去
 *    （那会导致用户以为在编辑 A，实际把 B 的服务器地址存进了 A）
 * 3. 保存成功后 → 草稿丢弃，下次"添加账户"是空表单
 *
 * 只钉第 1 条是危险的：最省事的实现（"永远不重置"）能让第 1 条通过，
 * 而它恰恰会造成第 2 条那种串号事故。
 */
describe('账户对话框的草稿保留', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  /**
   * 按顺序取表单里的输入框（0=名称 1=邮箱 …），避免依赖 i18n 文案。
   *
   * ⚠ 查 document 而不是 container：radix Dialog 走 Portal，内容挂在
   * document.body 上、根本不在挂载容器里。查 container 会拿到空数组，
   * 后续断言变成"对 undefined 取值"——错误信息与真实原因毫无关系。
   */
  const inputs = () => [...document.querySelectorAll<HTMLInputElement>('[role="dialog"] input')]
  const buttons = () => [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')]

  let onOpenChange: ReturnType<typeof vi.fn<(open: boolean) => void>>

  async function render(open: boolean, acct: Account | null) {
    await act(async () => {
      root.render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <AccountDialog open={open} account={acct} onOpenChange={onOpenChange} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
  }

  async function type(el: HTMLInputElement, value: string) {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(el, value)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  beforeEach(() => {
    onOpenChange = vi.fn()
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, [])
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

  it('前提：能取到表单输入框', async () => {
    await render(true, null)
    expect(inputs().length, '一个输入框都没渲染出来，下面的断言全部无意义').toBeGreaterThan(2)
  })

  it('关闭再打开，填到一半的内容还在', async () => {
    await render(true, null)
    const first = inputs()[0]
    await type(first, '我填了一半')

    // 点对话框外面 = radix 触发 onOpenChange(false)，父组件把 open 置假
    await render(false, null)
    await render(true, null)

    expect(
      inputs()[0].value,
      '内容被清空了——一次误点就要重填邮箱、密码和两组服务器地址',
    ).toBe('我填了一半')
  })

  it('换成另一个账户时必须重新初始化，不能串号', async () => {
    // 这条比上一条更重要：串过去的话，用户以为在编辑 A，
    // 实际把 B 的服务器地址存进了 A。
    await render(true, account(1, 'a@example.com'))
    const before = inputs().map((el) => el.value)
    expect(before.some((v) => v.includes('a@example.com'))).toBe(true)

    await render(false, account(1, 'a@example.com'))
    await render(true, account(2, 'b@example.com'))

    const after = inputs().map((el) => el.value)
    expect(after.some((v) => v.includes('b@example.com')), '没有加载新账户的数据').toBe(true)
    expect(
      after.some((v) => v.includes('a@example.com')),
      '上一个账户的数据串到了这一个上',
    ).toBe(false)
  })

  it('从编辑切到新建时也要重新初始化', async () => {
    await render(true, account(1, 'a@example.com'))
    await render(false, account(1, 'a@example.com'))
    await render(true, null) // 「添加账户」

    const vals = inputs().map((el) => el.value)
    expect(
      vals.some((v) => v.includes('a@example.com')),
      '点"添加账户"却看到了上一个被编辑账户的数据',
    ).toBe(false)
  })

  it('一个字没填就关闭再打开，不显示草稿提示', async () => {
    // 用户反馈：只是点开看了一眼、没填、关掉、再打开，也弹「已恢复上次未保存的内容」。
    // 空表单重开没有任何"草稿"可言，这条提示只会让人莫名其妙。
    await render(true, null)
    await render(false, null)
    await render(true, null)
    expect(document.querySelector('.acct-draft-note'), '没填任何东西却弹了草稿提示').toBeNull()
  })

  it('填过内容再打开才显示草稿提示，清空后提示消失', async () => {
    await render(true, null)
    await type(inputs()[0], '填了一半')
    await render(false, null)
    await render(true, null)
    const note = document.querySelector('.acct-draft-note')
    expect(note, '有草稿却没有提示，用户会以为是系统记住了账号').not.toBeNull()

    await act(async () => note!.querySelector('button')!.click())
    expect(inputs()[0].value).toBe('')
    expect(document.querySelector('.acct-draft-note'), '清空后提示条还挂着').toBeNull()
  })

  it('提供显式的清空入口', async () => {
    // 内容既然会保留，就必须有办法回到空表单——否则用户无路可走。
    await render(true, null)
    const first = inputs()[0]
    await type(first, '不要了')

    const reset = buttons().find((b) => b.textContent?.includes('account.reset'))
    expect(reset, '没有清空按钮').toBeTruthy()

    await act(async () => reset!.click())
    expect(inputs()[0].value, '点了清空却没清掉').toBe('')
  })
})

/**
 * 「填到一半点了框外」的防误触。
 *
 * 判据是**关闭会丢掉重建代价高的输入**，不是"这是个对话框"。所以三条要同时成立：
 *
 *   空表单点外面 → 照常关闭（没内容还加阻力，是纯粹的烦人）
 *   有内容点外面 → 不关（这是要修的那件事）
 *   有内容按 Esc → 照常关闭（ARIA 规定 Esc 关闭对话框，那是键盘用户
 *                  唯一的快速出口；拦掉等于把人困在框里）
 *
 * 只钉中间那条最危险：一个"永远不许点外面关"的实现能让它通过，
 * 而那正是比原问题更糟的结果。
 */
describe('账户对话框的防误触', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let onOpenChange: ReturnType<typeof vi.fn<(open: boolean) => void>>

  const inputs = () => [...document.querySelectorAll<HTMLInputElement>('[role="dialog"] input')]

  async function mount() {
    await act(async () => {
      root.render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <AccountDialog open account={null} onOpenChange={onOpenChange} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
  }

  async function type(el: HTMLInputElement, value: string) {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(el, value)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  async function pointerDownOutside() {
    await act(async () => {
      const ev = new MouseEvent('pointerdown', { bubbles: true, cancelable: true })
      Object.defineProperty(ev, 'pointerType', { value: 'mouse' })
      document.body.dispatchEvent(ev)
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
  }

  async function pressEscape() {
    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
  }

  /** onOpenChange(false) 被调用过 = 对话框要关了 */
  const askedToClose = () => onOpenChange.mock.calls.some((c) => c[0] === false)

  beforeEach(() => {
    onOpenChange = vi.fn()
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, [])
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

  it('空表单点外面照常关闭', async () => {
    await mount()
    await pointerDownOutside()
    expect(askedToClose(), '什么都没填却关不掉——那是给用户添堵').toBe(true)
  })

  it('填了内容之后点外面不关闭', async () => {
    await mount()
    await type(inputs()[0], '填了一半')
    await pointerDownOutside()
    expect(askedToClose(), '填到一半误点框外，对话框还是消失了').toBe(false)
  })

  it('填了内容之后按 Esc 仍然关闭', async () => {
    // ARIA 的对话框模式规定 Esc 关闭对话框。拦掉它，键盘用户就只能
    // Tab 到关闭按钮——比"点外面就没了"更糟。
    await mount()
    await type(inputs()[0], '填了一半')
    await pressEscape()
    expect(askedToClose(), 'Esc 也被拦了，键盘用户会被困在对话框里').toBe(true)
  })
})

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { NotificationsPage } from '@/components/notifications/NotificationsPage'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh' } }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 通知筛选器的键盘与读屏语义。
 *
 * 六个筛选器是一组**互斥**选项，语义上是 tablist。原先它们是六个裸 button：
 * 读屏既不报「第 2 项，共 6 项」也不报哪个是选中的，而且 Tab 要按六次才走得完
 * 这一组——而 ARIA 对这种控件的惯例是 roving tabindex：整组只占 Tab 序列里的
 * 一格，组内用方向键移动。
 */
describe('NotificationsPage 的筛选器语义', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  const tabs = () => [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')]

  async function press(el: HTMLElement, key: string) {
    await act(async () => {
      el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
    })
    await flush(1)
  }

  beforeEach(async () => {
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, { notifications: [], unread_count: 0 })

    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <NotificationsPage onClose={vi.fn()} />
        </QueryClientProvider>,
      )
    })
    await flush()
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('是一组 tab，而不是六个孤立按钮', () => {
    const list = container.querySelector('[role="tablist"]')
    expect(list, '没有 tablist：读屏不会报「第几项，共几项」').not.toBeNull()
    expect(list?.getAttribute('aria-label'), 'tablist 没有名字').toBeTruthy()
    expect(tabs().length).toBe(6)
  })

  it('选中项由 aria-selected 表达，而不只是一个 class', () => {
    // 原先选中态只有 .active 这个 class——纯视觉，读屏完全看不到。
    const selected = tabs().filter((b) => b.getAttribute('aria-selected') === 'true')
    expect(selected.length, '选中的 tab 不是恰好一个').toBe(1)
    expect(selected[0].className).toContain('active')
  })

  it('roving tabindex：整组只占 Tab 序列里的一格', () => {
    // 六个都是 tabindex=0 的话，键盘用户想越过这一组要按六次 Tab。
    const inSequence = tabs().filter((b) => b.tabIndex === 0)
    expect(inSequence.length).toBe(1)
    expect(inSequence[0].getAttribute('aria-selected')).toBe('true')
  })

  it('右键移到下一个，左键移回来，两端环绕', async () => {
    const first = tabs()[0]
    await press(first, 'ArrowRight')
    expect(tabs()[1].getAttribute('aria-selected'), '右键没有切到下一个').toBe('true')
    expect(tabs()[1].tabIndex, '焦点没跟着走，下一次方向键会从原处算起').toBe(0)

    await press(tabs()[1], 'ArrowLeft')
    expect(tabs()[0].getAttribute('aria-selected')).toBe('true')

    // 环绕：第一个再按左键回到最后一个（与侧栏账户列表一致）
    await press(tabs()[0], 'ArrowLeft')
    expect(tabs()[5].getAttribute('aria-selected'), '两端没有环绕').toBe('true')
  })

  it('tab 与它的面板互相连上', () => {
    // 只做一半的 tablist 语义：读屏报完「标签，已选中，第 2 项共 6 项」之后，
    // 用户按惯例去找面板，找不到。
    const panel = container.querySelector('[role="tabpanel"]')
    expect(panel, '没有 tabpanel').not.toBeNull()
    const selected = tabs().find((b) => b.getAttribute('aria-selected') === 'true')!
    expect(selected.getAttribute('aria-controls')).toBe(panel!.id)
    expect(panel!.getAttribute('aria-labelledby')).toBe(selected.id)
  })

  it('Home / End 跳到两端', async () => {
    await press(tabs()[0], 'End')
    expect(tabs()[5].getAttribute('aria-selected')).toBe('true')
    await press(tabs()[5], 'Home')
    expect(tabs()[0].getAttribute('aria-selected')).toBe('true')
  })

  it('带修饰键的方向键让给系统', async () => {
    // Ctrl+← / ⌥+→ 是很多人的词间移动习惯。吞掉它就成了
    //「想移动光标，结果换了筛选器」。
    const before = tabs().findIndex((b) => b.getAttribute('aria-selected') === 'true')
    for (const init of [{ ctrlKey: true }, { metaKey: true }, { altKey: true }]) {
      await act(async () => {
        tabs()[before].dispatchEvent(
          new KeyboardEvent('keydown', {
            key: 'ArrowRight',
            bubbles: true,
            cancelable: true,
            ...init,
          }),
        )
      })
      await flush(1)
    }
    expect(tabs().findIndex((b) => b.getAttribute('aria-selected') === 'true')).toBe(before)
  })

  it('方向键之外的键不改变选中项', async () => {
    // 这段 onKeyDown 只该对左右方向键有反应。
    //
    // ⚠ 这里不能断言 defaultPrevented：浮层自己的 focus trap 也装在 document 上，
    //   而 jsdom 不排版（元素尺寸恒为 0）导致它的 focusables() 恒空，于是每一次
    //   Tab 都走「没有可聚焦元素就吞掉」那一支。两者混在一起分不清是谁干的，
    //   要看的是这段代码自己负责的那件事——选中项有没有动。
    const before = tabs().findIndex((b) => b.getAttribute('aria-selected') === 'true')
    for (const key of ['Tab', 'Enter', 'ArrowDown', 'a']) {
      await press(tabs()[before], key)
    }
    const after = tabs().findIndex((b) => b.getAttribute('aria-selected') === 'true')
    expect(after).toBe(before)
  })
})

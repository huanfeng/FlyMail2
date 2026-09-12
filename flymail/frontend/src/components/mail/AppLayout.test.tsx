import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { AppLayout } from '@/components/mail/AppLayout'
import {
  LAYOUT_EVENT,
  LAYOUT_DEFAULTS,
  LAYOUT_LIMITS,
  LAYOUT_LS_KEY,
  type LayoutWidths,
} from '@/lib/layout-prefs'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

// jsdom 没有 matchMedia，AppLayout 用它判断窄屏
window.matchMedia = ((q: string) => ({
  matches: false,
  media: q,
  addEventListener: () => {},
  removeEventListener: () => {},
})) as unknown as typeof window.matchMedia

describe('AppLayout 的宽度持久化', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount() {
    await act(async () => {
      root.render(
        <AppLayout
          sidebar={<div />}
          list={<div />}
          reader={<div />}
          mobilePane="list"
          drawerOpen={false}
          onDrawerOpenChange={() => {}}
          onMobileBack={() => {}}
          layoutMode="three"
        />,
      )
    })
  }

  function broadcast(w: LayoutWidths) {
    window.dispatchEvent(new CustomEvent<LayoutWidths>(LAYOUT_EVENT, { detail: w }))
  }

  beforeEach(() => {
    localStorage.clear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.useFakeTimers()
  })

  afterEach(async () => {
    vi.useRealTimers()
    await act(async () => root.unmount())
    container.remove()
  })

  it('收到与当前宽度相同的广播时不再落盘——否则就是自激循环', async () => {
    await mount()
    // 挂载后的首次防抖落盘
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    const setItem = vi.spyOn(Storage.prototype, 'setItem')

    // saveLayoutWidths 会把自己写的值广播回来，detail 每次都是新对象。
    // 不按值比较就会「落盘 → 广播 → 新引用 → 再落盘」，一轮轮转下去。
    for (let i = 0; i < 5; i++) {
      await act(async () => broadcast({ ...LAYOUT_DEFAULTS }))
      await act(async () => {
        vi.advanceTimersByTime(300)
      })
    }

    expect(setItem).not.toHaveBeenCalled()
    setItem.mockRestore()
  })

  it('宽度确实变了才落盘，且走防抖', async () => {
    await mount()
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    await act(async () => broadcast({ ...LAYOUT_DEFAULTS, sidebar: 300 }))

    // 防抖窗口内先不写盘：拖拽每帧都写 localStorage 是同步操作，会拖慢拖拽
    expect(setItem).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(300)
    })
    expect(setItem).toHaveBeenCalled()
    expect(JSON.parse(localStorage.getItem(LAYOUT_LS_KEY)!).sidebar).toBe(300)
    setItem.mockRestore()
  })
})

describe('AppLayout 手柄报给读屏的上限', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    localStorage.clear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('三栏形态下报的是按窗口宽度收紧后的上限，不是静态常量', async () => {
    await act(async () => {
      root.render(
        <AppLayout
          sidebar={<div />}
          list={<div />}
          reader={<div />}
          mobilePane="list"
          drawerOpen={false}
          onDrawerOpenChange={() => {}}
          onMobileBack={() => {}}
          layoutMode="three"
        />,
      )
    })

    const handle = container.querySelector('[role="separator"]')!
    // 真正的上限 = min(静态上限, 窗口宽 - 另一栏 - 阅读区保底)，
    // Home/End 走的就是它。报静态上限的话，读屏被告知「最大 420」而
    // 按 End 停在别处，aria-valuenow 永远够不到 aria-valuemax。
    const expected = Math.min(
      LAYOUT_LIMITS.sidebar.max,
      window.innerWidth - LAYOUT_DEFAULTS.list - 300,
    )
    expect(handle.getAttribute('aria-valuemax')).toBe(String(expected))
    // 方向键怎么用，得有地方说
    expect(handle.getAttribute('aria-describedby')).toBeTruthy()
  })
})

describe('AppLayout 与 MailList 的 CSS 变量分工', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount() {
    await act(async () => {
      root.render(
        <AppLayout
          sidebar={<div />}
          list={<div />}
          reader={<div />}
          mobilePane="list"
          drawerOpen={false}
          onDrawerOpenChange={() => {}}
          onMobileBack={() => {}}
          layoutMode="three"
        />,
      )
    })
  }

  function cssVar(name: string) {
    return document.documentElement.style.getPropertyValue(name)
  }

  beforeEach(() => {
    localStorage.clear()
    document.documentElement.style.removeProperty('--sender-col-w')
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('挂载时按存盘值写一次 --sender-col-w，避免 MailList 挂载前的首帧闪烁', async () => {
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify({ ...LAYOUT_DEFAULTS, senderCol: 240 }))
    await mount()
    expect(cssVar('--sender-col-w')).toBe('240px')
  })

  it('之后不再碰 --sender-col-w：那一项归 MailList 管', async () => {
    await mount()

    // 哨兵值代表「MailList 刚把新宽度写进去，而它的防抖还挂着」
    document.documentElement.style.setProperty('--sender-col-w', '999px')

    // 此刻用户去拖侧栏：本组件的 effect 每帧触发。它若还写 senderCol，
    // 写的就是自己手上那份尚未更新的旧值——列宽会在侧栏拖拽过程中一直弹回去。
    await act(async () => {
      window.dispatchEvent(
        new CustomEvent<LayoutWidths>(LAYOUT_EVENT, {
          detail: { ...LAYOUT_DEFAULTS, sidebar: 300 },
        }),
      )
    })

    expect(cssVar('--sender-col-w')).toBe('999px')
    // 自己管的那几项照常更新
    expect(cssVar('--sidebar-w')).toBe('300px')
  })
})

describe('AppLayout 的关窗兜底', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount() {
    await act(async () => {
      root.render(
        <AppLayout
          sidebar={<div />}
          list={<div />}
          reader={<div />}
          mobilePane="list"
          drawerOpen={false}
          onDrawerOpenChange={() => {}}
          onMobileBack={() => {}}
          layoutMode="three"
        />,
      )
    })
  }

  function hidePage() {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
    window.dispatchEvent(new Event('pagehide'))
  }

  beforeEach(() => {
    localStorage.clear()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    vi.useFakeTimers()
  })

  afterEach(async () => {
    vi.useRealTimers()
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    await act(async () => root.unmount())
    container.remove()
  })

  it('防抖没到期就隐藏页面时，改动仍被写下', async () => {
    await mount()
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent<LayoutWidths>(LAYOUT_EVENT, {
          detail: { ...LAYOUT_DEFAULTS, sidebar: 333 },
        }),
      )
    })
    // 200ms 还没到，此刻关窗——Wails 关窗没有第二次机会
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    await act(async () => hidePage())

    expect(setItem).toHaveBeenCalled()
    expect(JSON.parse(localStorage.getItem(LAYOUT_LS_KEY)!).sidebar).toBe(333)
    setItem.mockRestore()
  })

  it('没有待写的改动时，切标签页不白写盘', async () => {
    // visibilitychange 每次切标签页都触发。flush 若无条件写，
    // LAYOUT_EVENT 就成了「切标签页也会响」的事件——今天只有值比较的监听器在听，
    // 哪天有谁订阅它做实事就会收到一堆莫名其妙的唤醒。
    await mount()
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    await act(async () => hidePage())
    await act(async () => hidePage())

    expect(setItem).not.toHaveBeenCalled()
    setItem.mockRestore()
  })
})

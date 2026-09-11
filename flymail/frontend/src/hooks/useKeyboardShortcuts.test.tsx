import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { useKeyboardShortcuts, FOCUS_SEARCH_EVENT } from '@/hooks/useKeyboardShortcuts'

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/** 所有回调的 spy 集合，用例按需断言。 */
function makeHandlers() {
  return {
    onCompose: vi.fn(),
    onReply: vi.fn(),
    onReplyAll: vi.fn(),
    onForward: vi.fn(),
    onNavigate: vi.fn(),
    onArchive: vi.fn(),
    onDelete: vi.fn(),
    onToggleStar: vi.fn(),
    onMarkUnread: vi.fn(),
    onBack: vi.fn(),
    onGo: vi.fn(),
    onToggleSelectCurrent: vi.fn(),
    onExtendSelection: vi.fn(),
    onCloseCompose: vi.fn(),
    onEscape: vi.fn(),
    onToggleHelp: vi.fn(),
    onCloseHelp: vi.fn(),
  }
}

type Handlers = ReturnType<typeof makeHandlers>

describe('useKeyboardShortcuts', () => {
  let container: HTMLDivElement
  let root: Root
  let h: Handlers

  function Probe({ composeOpen = false, helpOpen = false }: { composeOpen?: boolean; helpOpen?: boolean }) {
    useKeyboardShortcuts({
      ...h,
      navIds: [1, 2, 3],
      activeNavId: 2,
      composeOpen,
      helpOpen,
    })
    return <input data-testid="field" />
  }

  async function mount(props: { composeOpen?: boolean; helpOpen?: boolean } = {}) {
    await act(async () => root.render(<Probe {...props} />))
  }

  /** 向 window 派发一次按键；target 默认是 document.body（非输入型元素）。 */
  async function press(key: string, init: KeyboardEventInit = {}, target?: Element) {
    await act(async () => {
      const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
      ;(target ?? document.body).dispatchEvent(ev)
    })
  }

  beforeEach(() => {
    h = makeHandlers()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    vi.useRealTimers()
  })

  it('e 归档，# 与 Delete 都能删除', async () => {
    await mount()
    await press('e')
    expect(h.onArchive).toHaveBeenCalledTimes(1)

    await press('#')
    await press('Delete')
    expect(h.onDelete).toHaveBeenCalledTimes(2)
  })

  it('s 切换星标，u 回到列表', async () => {
    await mount()
    await press('s')
    expect(h.onToggleStar).toHaveBeenCalledTimes(1)
    await press('u')
    expect(h.onBack).toHaveBeenCalledTimes(1)
  })

  it('Shift+U 标为未读，且不触发「回到列表」', async () => {
    await mount()
    await press('U', { shiftKey: true })
    expect(h.onMarkUnread).toHaveBeenCalledTimes(1)
    // 同一个键靠修饰键区分，走岔了就会一边标未读一边跳回列表
    expect(h.onBack).not.toHaveBeenCalled()
  })

  it('a 全部回复，f 转发', async () => {
    await mount()
    await press('a')
    await press('f')
    expect(h.onReplyAll).toHaveBeenCalledTimes(1)
    expect(h.onForward).toHaveBeenCalledTimes(1)
  })

  it('x 选中当前行，Shift+J / Shift+K 扩展选择', async () => {
    await mount()
    await press('x')
    expect(h.onToggleSelectCurrent).toHaveBeenCalledTimes(1)

    await press('J', { shiftKey: true })
    await press('K', { shiftKey: true })
    expect(h.onExtendSelection.mock.calls).toEqual([[1], [-1]])
    // 扩展选择不应顺带走 j/k 的普通导航
    expect(h.onNavigate).not.toHaveBeenCalled()
  })

  it('g 之后接 i/s/t/d 跳转', async () => {
    await mount()
    await press('g')
    await press('i')
    expect(h.onGo).toHaveBeenCalledWith('inbox')

    await press('g')
    await press('d')
    expect(h.onGo).toHaveBeenCalledWith('drafts')
  })

  it('单独按 g 不触发任何动作', async () => {
    await mount()
    await press('g')
    expect(h.onGo).not.toHaveBeenCalled()
  })

  it('g 之后接无效键时，那个键按普通单键处理', async () => {
    await mount()
    await press('g')
    await press('e') // 不是合法跳转目标 → 落回「归档」
    expect(h.onGo).not.toHaveBeenCalled()
    expect(h.onArchive).toHaveBeenCalledTimes(1)
  })

  it('g 超时后不再构成组合键', async () => {
    vi.useFakeTimers()
    await mount()
    await press('g')
    await act(async () => {
      vi.advanceTimersByTime(2000)
    })
    await press('i')
    // 一个误按的 g 不该让之后随便哪次按 i 都变成跳转
    expect(h.onGo).not.toHaveBeenCalled()
  })

  it('j / k 走列表导航', async () => {
    await mount()
    await press('j')
    expect(h.onNavigate).toHaveBeenCalledWith(3)
    await press('k')
    expect(h.onNavigate).toHaveBeenCalledWith(1)
  })

  it('焦点在输入框内时不触发单键', async () => {
    await mount()
    const field = container.querySelector('input')!
    await press('e', {}, field)
    await press('x', {}, field)
    expect(h.onArchive).not.toHaveBeenCalled()
    expect(h.onToggleSelectCurrent).not.toHaveBeenCalled()
  })

  it('撰写窗口打开时屏蔽单键，Esc 仍然关闭它', async () => {
    await mount({ composeOpen: true })
    await press('e')
    expect(h.onArchive).not.toHaveBeenCalled()

    await press('Escape')
    expect(h.onCloseCompose).toHaveBeenCalledTimes(1)
    expect(h.onEscape).not.toHaveBeenCalled()
  })

  it('速查浮层打开时，Esc 优先关闭它', async () => {
    await mount({ helpOpen: true })
    await press('Escape')
    expect(h.onCloseHelp).toHaveBeenCalledTimes(1)
    expect(h.onCloseCompose).not.toHaveBeenCalled()
  })

  it('带 Ctrl/Alt 的组合不触发单键动作', async () => {
    await mount()
    await press('e', { ctrlKey: true })
    await press('s', { altKey: true })
    expect(h.onArchive).not.toHaveBeenCalled()
    expect(h.onToggleStar).not.toHaveBeenCalled()
  })

  it('Ctrl/⌘+K 广播聚焦搜索事件', async () => {
    await mount()
    const spy = vi.fn()
    window.addEventListener(FOCUS_SEARCH_EVENT, spy)
    await press('k', { ctrlKey: true })
    window.removeEventListener(FOCUS_SEARCH_EVENT, spy)
    expect(spy).toHaveBeenCalledTimes(1)
  })

  it('动作回调为 null 时按键不报错', async () => {
    h.onArchive = null as unknown as typeof h.onArchive
    h.onDelete = null as unknown as typeof h.onDelete
    await mount()
    await press('e')
    await press('#')
    expect(true).toBe(true)
  })
})

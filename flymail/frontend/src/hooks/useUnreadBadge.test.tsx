import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { useUnreadBadge } from '@/hooks/useUnreadBadge'
import { setNotifyPrefs, resetNotifyPrefsCache } from '@/lib/notify-prefs'

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

// jsdom 没有 canvas 后端：getContext 返回 null。这正好覆盖「画不出来就只改标题」
// 这条降级路径，标题断言在两种环境下都成立。
describe('useUnreadBadge', () => {
  let container: HTMLDivElement
  let root: Root

  function Harness({ unread }: { unread: number }) {
    useUnreadBadge(unread)
    return null
  }

  async function mount(unread: number) {
    await act(async () => {
      root.render(<Harness unread={unread} />)
    })
  }

  beforeEach(() => {
    localStorage.clear()
    resetNotifyPrefsCache()
    document.title = 'FlyMail'
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('未读数写进标签页标题', async () => {
    await mount(7)
    expect(document.title).toBe('(7) FlyMail')
  })

  it('读完归零时标题回到干净状态', async () => {
    await mount(7)
    await mount(0)
    expect(document.title).toBe('FlyMail')
  })

  it('三位数以上收成 99+，免得把标题挤没了', async () => {
    await mount(1234)
    expect(document.title).toBe('(99+) FlyMail')
  })

  it('关掉角标偏好后标题不再变化', async () => {
    setNotifyPrefs({ titleBadge: false })
    await mount(7)
    expect(document.title).toBe('FlyMail')
  })

  it('canvas 不可用时只降级标题，不抛错', async () => {
    // 隐私模式/无头环境里 getContext 可能返回 null——这条路径不能把整页带崩
    const spy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    await mount(3)
    expect(document.title).toBe('(3) FlyMail')
    spy.mockRestore()
  })
})

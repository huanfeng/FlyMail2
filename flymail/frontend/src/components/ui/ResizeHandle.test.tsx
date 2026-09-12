import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ResizeHandle } from '@/components/ui/ResizeHandle'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) => (o ? `${k}:${JSON.stringify(o)}` : k),
  }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('ResizeHandle', () => {
  let container: HTMLDivElement
  let root: Root

  function handle() {
    return container.querySelector<HTMLElement>('[role="separator"]')!
  }

  function press(key: string, shiftKey = false) {
    handle().dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true }))
  }

  async function mount(props: Partial<React.ComponentProps<typeof ResizeHandle>> = {}) {
    const onDelta = vi.fn()
    const onJump = vi.fn()
    await act(async () => {
      root.render(
        <ResizeHandle
          className="col-resize"
          label="调整侧栏宽度"
          value={248}
          min={180}
          max={420}
          onDelta={onDelta}
          onJump={onJump}
          {...props}
        />,
      )
    })
    return { onDelta, onJump }
  }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('向读屏宣告的可调节性是真的：可聚焦并带上当前值', async () => {
    await mount()
    const el = handle()

    // 此前这里只有 role="separator" + onPointerDown：宣告了「可以调」却调不动，
    // 比不加这个角色更糟
    expect(el.getAttribute('tabindex')).toBe('0')
    expect(el.getAttribute('aria-valuenow')).toBe('248')
    expect(el.getAttribute('aria-valuemin')).toBe('180')
    expect(el.getAttribute('aria-valuemax')).toBe('420')
    expect(el.getAttribute('aria-label')).toBe('调整侧栏宽度')
    // 读屏默认把 valuenow 念成百分比，对宽度没有意义
    expect(el.getAttribute('aria-valuetext')).toContain('layout.widthPx')
  })

  it('方向键按步长调整，Shift 加速', async () => {
    const { onDelta } = await mount()

    await act(async () => press('ArrowRight'))
    expect(onDelta).toHaveBeenLastCalledWith(16)

    await act(async () => press('ArrowLeft'))
    expect(onDelta).toHaveBeenLastCalledWith(-16)

    await act(async () => press('ArrowRight', true))
    expect(onDelta).toHaveBeenLastCalledWith(64)
  })

  it('Home / End 跳到区间端点', async () => {
    const { onJump, onDelta } = await mount()

    await act(async () => press('Home'))
    expect(onJump).toHaveBeenLastCalledWith('min')

    await act(async () => press('End'))
    expect(onJump).toHaveBeenLastCalledWith('max')

    expect(onDelta).not.toHaveBeenCalled()
  })

  it('无关按键不拦截', async () => {
    const { onDelta, onJump } = await mount()

    await act(async () => press('Tab'))
    await act(async () => press('a'))

    expect(onDelta).not.toHaveBeenCalled()
    expect(onJump).not.toHaveBeenCalled()
  })
})

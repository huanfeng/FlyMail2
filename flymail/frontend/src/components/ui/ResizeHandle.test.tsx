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

  /**
   * ── 拖拽只发整数增量，余量留到下次 ────────────────────────────────────────
   *
   * 调用方是 `clamp(prev + dx, …)`——**状态本身就是累加器**。分数缩放的显示器上
   * `ev.clientX` 带小数，dx 原样传出去，宽度就变成 503.000003242，设置里那一行
   * 把它渲染成 `503.000003242px`，撑出横向滚动条（用户报的第 4 条）。
   *
   * 修法有两种，选错一种会换来更糟的毛病：
   *
   *   ✗ 在调用方 `Math.round(prev + dx)`：每次小于 0.5px 的位移都被抹成 0，
   *     余量无处可存，**拖拽在分数缩放下彻底推不动**。
   *   ✓ 在这里只发整数部分、把余数留在 lastX 里：小位移会攒够 1px 再发出去，
   *     拖拽依然跟手，而调用方拿到的永远是整数。
   *
   * 下面两个用例分别钉这两件事，缺一不可。
   */
  describe('拖拽的亚像素处理', () => {
    function pointer(type: string, clientX: number) {
      // jsdom 没有 PointerEvent。MouseEvent 带得动 clientX，再补上 pointerId 即可。
      const ev = new MouseEvent(type, { clientX, bubbles: true }) as MouseEvent & {
        pointerId?: number
      }
      ev.pointerId = 1
      return ev as unknown as PointerEvent
    }

    function startDrag(el: HTMLElement, atX: number) {
      // jsdom 的 Element 上没有这两个方法，组件里直接调用会抛
      const stub = el as HTMLElement & Record<string, unknown>
      stub.setPointerCapture = () => {}
      stub.releasePointerCapture = () => {}
      el.dispatchEvent(pointer('pointerdown', atX))
    }

    it('小数位移不泄漏给调用方', async () => {
      const { onDelta } = await mount()
      const el = handle()

      await act(async () => startDrag(el, 100))
      // 典型的分数缩放轨迹：每帧 1.2px
      for (const x of [101.2, 102.4, 103.6, 104.8, 106.0]) {
        await act(async () => el.dispatchEvent(pointer('pointermove', x)))
      }

      expect(onDelta.mock.calls.length, '一次移动都没发出去').toBeGreaterThan(0)
      for (const [dx] of onDelta.mock.calls) {
        expect(Number.isInteger(dx), `发出了小数增量 ${dx}`).toBe(true)
      }
    })

    it('余量会累加，不会被丢掉', async () => {
      // 这条是上一条的必要补充：全部返回 0 也能让上面那条通过（整数），
      // 但那正是「拖不动」。总位移 6px，发出去的总和必须也是 6。
      const { onDelta } = await mount()
      const el = handle()

      await act(async () => startDrag(el, 100))
      for (const x of [101.2, 102.4, 103.6, 104.8, 106.0]) {
        await act(async () => el.dispatchEvent(pointer('pointermove', x)))
      }

      const total = onDelta.mock.calls.reduce((s: number, call: unknown[]) => s + (call[0] as number), 0)
      expect(total, '亚像素余量被丢掉了，拖拽会比手慢').toBe(6)
    })

    it('反向拖动同样不丢余量', async () => {
      // 负方向用 Math.trunc 而不是 Math.floor：floor(-0.4) = -1 会让向左的
      // 微小抖动被放大成整整 1px，向右却是 0——拖拽会朝一侧漂。
      const { onDelta } = await mount()
      const el = handle()

      await act(async () => startDrag(el, 100))
      for (const x of [98.8, 97.6, 96.4, 95.2, 94.0]) {
        await act(async () => el.dispatchEvent(pointer('pointermove', x)))
      }

      const total = onDelta.mock.calls.reduce((s: number, call: unknown[]) => s + (call[0] as number), 0)
      expect(total).toBe(-6)
      for (const [dx] of onDelta.mock.calls) {
        expect(Number.isInteger(dx)).toBe(true)
      }
    })
  })
})

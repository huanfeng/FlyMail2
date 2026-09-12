import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ConfirmProvider, useConfirm } from '@/components/ui/Confirm'
import { useFocusTrap } from '@/hooks/useFocusTrap'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 手写浮层的焦点约束，与它和 radix 浮层的相处。
 *
 * 关键处境：设置面板（手写，trap 装在 document 上）里弹出一个 radix 浮层
 * （账户 / 规则 / 渠道对话框、删除确认框）。radix 走 Portal 挂在 body 末尾，
 * 于是 trap 的判据「焦点不在我的 root 里就拉回来」对它**恒成立**——
 * 每一次 Tab 焦点都被拽回外层浮层，那个对话框里的按钮键盘完全够不着。
 *
 * ⚙ 两处 jsdom 的坑，绕不开：
 *
 * 1. jsdom 不排版，`offsetParent` 与 `getBoundingClientRect()` 全是 0，于是
 *    `focusables()` 恒为空、trap 每次都走「没有可聚焦元素就吞掉这次 Tab」那一支。
 *    照这样测，修不修都是同一个结果，用例毫无区分度。所以下面把 rect 打了桩。
 * 2. 断言不能看 `defaultPrevented`：radix 自己的 FocusScope 在环绕时也会
 *    preventDefault，两者混在一起分不清是谁干的（实测 Shift+Tab 就是被 radix
 *    正当地拦下的）。要看的是用户能感知的那件事——**焦点有没有被抢走**。
 */
describe('useFocusTrap', () => {
  let container: HTMLDivElement
  let root: Root
  const realRect = HTMLElement.prototype.getBoundingClientRect

  function Panel() {
    const ref = useFocusTrap<HTMLDivElement>(true)
    const confirm = useConfirm()
    return (
      <div ref={ref} role="dialog" aria-modal="true">
        <button type="button" id="a">a</button>
        <button type="button" id="ask" onClick={() => void confirm({ title: 'x' })}>
          ask
        </button>
        <button type="button" id="c">c</button>
      </div>
    )
  }

  async function pressTab(shiftKey = false) {
    await act(async () => {
      ;(document.activeElement ?? document.body).dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true, shiftKey }),
      )
    })
  }

  const panel = () => container.querySelector('[role="dialog"]')!
  const confirmBox = () => document.querySelector('.confirm-dialog')

  beforeEach(async () => {
    HTMLElement.prototype.getBoundingClientRect = function () {
      return { width: 10, height: 10, top: 0, left: 0, right: 10, bottom: 10, x: 0, y: 0, toJSON: () => ({}) } as DOMRect
    }
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    await act(async () => root.render(<ConfirmProvider><Panel /></ConfirmProvider>))
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    HTMLElement.prototype.getBoundingClientRect = realRect
  })

  it('没有别的浮层时把焦点关在自己里面', async () => {
    container.querySelector<HTMLButtonElement>('#c')!.focus()
    await pressTab()
    expect(document.activeElement?.id, '最后一个之后没有绕回第一个').toBe('a')

    // 焦点跑到浮层外面时拉回来
    document.body.focus()
    await pressTab()
    expect(panel().contains(document.activeElement), '焦点在浮层外时没有被拉回来').toBe(true)
  })

  it('radix 浮层开着时整个让位，焦点留在那个浮层里', async () => {
    // 回退掉 useFocusTrap 里那行 modalLayerOpen() 判断，这条会失败：焦点被拽回
    // 外层浮层的第一个按钮上——用户表现为「确认框里的确认键 Tab 不过去」。
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
    expect(confirmBox(), '前提：确认框已弹出').not.toBeNull()
    expect(confirmBox()!.contains(document.activeElement), '前提：初始焦点在确认框里').toBe(true)

    await pressTab()
    expect(confirmBox()!.contains(document.activeElement), 'Tab 之后焦点被外层 trap 抢走了').toBe(true)

    await pressTab(true)
    expect(confirmBox()!.contains(document.activeElement), 'Shift+Tab 之后焦点被外层 trap 抢走了').toBe(true)
  })

  it('radix 浮层关闭之后 trap 重新接管', async () => {
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
    const cancel = document.querySelector<HTMLButtonElement>('.confirm-dialog .confirm-actions button')
    await act(async () => cancel?.click())
    expect(confirmBox()).toBeNull()

    document.body.focus()
    await pressTab()
    expect(panel().contains(document.activeElement), '浮层关了之后 trap 没有恢复').toBe(true)
  })
})

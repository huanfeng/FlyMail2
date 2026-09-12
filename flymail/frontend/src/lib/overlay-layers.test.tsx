import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ConfirmProvider, useConfirm } from '@/components/ui/Confirm'
import { modalLayerOpen } from '@/lib/overlay-layers'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 判据本身。
 *
 * 这里**故意**用真的 radix 浮层而不是手搭一个带 data-state 的 div：要验的正是
 * 「这个选择器和 radix 实际渲染出来的 DOM 对不对得上」。手搭的话测的只是我对
 * radix 的记忆，radix 升级换了属性名时它照样绿——而线上表现会是 Esc 又开始
 * 连着关两层，且没有任何报错。
 */
describe('modalLayerOpen', () => {
  let container: HTMLDivElement
  let root: Root

  function Probe() {
    const confirm = useConfirm()
    return (
      <>
        {/* 手写浮层也写 role="dialog"。它不该被自己的判据匹配上，
            否则设置面板一打开就把自己的 Esc 和 Tab 全禁掉了。 */}
        <div role="dialog" aria-modal="true" className="settings-dialog">
          <button type="button" id="ask" onClick={() => void confirm({ title: 'x' })}>
            ask
          </button>
        </div>
      </>
    )
  }

  beforeEach(async () => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    await act(async () => root.render(<ConfirmProvider><Probe /></ConfirmProvider>))
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('只有手写浮层时为假', () => {
    expect(container.querySelector('[role="dialog"]'), '前提：手写浮层确实带 role=dialog').not.toBeNull()
    expect(modalLayerOpen()).toBe(false)
  })

  it('radix 浮层打开时为真，关闭后回到假', async () => {
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
    expect(modalLayerOpen()).toBe(true)

    const cancel = document.querySelector<HTMLButtonElement>('.confirm-dialog .confirm-actions button')
    await act(async () => cancel?.click())
    expect(modalLayerOpen()).toBe(false)
  })
})

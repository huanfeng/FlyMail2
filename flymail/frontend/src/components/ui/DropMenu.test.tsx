import { describe, it, expect } from 'vitest'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { DropMenu } from '@/components/ui/DropMenu'

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * disabled 必须透传到 radix 的 Trigger：radix 的 onPointerDown 只认 Trigger 自己的
 * disabled，子按钮上的 disabled 它不看——发送中的撰写器本应锁住发件人，不透传的话
 * 对禁用按钮仍派发 pointerdown 的浏览器里照样能打开菜单改发件人。
 */
describe('DropMenu disabled', () => {
  it('disabled 透传到触发元素（radix 打 data-disabled）', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    act(() => {
      root.render(
        <DropMenu disabled items={[{ key: 'a', label: 'A' }]} trigger={<button type="button">open</button>} />,
      )
    })
    const btn = host.querySelector('button')!
    expect(btn.hasAttribute('data-disabled')).toBe(true)
    expect(btn.disabled).toBe(true)
    act(() => root.unmount())
    host.remove()
  })
})

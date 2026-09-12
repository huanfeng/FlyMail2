import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ErrorBoundary } from '@/components/ui/ErrorBoundary'

// i18n 换成「原样返回 key」：这里验的是兜底行为，文案由 locales.test.ts 兜住。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/** 受开关控制的爆炸组件：第一次渲染抛错，关掉开关后正常渲染。 */
let boom = true
function Boom() {
  if (boom) throw new Error('渲染炸了')
  return <div id="ok">正常内容</div>
}

describe('ErrorBoundary', () => {
  let container: HTMLDivElement
  let root: Root
  let errSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    boom = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    // React 自己会把被捕获的异常打到 console.error，测试输出里这是噪音
    errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    errSpy.mockRestore()
  })

  it('子组件渲染期抛错时给出回退界面，而不是白屏', async () => {
    await act(async () => {
      root.render(
        <ErrorBoundary>
          <Boom />
        </ErrorBoundary>,
      )
    })

    // 整棵树被卸载 = 一片空白，这正是这个组件要避免的结果
    expect(container.textContent).not.toBe('')
    expect(container.querySelector('[role="alert"]')).not.toBeNull()
    expect(container.textContent).toContain('app.crashTitle')
    // 错误细节要露出来：不同的错对应的下一步动作不同
    expect(container.textContent).toContain('渲染炸了')
  })

  it('重试按钮清掉错误重新挂载，瞬时故障可恢复', async () => {
    await act(async () => {
      root.render(
        <ErrorBoundary>
          <Boom />
        </ErrorBoundary>,
      )
    })
    expect(container.textContent).toContain('app.crashTitle')

    // 故障消失后点重试：不必丢掉整个页面
    boom = false
    const retry = [...container.querySelectorAll('button')].find(
      (b) => b.textContent === 'app.retry',
    )
    expect(retry).toBeDefined()
    await act(async () => {
      retry!.click()
    })

    expect(container.querySelector('#ok')).not.toBeNull()
    expect(container.textContent).not.toContain('app.crashTitle')
  })
})

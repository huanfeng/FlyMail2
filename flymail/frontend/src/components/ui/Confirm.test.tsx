import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ConfirmProvider, useConfirm } from '@/components/ui/Confirm'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 替掉八处 `window.confirm` 的确认框。
 *
 * 与原生 confirm 的关键差别是**异步**：原生的阻塞整个 JS 线程，调用方写
 * `if (!window.confirm(...)) return` 就完了；这里返回 Promise，
 * 于是「Promise 什么时候兑现、兑现成什么」变成了必须验的东西——
 * 一个不兑现的 Promise 会让调用方永远卡在 await 上，界面表现为「点了删除没反应」，
 * 而这正是换掉 confirm 想避免的那种症状。
 */
describe('useConfirm', () => {
  let container: HTMLDivElement
  let root: Root

  /** 渲染一个按钮，点它就发起一次确认；结果推进 results */
  function harness(results: (boolean | string)[]) {
    function Probe() {
      const confirm = useConfirm()
      return (
        <button
          type="button"
          id="ask"
          onClick={() => {
            void confirm({ title: '删掉它？', body: '这一步不可逆', danger: true }).then((ok) =>
              results.push(ok),
            )
          }}
        >
          ask
        </button>
      )
    }
    return (
      <ConfirmProvider>
        <Probe />
      </ConfirmProvider>
    )
  }

  const dialog = () => document.querySelector('.confirm-dialog')
  const actions = () =>
    document.querySelectorAll<HTMLButtonElement>('.confirm-dialog .confirm-actions button')

  async function ask() {
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
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

  it('点确认兑现为 true，点取消兑现为 false', async () => {
    const results: (boolean | string)[] = []
    await act(async () => root.render(harness(results)))

    await ask()
    expect(dialog(), '确认框没弹出来').toBeTruthy()
    await act(async () => actions()[1].click())
    expect(results).toEqual([true])
    expect(dialog(), '点完之后确认框还在').toBeNull()

    await ask()
    await act(async () => actions()[0].click())
    expect(results).toEqual([true, false])
  })

  it('取消排在前面并带初始焦点', async () => {
    // 确认框多半用于不可逆操作，默认落点应当是「什么都不做」。
    // 连按两下回车不该删掉东西。
    //
    // 这条对「去掉 autoFocus」没有区分度（radix 本来就聚焦第一个可聚焦元素），
    // 但对真正要防的那种改动有——把确认键挪到取消前面时它会失败，实测过。
    const results: (boolean | string)[] = []
    await act(async () => root.render(harness(results)))
    await ask()
    const btns = actions()
    expect(btns.length).toBe(2)
    expect(btns[0].textContent).toBe('common.cancel')
    expect(document.activeElement).toBe(btns[0])
  })

  it('Esc 关闭时兑现为 false，而不是把 Promise 悬在那里', async () => {
    // 悬着的 Promise 会让调用方永远停在 await 上——界面表现为「点了删除没反应」，
    // 正是换掉 window.confirm 想避免的那种症状。
    const results: (boolean | string)[] = []
    await act(async () => root.render(harness(results)))
    await ask()
    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(results).toEqual([false])
  })

  it('第二次确认打断第一次时，第一次也兑现（为 false）', async () => {
    // 两个确认框叠在一起用户分不清在答哪一个，所以前一个直接当取消。
    // 但它必须**兑现**——否则第一个调用方永远卡住。
    const results: (boolean | string)[] = []
    await act(async () => root.render(harness(results)))
    await ask()
    await ask()
    expect(results, '被打断的那次没有兑现').toEqual([false])
    await act(async () => actions()[1].click())
    expect(results).toEqual([false, true])
  })

  it('danger 的确认键用 danger 语义色', async () => {
    const results: (boolean | string)[] = []
    await act(async () => root.render(harness(results)))
    await ask()
    expect(actions()[1].className).toContain('danger')
  })

  it('没有 Provider 时直接抛，而不是安静地退回 window.confirm', async () => {
    // 退回原生 confirm 会让「忘了挂 Provider」在界面上完全看不出来
    // （弹的还是个确认框，只是长得不一样）——那正是这次要消灭的东西。
    function Bare() {
      useConfirm()
      return null
    }
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    let caught: unknown = null
    try {
      await act(async () => root.render(<Bare />))
    } catch (e) {
      caught = e
    }
    spy.mockRestore()
    expect(caught).toBeTruthy()
  })
})

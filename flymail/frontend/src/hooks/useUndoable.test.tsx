import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { useUndoable, type Undoable } from '@/hooks/useUndoable'

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('useUndoable', () => {
  let container: HTMLDivElement
  let root: Root
  let api: Undoable

  /** 把 hook 挂起来，并把它的返回值暴露给用例。 */
  function Probe() {
    api = useUndoable()
    return null
  }

  beforeEach(async () => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    await act(async () => root.render(<Probe />))
  })

  afterEach(() => {
    container.remove()
  })

  it('flush 把挂起的操作提交掉', async () => {
    const commit = vi.fn()
    const rollback = vi.fn()
    await act(async () => api.begin({ commit, rollback }))

    expect(commit).not.toHaveBeenCalled() // 挂起期间请求不该发出

    await act(async () => api.flush())
    expect(commit).toHaveBeenCalledTimes(1)
    expect(rollback).not.toHaveBeenCalled()
  })

  it('undo 只回滚，不提交', async () => {
    const commit = vi.fn()
    const rollback = vi.fn()
    await act(async () => api.begin({ commit, rollback }))
    let ok: boolean | undefined
    await act(async () => { ok = api.undo() })

    expect(ok).toBe(true)
    expect(rollback).toHaveBeenCalledTimes(1)
    expect(commit).not.toHaveBeenCalled()
  })

  it('已被强制落地后，undo 返回 false —— 调用方据此告诉用户撤销来不及了', async () => {
    const commit = vi.fn()
    await act(async () => api.begin({ commit, rollback: vi.fn() }))
    await act(async () => api.flush())

    let ok: boolean | undefined
    await act(async () => { ok = api.undo() })

    // 这正是「切文件夹后撤销条还挂在屏幕上」的那一刻：邮件已经真的删了。
    // 返回 false 才能让 UI 说实话，静默无操作会让用户以为撤销成功。
    expect(ok).toBe(false)
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('从未有过挂起项时 undo 也返回 false', async () => {
    let ok: boolean | undefined
    await act(async () => { ok = api.undo() })
    expect(ok).toBe(false)
  })

  it('关闭页面时落地挂起的操作', async () => {
    const commit = vi.fn()
    await act(async () => api.begin({ commit, rollback: vi.fn() }))
    // 关标签页/关桌面端窗口不会触发 React 卸载清理，
    // 没有这一道兜底，请求就随进程消失了——UI 显示删掉了，重开却又回来
    await act(async () => { window.dispatchEvent(new Event('beforeunload')) })
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('beforeunload 落地后不会在卸载时重复提交', async () => {
    const commit = vi.fn()
    await act(async () => api.begin({ commit, rollback: vi.fn() }))
    await act(async () => { window.dispatchEvent(new Event('beforeunload')) })
    await act(async () => root.unmount())
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('提交与回滚都只发生一次', async () => {
    const commit = vi.fn()
    await act(async () => api.begin({ commit, rollback: vi.fn() }))
    await act(async () => {
      api.flush()
      api.flush()
      api.undo()
    })
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('新操作到来时，上一个先落地', async () => {
    const first = vi.fn()
    const second = vi.fn()
    await act(async () => api.begin({ commit: first, rollback: vi.fn() }))
    await act(async () => api.begin({ commit: second, rollback: vi.fn() }))

    // 第一个的撤销入口已被顶掉，不能让它永远悬在半空
    expect(first).toHaveBeenCalledTimes(1)
    expect(second).not.toHaveBeenCalled()

    await act(async () => api.flush())
    expect(second).toHaveBeenCalledTimes(1)
  })

  it('卸载时落地挂起的操作', async () => {
    const commit = vi.fn()
    await act(async () => api.begin({ commit, rollback: vi.fn() }))
    // 用户以为删掉了，卸载却把请求带走 —— 那是数据不一致
    await act(async () => root.unmount())
    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('没有挂起项时 flush / undo 都是空操作', async () => {
    await act(async () => {
      api.flush()
      api.undo()
    })
    // 不抛异常即可
    expect(true).toBe(true)
  })
})

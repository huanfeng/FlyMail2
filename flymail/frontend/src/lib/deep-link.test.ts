import { describe, it, expect, vi } from 'vitest'
import { needsThreadResolution, createDeepLinkResolver, type DeepLinkActions } from '@/lib/deep-link'

/**
 * 外部链接进来的 `?message=` 要不要补一次定位。
 *
 * 阅读区在两种视图下认的参数不一样（普通列表认 message，会话视图认 thread），
 * 而通知链接只能带 message——发通知时不知道收件人开没开会话视图。
 * 会话视图下点开这种链接就是「列表对了、右边一片空白」。
 */
describe('needsThreadResolution', () => {
  it('会话视图下只有 message 时要补', () => {
    expect(needsThreadResolution({ messageId: 42, conversationView: true, threadId: null })).toBe(
      true,
    )
  })

  it('普通列表本来就认 message，不必绕一趟接口', () => {
    expect(needsThreadResolution({ messageId: 42, conversationView: false, threadId: null })).toBe(
      false,
    )
  })

  it('已经有 thread 了就不补——否则补完又触发，来回打转', () => {
    expect(needsThreadResolution({ messageId: 42, conversationView: true, threadId: 'T1' })).toBe(
      false,
    )
  })

  it('没有 message 时什么也不做', () => {
    expect(needsThreadResolution({ messageId: null, conversationView: true, threadId: null })).toBe(
      false,
    )
  })
})

function makeActions(openResult = true) {
  return {
    open: vi.fn(async (): Promise<boolean> => openResult),
    clearMessage: vi.fn((): void => {}),
    notifyExpired: vi.fn((): void => {}),
  } satisfies DeepLinkActions
}

const deepLink = { messageId: 42, conversationView: true, threadId: null }

/**
 * ── 为什么这组测试存在 ───────────────────────────────────────────────────────
 *
 * 判断「要不要补」的那个纯函数上面已经测过了，但**缺陷从来不在那里**——它们都在
 * 接线上：用 push 还是 replace、失败了管不管、同一个 id 会不会重复解析。
 * 这几条原先长在 Shell 的一个 useEffect 里，那个组件太大没法单独渲染，于是
 * 全部无人看守：改坏了整套前端测试仍然全绿。
 */
describe('createDeepLinkResolver', () => {
  /**
   * ⚠ 必须是 replace。
   *
   * 补定位在语义上是「把这个 URL 修正成等价的 thread 形式」，不是一次导航。
   * push 的话历史里留下 `?message=42` 那一条，用户按后退就回到它——而解析器
   * 已经记下这个 id 不会再补，阅读区于是停在空白且**再也回不来**（只能按前进）。
   */
  it('改写 URL 用 replace，不往历史里塞条目', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions()

    expect(await resolve(deepLink, actions)).toBe('resolved')
    expect(actions.open).toHaveBeenCalledWith(42, true)
  })

  /**
   * 同一个 id 只解析一次。
   *
   * 这条同时挡住 StrictMode 的双跑（两次挂载会并发发两次请求）和
   * 「用户手动删掉 thread 参数 → 又触发」的来回打转。
   */
  it('同一个 id 不重复解析', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions()

    await resolve(deepLink, actions)
    await resolve(deepLink, actions)
    await resolve(deepLink, actions)
    expect(actions.open).toHaveBeenCalledTimes(1)
  })

  it('换一封邮件时正常重新解析', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions()

    await resolve(deepLink, actions)
    await resolve({ ...deepLink, messageId: 99 }, actions)
    expect(actions.open).toHaveBeenCalledTimes(2)
    expect(actions.open).toHaveBeenLastCalledWith(99, true)
  })

  /**
   * ⚠ 取不到邮件时必须清参数并告诉用户。
   *
   * 通知发出后邮件可能被规则移走、在另一端删掉。不处理的话 URL 上挂着一个
   * 永远解释不了的 message，阅读区空白，用户看到的是「点了通知没反应」——
   * 而这恰恰是这个功能最容易被撞上的失败场景。
   */
  it('邮件取不到时清掉 message 参数并提示', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions(false)

    expect(await resolve(deepLink, actions)).toBe('expired')
    expect(actions.clearMessage).toHaveBeenCalledTimes(1)
    expect(actions.notifyExpired).toHaveBeenCalledTimes(1)
  })

  it('成功时不要多余的提示', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions(true)

    await resolve(deepLink, actions)
    expect(actions.clearMessage).not.toHaveBeenCalled()
    expect(actions.notifyExpired).not.toHaveBeenCalled()
  })

  it('不需要补的情况一概不碰接口', async () => {
    const resolve = createDeepLinkResolver()
    const actions = makeActions()

    for (const state of [
      { messageId: 42, conversationView: false, threadId: null },
      { messageId: 42, conversationView: true, threadId: 'T1' },
      { messageId: null, conversationView: true, threadId: null },
    ]) {
      expect(await resolve(state, actions)).toBe('skipped')
    }
    expect(actions.open).not.toHaveBeenCalled()
  })
})

/**
 * 外部链接进来的 `?message=` 要不要补一次定位。
 *
 * ── 为什么会有这个判断 ───────────────────────────────────────────────────────
 *
 * 阅读区在两种视图下认的参数不一样：
 *
 *   普通列表  认 message
 *   会话视图  认 thread
 *
 * 而通知里的「打开邮件」链接只能带 message（发通知时不知道收件人开没开会话视图，
 * 而且用户随时会改）。会话视图下点开这种链接就是「列表对了、右边一片空白」。
 *
 * 所以只在「会话视图 + 有 message 没 thread」时补一次：拿 message 去查详情，
 * 换成 thread 参数。补完 thread 就有了，条件自然不再成立。
 */
export function needsThreadResolution(o: {
  messageId: number | null
  conversationView: boolean
  threadId: string | null
}): boolean {
  if (o.messageId == null) return false
  // 普通列表本来就认 message，不必绕一趟接口
  if (!o.conversationView) return false
  // 已经有 thread 了：要么本来就是会话链接，要么刚补过
  if (o.threadId != null) return false
  return true
}

/** 深链解析要用到的外部动作，由 Shell 注入（每次调用传最新的，避免闭包过期）。 */
export interface DeepLinkActions {
  /** 拉取详情并把 URL 改写成阅读区认得的形式；返回 false 表示这封邮件取不到了 */
  open: (messageId: number, replace: boolean) => Promise<boolean>
  /** 去掉 URL 上那个解释不了的 message 参数 */
  clearMessage: () => void
  /** 告诉用户链接指向的邮件已经不在了 */
  notifyExpired: () => void
}

export type DeepLinkOutcome = 'skipped' | 'resolved' | 'expired'

/**
 * 造一个深链解析器。它记住已经处理过的 id，其余状态全部由调用方传入。
 *
 * ── 为什么抽出来 ─────────────────────────────────────────────────────────────
 *
 * 这段逻辑本身只有几行，但**每一行都是踩出来的**，而它原先长在 Shell 的一个
 * useEffect 里——那个组件太大，没法单独渲染，于是这几条判据一条都没有测试守着，
 * 改坏了整套前端测试仍然全绿。抽成不依赖 React 的形态之后就能直接钉住。
 *
 * 三条不变量：
 *
 * 1. **同一个 id 只解析一次**。不记的话：解析成功会改 URL，改 URL 触发重渲染，
 *    用户手动删掉 thread 参数又会再触发，来回打转。
 *
 * 2. **改写 URL 用 replace，不能 push**。补定位在语义上是「把这个 URL 修正成
 *    等价的 thread 形式」，不是一次导航。push 的话历史里留下 `?message=100`
 *    那一条，用户按后退就回到它——而第 1 条不变量已经记下这个 id 不会再补，
 *    阅读区于是停在空白且**再也回不来**（只能按前进）。
 *    去掉第 1 条更糟：后退会被立刻重新 push 回去，变成后退键失灵。
 *
 * 3. **取不到邮件要清参数并告诉用户**。通知发出后邮件可能被规则移走、在另一端
 *    删掉。不清的话 URL 上挂着一个永远解释不了的 message，用户看到的是
 *    「点了通知没反应」。
 */
export function createDeepLinkResolver() {
  let resolved: number | null = null

  return async function resolve(
    state: { messageId: number | null; conversationView: boolean; threadId: string | null },
    actions: DeepLinkActions,
  ): Promise<DeepLinkOutcome> {
    if (!needsThreadResolution(state)) return 'skipped'
    const id = state.messageId as number
    if (resolved === id) return 'skipped'
    // 先记下再 await：否则 StrictMode 的双跑会并发发两次请求
    resolved = id

    if (await actions.open(id, true)) return 'resolved'

    actions.clearMessage()
    actions.notifyExpired()
    return 'expired'
  }
}

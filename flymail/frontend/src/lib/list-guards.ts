// 邮件列表交互的两道「闸门」。
//
// 背景：列表页有两条会自激的路径，都在「筛选未读 → 逐封点开」时被同时踩中，
// 表现为 React 抛 Minified React error #185（Maximum update depth exceeded）：
//
//   1. 自动标已读：effect 依据缓存里的 seen 决定要不要发请求，但乐观更新要等
//      onMutate 里的 cancelQueries 落地，而 mutate() 立刻触发一次重渲染——
//      这段窗口里 seen 还是 false，于是同一封被反复重发。
//   2. 无限翻页：触发条件基于「筛选后的行数」，未读筛选把已读行剔掉后行数极少，
//      `lastIndex >= rowCount - 5` 恒成立，翻页停不下来。
//
// 两者都不是靠调依赖数组能根治的（数据回填随时会把判据打回原状），必须显式记住
// 「这件事已经做过了」。逻辑抽在这里以便单测覆盖，别再内联回组件。

/** 自动标已读的幂等闸门：同一封邮件在一次打开期间只放行一次。 */
export interface AutoReadGate {
  /**
   * 判断本次是否应发起「标为已读」请求，放行时同时记下状态。
   * @param messageId 当前打开的邮件；null 表示没有打开任何邮件
   * @param unread 该邮件在当前列表数据里是否仍为未读
   */
  shouldSend(messageId: number | null, unread: boolean): boolean
}

export function createAutoReadGate(): AutoReadGate {
  let sent: number | null = null
  return {
    shouldSend(messageId, unread) {
      // 关掉阅读器 → 闸门复位，下次再打开同一封可以重新判断
      if (messageId == null) {
        sent = null
        return false
      }
      // 这次打开已经发过了。即便列表数据把 seen 刷回 false 也不再重发，
      // 顺带让「标为未读」按钮不会被自动标已读立刻改回去。
      if (sent === messageId) return false
      if (!unread) return false
      sent = messageId
      return true
    },
  }
}

/** shouldLoadMore 的输入 */
export interface LoadMoreInput {
  /** 虚拟列表当前最末可见行索引；没有可见行时为 -1 */
  lastIndex: number
  /** 筛选后的行数（含日期分组 header） */
  rowCount: number
  /** 底层邮件条数（未经前端 chips 筛选） */
  messageCount: number
  /** 上一次触发翻页时的 messageCount；从未触发过传 -1 */
  lastLoadedCount: number
  hasNextPage: boolean
  isFetchingNextPage: boolean
}

/**
 * 是否应该自动加载下一页。
 *
 * 除了常规的「接近底部 + 还有下一页 + 当前不在请求中」，额外要求
 * **底层数据相比上次触发确实增长过**：unread / flagged 是前端筛选，新页里的邮件
 * 可能一封都通不过筛选，rowCount 不涨会让接近底部的条件继续成立而无限翻页。
 * 有了这条，翻页次数最多等于总页数。
 */
export function shouldLoadMore(input: LoadMoreInput): boolean {
  const { lastIndex, rowCount, messageCount, lastLoadedCount, hasNextPage, isFetchingNextPage } = input
  if (!hasNextPage || isFetchingNextPage) return false
  if (lastIndex < rowCount - 5) return false
  // 上一轮翻页没带回任何新邮件，别再原地重试
  if (lastLoadedCount === messageCount) return false
  return true
}

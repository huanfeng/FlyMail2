import { useCallback, useEffect, useRef } from 'react'

/**
 * 一次可撤销的操作。
 *
 * UI 的变更在调用 `begin` 之前就已经发生（邮件从列表里消失），
 * 这里的两个回调决定那个变更最终是坐实还是收回。
 */
export interface UndoableOp {
  /** 撤销窗口结束，真正把请求发出去 */
  commit: () => void
  /** 用户点了撤销，把 UI 变更收回 */
  rollback: () => void
}

export interface Undoable {
  /** 挂起一个操作；已有挂起项会先行落地 */
  begin: (op: UndoableOp) => void
  /** 立刻落地挂起项（切换上下文、卸载前调用） */
  flush: () => void
  /**
   * 收回挂起项。
   *
   * 返回是否真的收回了：没有挂起项时返回 false，调用方**必须**据此给出反馈。
   * 详见下方「为什么 undo 有返回值」。
   */
  undo: () => boolean
}

/**
 * 延迟提交 + 撤销。
 *
 * 为什么是「延迟提交」而不是「先做再恢复」：后端删除/移动会物理删掉本地行，
 * 邮件 id 立刻失效，做完之后前端已经没有可以指回去的东西了。所以撤销窗口期内
 * 请求根本不发出——这也正是 Gmail「撤销发送」的做法。
 *
 * 代价是操作比点击晚若干秒才真正生效，因此有四处必须强制落地，
 * 否则用户以为删掉了、服务器上却还在：
 *   1. 新的可撤销操作到来（撤销入口被顶掉了）
 *   2. 组件卸载
 *   3. 切换数据源（撤销入口随当前列表一起消失）
 *   4. 页面被关闭 —— 见下方 beforeunload
 *
 * ## 为什么 undo 有返回值
 *
 * 撤销窗口的存在与否有两个表示：这里的 `pending` 和屏幕上那条 toast。
 * 它们生命周期独立，一旦不同步，用户看到撤销按钮、点下去却什么都没发生——
 * 得到「已撤销」的错觉，邮件实际已永久删除。这是真实的数据损失。
 *
 * 调用方要做的是关掉 toast（见 `useToast().dismiss`），让这种情形根本不出现；
 * 返回值是第二道防线：万一将来又冒出一条绕过 dismiss 的路径，哑火也会变成
 * 一句明确的「已无法撤销」，而不是静默无操作。
 */
export function useUndoable(): Undoable {
  const pending = useRef<UndoableOp | null>(null)

  /** 取出并清空挂起项——所有出口都经由它，保证「取出」与「清空」不会脱节。 */
  const take = useCallback(() => {
    const op = pending.current
    pending.current = null
    return op
  }, [])

  const flush = useCallback(() => {
    take()?.commit()
  }, [take])

  const begin = useCallback(
    (op: UndoableOp) => {
      flush()
      pending.current = op
    },
    [flush],
  )

  const undo = useCallback(() => {
    const op = take()
    op?.rollback()
    return op != null
  }, [take])

  // 卸载时落地。读 ref 而不是调 flush：清理函数里 flush 的身份可能已经变了，
  // 而我们要的只是「把此刻挂着的那个提交掉」。
  useEffect(() => {
    return () => {
      const op = pending.current
      pending.current = null
      op?.commit()
    }
  }, [])

  // 关标签页 / 关桌面端窗口不会触发 React 的卸载清理，挂起的请求会随进程一起消失——
  // UI 上已经删掉了，重新打开却又回来。这里补上最后一次落地机会。
  //
  // 不弹「确定要离开吗」：那是拿一个用户没要求的拦截去换几百毫秒，代价不对等。
  // commit 内部走的 mutation 在 unload 期间未必送得出去，但这是浏览器给的最后一程，
  // 送不出去也只是回到没有这段代码的状态，不会更糟。
  useEffect(() => {
    function onBeforeUnload() {
      const op = pending.current
      pending.current = null
      op?.commit()
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [])

  return { begin, flush, undo }
}

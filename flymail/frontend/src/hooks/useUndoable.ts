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
  /** 收回挂起项 */
  undo: () => void
}

/**
 * 延迟提交 + 撤销。
 *
 * 为什么是「延迟提交」而不是「先做再恢复」：后端删除/移动会物理删掉本地行，
 * 邮件 id 立刻失效，做完之后前端已经没有可以指回去的东西了。所以撤销窗口期内
 * 请求根本不发出——这也正是 Gmail「撤销发送」的做法。
 *
 * 代价是操作比点击晚若干秒才真正生效，因此有两处必须强制落地，
 * 否则用户以为删掉了、服务器上却还在：
 *   1. 新的可撤销操作到来（撤销入口被顶掉了）
 *   2. 组件卸载 / 切换数据源（撤销入口消失了）
 */
export function useUndoable(): Undoable {
  const pending = useRef<UndoableOp | null>(null)

  const flush = useCallback(() => {
    const op = pending.current
    pending.current = null
    op?.commit()
  }, [])

  const begin = useCallback(
    (op: UndoableOp) => {
      flush()
      pending.current = op
    },
    [flush],
  )

  const undo = useCallback(() => {
    const op = pending.current
    pending.current = null
    op?.rollback()
  }, [])

  // 卸载时落地。读 ref 而不是调 flush：清理函数里 flush 的身份可能已经变了，
  // 而我们要的只是「把此刻挂着的那个提交掉」。
  useEffect(() => {
    return () => {
      const op = pending.current
      pending.current = null
      op?.commit()
    }
  }, [])

  return { begin, flush, undo }
}

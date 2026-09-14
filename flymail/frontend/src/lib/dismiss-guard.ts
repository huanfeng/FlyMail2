import * as React from 'react'

/**
 * 「填到一半点了对话框外面」的防误触。
 *
 * ── 判据：拦什么、不拦什么 ───────────────────────────────────────────────────
 *
 * 只拦**关闭会丢掉重建代价高的输入**的对话框，不是所有对话框：
 *
 *   拦   账户 / 通知渠道 / 规则——表单要填一堆字段，误点一次全部重来
 *   拦   配置导入（已载入文件后）——重选文件加重新勾选很烦
 *   不拦 配置导出——只是几个勾选框，重来很便宜
 *   不拦 确认框——拦住一个只有"确定/取消"的框，那是陷阱不是保护
 *   不拦 设置——改动即时生效，根本没有"未保存"这回事
 *
 * 一刀切"所有对话框都不许点外面关"会比原来的问题更糟：用户会发现有些框
 * 怎么点都关不掉，而那些框里根本没有值得保护的东西。
 *
 * ── 只拦点外面，不拦 Esc ─────────────────────────────────────────────────────
 *
 * ⚠ Esc 必须照常关闭。ARIA 的对话框模式规定 Esc 关闭对话框，这是键盘用户
 * 唯一的快速出口——拦掉它等于把人困在框里（要 Tab 到关闭按钮才出得来）。
 * 而且 Esc 是**明确的**离开手势，点到框外多半是手滑，两者性质不同。
 *
 * ── 拦下来要有反馈 ─────────────────────────────────────────────────────────
 *
 * 点了外面却什么都没发生，本身就是另一种困惑（"是不是卡了？"）。
 * 所以拦下时让对话框轻轻抖一下，表示"我收到了，但不能这么关"。
 */

/** 抖动动画的时长，必须与 index.css 里 @keyframes dialogNudge 一致。 */
const NUDGE_MS = 320

export interface DismissGuard {
  /** 挂到 Dialog.Content 上，取得 DOM 以触发抖动 */
  contentRef: React.RefObject<HTMLDivElement | null>
  /** 展开到 Dialog.Content 上的 radix 事件处理器 */
  dismissProps: {
    onPointerDownOutside?: (e: Event) => void
    onInteractOutside?: (e: Event) => void
  }
}

/**
 * @param hasUnsaved 在**关闭那一刻**求值的谓词：此刻有没有值得保护的内容。
 *
 * 传谓词而不是布尔值，有两个实打实的好处：
 *
 * 1. 表单状态不必提到对话框外壳那一层。规则对话框就是这个形状——
 *    外壳持有 Dialog.Content，表单状态在子组件里。把状态提上去要么得
 *    大改结构，要么得在 effect 里往父组件同步 setState（级联渲染）。
 *    而守卫只需要"按下那一刻"的值，压根不需要它参与渲染。
 * 2. 深比较不必每次渲染都跑一遍——只在真的点了框外时才算。
 *
 * 返回假时完全不拦，点外面照常关闭：空表单上加阻力是纯粹的烦人。
 */
export function useDismissGuard(hasUnsaved: () => boolean): DismissGuard {
  const contentRef = React.useRef<HTMLDivElement | null>(null)
  const timer = React.useRef<ReturnType<typeof setTimeout> | null>(null)

  React.useEffect(() => () => {
    if (timer.current != null) clearTimeout(timer.current)
  }, [])

  const nudge = React.useCallback(() => {
    const el = contentRef.current
    if (!el) return
    // 重启 CSS 动画：光是"再加一次同名类"不会重新播放，必须先移除、
    // 强制一次重排、再加回去。读 offsetWidth 就是那次强制重排。
    el.classList.remove('dlg-nudge')
    void el.offsetWidth
    el.classList.add('dlg-nudge')
    if (timer.current != null) clearTimeout(timer.current)
    timer.current = setTimeout(() => el.classList.remove('dlg-nudge'), NUDGE_MS)
  }, [])

  // 处理器**恒定挂着**，判断放在里面：挂不挂随 dirty 变化的话，
  // 同一个组件在两次渲染之间会给 radix 换掉事件处理器，行为更难预料。
  return {
    contentRef,
    dismissProps: {
      // 两个都要拦：onPointerDownOutside 管鼠标/触摸按下，
      // onInteractOutside 还覆盖聚焦移到框外等情形。只拦前者的话，
      // 某些输入方式仍然会把框关掉。
      onPointerDownOutside: (e: Event) => {
        if (!hasUnsaved()) return
        e.preventDefault()
        nudge()
      },
      onInteractOutside: (e: Event) => {
        if (!hasUnsaved()) return
        e.preventDefault()
      },
    },
  }
}

/**
 * 表单是否与基线不同（即"用户改过东西"）。
 *
 * ⚠ 必须**按结构**比，不能按引用。规则表单里是
 * `conditions: ConditionRow[]` / `actions: ActionRow[]`——一组对象。
 * 按引用比的话，每次渲染新建的行对象永远不等于基线里的，表单会被恒判成
 * dirty，于是那个对话框**永远点不掉外面**。防误触变成了关不掉的框，
 * 比它要解决的问题更糟。
 *
 * 也不用 JSON.stringify：键顺序不同会让两个内容相同的对象字符串不等，
 * 同样会得到恒真的 dirty。
 */
function sameValue(a: unknown, b: unknown): boolean {
  if (a === b) return true
  // NaN !== NaN，但对"用户改没改过"而言它们是同一个值
  if (typeof a === 'number' && typeof b === 'number' && Number.isNaN(a) && Number.isNaN(b)) {
    return true
  }
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false
    return a.every((v, i) => sameValue(v, b[i]))
  }
  if (a != null && b != null && typeof a === 'object' && typeof b === 'object') {
    const ka = Object.keys(a as object)
    const kb = Object.keys(b as object)
    if (ka.length !== kb.length) return false
    return ka.every((k) =>
      Object.prototype.hasOwnProperty.call(b, k) &&
      sameValue((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]),
    )
  }
  return false
}

export function isDirty<T extends object>(current: T, baseline: T): boolean {
  return !sameValue(current, baseline)
}

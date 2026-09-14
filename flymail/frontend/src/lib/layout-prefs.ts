// 三栏宽度偏好：AppLayout 拖拽与设置弹框滑块共用。
// 单一真相源 = localStorage；变更通过 LAYOUT_EVENT 广播，使另一处即时同步。

export const LAYOUT_LS_KEY = 'flymail-layout-v1'
/** 宽度变更广播事件名（detail: LayoutWidths）。 */
export const LAYOUT_EVENT = 'flymail:layout-changed'

export interface LayoutWidths {
  sidebar: number
  list: number
  /** 双栏模式右侧浮动阅读/通知面板宽度 */
  slide: number
  /**
   * 紧凑列表行里「发件人」列的宽度。
   *
   * 原来写死成 minmax(160px, 220px)，在三栏形态下总是吃满 220px——
   * 发件人多数是三五个字，剩下一大半空着，而右边的主题却被挤到只剩几个字。
   * 主题才是扫读时真正要看的内容，所以这一列既调窄了默认值，也开放给用户调。
   */
  senderCol: number
}

export const LAYOUT_DEFAULTS: LayoutWidths = { sidebar: 248, list: 380, slide: 720, senderCol: 150 }

export const LAYOUT_LIMITS = {
  sidebar: { min: 180, max: 420 },
  list: { min: 300, max: 680 },
  slide: { min: 420, max: 1200 },
  senderCol: { min: 90, max: 320 },
} as const

export function clampWidth(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v))
}

/**
 * 夹紧并取整到整像素。
 *
 * ⚠ 取整**只能**放在存取的边界上，不能放进 clampWidth——那个函数是拖拽累加器的
 * 出口（`clamp(prev + dx, …)`，状态本身就是累加器）。在那里取整，分数缩放的显示器上
 * 每次小于 0.5px 的位移都会被抹成 0，宽度永远推不动一格，拖拽彻底卡死。
 * 源头的余量累加见 ResizeHandle。
 */
function clampPx(v: number, min: number, max: number): number {
  // 顺序是「先夹紧、再取整」：反过来时取整可能把值推出区间一侧。
  return Math.round(clampWidth(v, min, max))
}

/**
 * 从 localStorage 读取宽度，缺失/非法时回落默认值，夹紧到约束区间并取整。
 *
 * 读取侧取整是为了**自愈**：小数是从早先的版本写进去的，只在写入侧取整的话，
 * 用户得重新拖一次才恢复正常，在那之前设置里那一行仍会渲染成 `503.000003242px`
 * 并撑出横向滚动条。
 */
export function loadLayoutWidths(): LayoutWidths {
  try {
    const raw = localStorage.getItem(LAYOUT_LS_KEY)
    if (raw) {
      const p = JSON.parse(raw) as Partial<LayoutWidths>
      return {
        sidebar: clampPx(p.sidebar ?? LAYOUT_DEFAULTS.sidebar, LAYOUT_LIMITS.sidebar.min, LAYOUT_LIMITS.sidebar.max),
        list: clampPx(p.list ?? LAYOUT_DEFAULTS.list, LAYOUT_LIMITS.list.min, LAYOUT_LIMITS.list.max),
        slide: clampPx(p.slide ?? LAYOUT_DEFAULTS.slide, LAYOUT_LIMITS.slide.min, LAYOUT_LIMITS.slide.max),
        senderCol: clampPx(p.senderCol ?? LAYOUT_DEFAULTS.senderCol, LAYOUT_LIMITS.senderCol.min, LAYOUT_LIMITS.senderCol.max),
      }
    }
  } catch {
    /* ignore */
  }
  return { ...LAYOUT_DEFAULTS }
}

/**
 * 写入 localStorage 并广播变更事件（供另一处监听同步）。
 *
 * 只接受**要改的那几项**，其余从已存的值补齐。这几个宽度有两个所有者
 * （AppLayout 管 sidebar/list/slide，MailList 管 senderCol），各自带防抖：
 * 谁要是按自己手上的快照写整份对象，就会把另一方在这段窗口里的改动覆盖掉。
 * 可复现的路径是先拖发件人列、在它的防抖到期前去拖侧栏——
 * 侧栏会在拖拽中途弹回旧宽度。合并放在这里，两个调用方就都只需关心自己那份。
 */
export function saveLayoutWidths(patch: Partial<LayoutWidths>): void {
  const merged: LayoutWidths = { ...loadLayoutWidths(), ...patch }
  // 写入侧再取一次整：loadLayoutWidths 已经把存量洗干净了，但 patch 是调用方
  // 直接给的，任何一个忘了取整的调用方都会把小数带进来。这里是唯一的写入口。
  const next: LayoutWidths = {
    sidebar: Math.round(merged.sidebar),
    list: Math.round(merged.list),
    slide: Math.round(merged.slide),
    senderCol: Math.round(merged.senderCol),
  }
  try {
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify(next))
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent<LayoutWidths>(LAYOUT_EVENT, { detail: next }))
}

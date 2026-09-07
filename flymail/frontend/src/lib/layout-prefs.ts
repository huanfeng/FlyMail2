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

/** 从 localStorage 读取宽度，缺失/非法时回落默认值并夹紧到约束区间。 */
export function loadLayoutWidths(): LayoutWidths {
  try {
    const raw = localStorage.getItem(LAYOUT_LS_KEY)
    if (raw) {
      const p = JSON.parse(raw) as Partial<LayoutWidths>
      return {
        sidebar: clampWidth(p.sidebar ?? LAYOUT_DEFAULTS.sidebar, LAYOUT_LIMITS.sidebar.min, LAYOUT_LIMITS.sidebar.max),
        list: clampWidth(p.list ?? LAYOUT_DEFAULTS.list, LAYOUT_LIMITS.list.min, LAYOUT_LIMITS.list.max),
        slide: clampWidth(p.slide ?? LAYOUT_DEFAULTS.slide, LAYOUT_LIMITS.slide.min, LAYOUT_LIMITS.slide.max),
        senderCol: clampWidth(p.senderCol ?? LAYOUT_DEFAULTS.senderCol, LAYOUT_LIMITS.senderCol.min, LAYOUT_LIMITS.senderCol.max),
      }
    }
  } catch {
    /* ignore */
  }
  return { ...LAYOUT_DEFAULTS }
}

/** 写入 localStorage 并广播变更事件（供另一处监听同步）。 */
export function saveLayoutWidths(w: LayoutWidths): void {
  try {
    localStorage.setItem(LAYOUT_LS_KEY, JSON.stringify(w))
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent<LayoutWidths>(LAYOUT_EVENT, { detail: w }))
}

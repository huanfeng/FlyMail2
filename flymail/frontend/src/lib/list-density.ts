/**
 * 列表行在窄栏下的堆叠阈值与行高。
 *
 * ── ⚠ 这是一份跨 CSS / JS 的硬契约 ─────────────────────────────────────────
 *
 * 行的排布由 `index.css` 的 `@container (max-width: 520px)` 决定，而虚拟列表按
 * `estimateSize` 的数值用 translateY 排槽位、**不做实测**。两边一旦对不上：
 *
 *   - 阈值对不上 → 某个宽度区间里 CSS 已经堆叠成两行、JS 还按单行给高度，
 *     行内容溢出槽位，主题被下一行盖掉（表现为「主题只露出上半截」）
 *   - 行高对不上 → 同上
 *
 * 所以数值集中在这里，CSS 那侧在注释里指回本文件，并有测试扫 index.css 校验一致。
 */

/** 列表栏宽度小于等于这个值时，行改为两行堆叠。与 index.css 的 @container 同值。 */
export const STACK_WIDTH = 520

/** 紧凑行高：单行形态。= 22(padding) + 21(内容) + 1(border) */
export const COMPACT_ROW_H = 44

/**
 * 紧凑行高：窄栏堆叠形态。
 * = 17(padding 7+10) + 28(第一行) + 2(row-gap) + 19(主题行) + 1(border)
 *
 * ⚠ 第一行是 28 不是文字的 21：那一行里有 28px 的删除/星标按钮，它才是撑高的那个。
 * 顶部内边距取 7 而不是 10，是为了抵掉文字在 28px 行里居中多出的那 3.5px，
 * 让上下**看起来**一样宽（详见 index.css 对应处的注释）。
 * 按 21 算会得到 63——而 .mail-item 被 height:100% 钉死在槽位高度上，grid 只能
 * 从第二行挤：实测行高分配变成 `28px 12px`，19px 的主题被塞进 12px 的行里向下
 * 溢出，视觉上就是「上面留白 14px、下面只剩 7px」的上下不对称。
 */
export const COMPACT_ROW_H_STACKED = 67

/** 卡片行高（三行堆叠），不随栏宽变化。 */
export const CARD_ROW_H = 105

/** 分组标题行高。 */
export const HEADER_ROW_H = 28

/** 按列表样式与栏宽给出行高。 */
export function rowHeight(listStyle: 'compact' | 'card' | string, stacked: boolean): number {
  if (listStyle !== 'compact') return CARD_ROW_H
  return stacked ? COMPACT_ROW_H_STACKED : COMPACT_ROW_H
}

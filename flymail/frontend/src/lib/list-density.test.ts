/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'
import { STACK_WIDTH, COMPACT_ROW_H, COMPACT_ROW_H_STACKED, CARD_ROW_H, rowHeight } from '@/lib/list-density'

const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf-8')

/**
 * 行的堆叠阈值是一份跨 CSS / JS 的硬契约。
 *
 * ── 为什么要用测试钉住 ───────────────────────────────────────────────────────
 *
 * 排布由 index.css 的 `@container` 决定，而虚拟列表按 estimateSize 的数值用
 * translateY 排槽位、**不做实测**。两边对不上时不会报任何错，只是行内容溢出
 * 槽位、被下一行盖住——表现为「主题只露出上半截」，而且只在某个宽度区间出现，
 * 很难联想到是两个数字不一致造成的。
 */
describe('列表行密度契约', () => {
  it('CSS 的 @container 阈值与 JS 常量一致', () => {
    const m = css.match(/@container \(max-width:\s*(\d+)px\)/)
    expect(m, 'index.css 里找不到 @container 规则').not.toBeNull()
    expect(Number(m![1])).toBe(STACK_WIDTH)
  })

  /** 堆叠成两行必然更高；写反了就是行被压扁、内容互相重叠。 */
  it('堆叠形态的行高大于单行形态', () => {
    expect(COMPACT_ROW_H_STACKED).toBeGreaterThan(COMPACT_ROW_H)
  })

  it('rowHeight 按样式与栏宽给值', () => {
    expect(rowHeight('compact', false)).toBe(COMPACT_ROW_H)
    expect(rowHeight('compact', true)).toBe(COMPACT_ROW_H_STACKED)
    // 卡片行本来就是三行堆叠，不随栏宽变化
    expect(rowHeight('card', true)).toBe(CARD_ROW_H)
    expect(rowHeight('card', false)).toBe(CARD_ROW_H)
  })
})

/**
 * 行高必须装得下它的内容。
 *
 * ⚠ .mail-item 被 height:100% 钉死在槽位高度上，算小了 grid 不会把行撑高，
 * 而是**从某一行挤**——实测算成 63 时行高分配变成 `28px 12px`，19px 的主题被
 * 塞进 12px 的行里向下溢出，表现为上下留白不对称（上 14px、下 7px）。
 * 所以这里按构成逐项核对，而不是只写一个数字。
 */
describe('堆叠行高的构成', () => {
  const PADDING = 7 + 10 // 上下故意不等，抵掉文字在按钮撑高的那行里居中的偏移
  const AVATAR_ROW = 28 // 第一行由 28px 的删除/星标按钮撑高，不是文字的 21px
  const ROW_GAP = 2
  const SUBJECT_ROW = 19
  const BORDER = 1

  it('装得下头像行 + 主题行', () => {
    expect(COMPACT_ROW_H_STACKED).toBe(PADDING + AVATAR_ROW + ROW_GAP + SUBJECT_ROW + BORDER)
  })

  it('第一行按头像高度算，不是文字高度', () => {
    // 按文字 21px 算会挤掉主题行的高度，正是造成留白不对称的那个错误
    expect(COMPACT_ROW_H_STACKED).not.toBe(PADDING + 21 + ROW_GAP + SUBJECT_ROW + BORDER)
  })
})

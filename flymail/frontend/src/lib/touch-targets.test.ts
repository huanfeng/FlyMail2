/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 粗指针下触摸目标的两条几何不变量。
 *
 * 背景：`.mi-star` / `.mi-del` 的 class 是 `mi-star icon-btn`——**宽高来自 `.icon-btn`**，
 * 而卡片模式下它们是绝对定位、间隔只有 26px。给 `.icon-btn` 加一条
 * `@media (pointer: coarse) { width: 36px }` 就会让两者叠掉 10px，
 * 点删除键靠右那一侧变成加星标。第一版改动里我写了句注释说「它们不带宽高所以不受影响」，
 * 那句是错的，独立审查抓了出来。
 *
 * 这条测试把「不重叠」变成算术：从 CSS 里读出尺寸与 right，自己算一遍。
 * 它不依赖任何布局引擎，因此在 jsdom 里也成立。
 */
const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf-8')

/** 取出 `@media (pointer: coarse)` 块的内容（括号配对扫描，避免正则被内层 } 截断） */
function coarseBlock(): string {
  const start = css.indexOf('@media (pointer: coarse)')
  expect(start, '找不到 @media (pointer: coarse) 块').toBeGreaterThan(-1)
  const open = css.indexOf('{', start)
  let depth = 0
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}') {
      depth--
      if (depth === 0) return css.slice(open + 1, i)
    }
  }
  throw new Error('@media (pointer: coarse) 块没有闭合')
}

/** 在一段 CSS 里找某个选择器规则的某个属性值（px） */
function px(block: string, selector: string, prop: string): number | null {
  const re = new RegExp(
    `(^|[},])\\s*${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`,
    'm',
  )
  const m = re.exec(block)
  if (!m) return null
  const p = new RegExp(`${prop}:\\s*(-?[\\d.]+)px`).exec(m[2])
  return p ? Number(p[1]) : null
}

describe('触摸尺度', () => {
  const block = coarseBlock()

  it('星标与删除在卡片模式下放大后不重叠', () => {
    const size = px(block, '.mi-star, .mi-del', 'width')
    const starRight = px(block, '.mail-item:not(.mail-item-row) .mi-star', 'right')
    const delRight = px(block, '.mail-item:not(.mail-item-row) .mi-del', 'right')

    expect(size, '粗指针下没有给 .mi-star/.mi-del 定尺寸——它们会继承 .icon-btn 的那条').not.toBeNull()
    expect(starRight, '缺 .mi-star 的 right').not.toBeNull()
    expect(delRight, '缺 .mi-del 的 right').not.toBeNull()

    // 两者都从右边缘算起：星标占 [starRight, starRight + size]，删除占 [delRight, delRight + size]
    expect(
      delRight!,
      `删除按钮 right=${delRight} 落在星标占据的 [${starRight}, ${starRight! + size!}] 里`,
    ).toBeGreaterThanOrEqual(starRight! + size!)
  })

  it('放大后的尺寸确实比默认大', () => {
    // 防的是「为了不重叠把它们改小」这种把问题掉个头的修法
    const base = /\.icon-btn\s*\{[^}]*width:\s*(\d+)px/.exec(css)
    expect(base, '找不到 .icon-btn 的默认宽度').not.toBeNull()
    const size = px(block, '.mi-star, .mi-del', 'width')
    expect(size!).toBeGreaterThanOrEqual(Number(base![1]))
  })
})

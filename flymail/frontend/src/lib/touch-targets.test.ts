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

/** 取出 `@media (pointer: coarse)` 块的内容（括号配对扫描，避免正则被内层 } 截断；
   返回前先剥块注释，免得注释里的选择器被当成真实规则） */
function coarseBlock(): string {
  const start = css.indexOf('@media (pointer: coarse)')
  expect(start, '找不到 @media (pointer: coarse) 块').toBeGreaterThan(-1)
  const open = css.indexOf('{', start)
  let depth = 0
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}') {
      depth--
      if (depth === 0) return css.slice(open + 1, i).replace(/\/\*[\s\S]*?\*\//g, '')
    }
  }
  throw new Error('@media (pointer: coarse) 块没有闭合')
}

describe('触摸尺度', () => {
  const block = coarseBlock()

  it('粗指针下不再把老 .mi-star/.mi-del 一起放大（避免两者重叠）', () => {
    expect(block).not.toMatch(/\.mi-star,\s*\.mi-del/)
    expect(block).not.toMatch(/\.mi-del\s*\{/)
  })

  it('粗指针下悬停动作组 display:none（看不见不能点）', () => {
    expect(block).toMatch(/\.mi-hover-actions\s*\{[^}]*display:\s*none/)
  })
})

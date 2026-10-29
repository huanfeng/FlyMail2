/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve as resolvePath } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 第九轮 UI 打磨的 CSS 结构性约束（设计见 docs/flymail/r9.md / ui-polish-r9-plan.md）。
 *
 * 断言的是「规则存在且落在正确的媒体查询块里」，像素由截图回归负责。
 */
const css = readFileSync(resolvePath(__dirname, '../index.css'), 'utf8')
/** 剥掉块注释：仓库 CSS 注释里大量出现选择器与属性名，不剥会误匹配 */
const stripped = css.replace(/\/\*[\s\S]*?\*\//g, '')

/** 取某个选择器块的声明文本（第一处命中） */
function block(selector: string): string {
  const esc = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const m = stripped.match(new RegExp(`(^|[}\\s])${esc}\\s*\\{([^}]*)\\}`, 'm'))
  return m ? m[2] : ''
}

/** 把所有匹配的 @media 块拼成一段声明文本（括号配对扫描，不被内层规则截断） */
function mediaBlocks(mediaQuery: string): string {
  const out: string[] = []
  let pos = 0
  while (true) {
    const start = stripped.indexOf(`@media ${mediaQuery}`, pos)
    if (start === -1) break
    const open = stripped.indexOf('{', start)
    if (open === -1) break
    let depth = 0
    let end = -1
    for (let i = open; i < stripped.length; i++) {
      if (stripped[i] === '{') depth++
      else if (stripped[i] === '}') {
        depth--
        if (depth === 0) { end = i; break }
      }
    }
    if (end === -1) break
    out.push(stripped.slice(open + 1, end))
    pos = end + 1
  }
  return out.join('\n')
}

describe('R9-1 悬停动作组', () => {
  it('细指针下默认隐藏、行悬停显示；时间同步淡出', () => {
    const m = mediaBlocks('(hover: hover) and (pointer: fine)')
    expect(m).toMatch(/\.mi-hover-actions\s*\{[^}]*opacity:\s*0/)
    expect(m).toMatch(/\.mail-item:hover \.mi-hover-actions[^{]*\{[^}]*opacity:\s*1/)
    expect(m).toMatch(/\.mail-item:hover \.mi-time[^{]*\{[^}]*opacity:\s*0/)
  })
  it('粗指针下 display:none（看不见的按钮不能可点）', () => {
    const m = mediaBlocks('(pointer: coarse)')
    expect(m).toMatch(/\.mi-hover-actions\s*\{[^}]*display:\s*none/)
  })
  it('旧的 .mi-del 规则已移除', () => {
    expect(stripped).not.toMatch(/\.mi-del\b/)
  })
})

describe('R9-2 选择栏对齐', () => {
  it('工具栏左内边距与行复选框列共用 --select-col-x', () => {
    expect(block('.list-toolbar')).toMatch(/padding:\s*0 14px 0 var\(--select-col-x\)/)
    expect(block('.mail-item')).toMatch(/padding:\s*14px\s+20px/)
    expect(stripped).toMatch(/--select-col-x:\s*20px/)
  })
  it('无选中时复选框仅在悬停行覆盖头像出现', () => {
    const m = [...stripped.matchAll(/@media \(hover: hover\) and \(pointer: fine\)\s*\{([\s\S]*?)\n\}/g)].map((x) => x[1]).join('\n')
    expect(m).toMatch(/\.mail-list:not\(\.selecting\) \.mail-item:hover \.mi-select-hover[^{]*\{[^}]*display:\s*flex/)
  })
  it('粗指针下显示选择开关', () => {
    const m = mediaBlocks('(pointer: coarse)')
    expect(m).toMatch(/\.lt-select-touch\s*\{[^}]*display:\s*inline-flex/)
  })
})

describe('R9-3 字体与字重', () => {
  it('字重令牌齐全', () => {
    for (const [k, v] of [['regular', 400], ['medium', 500], ['semibold', 600], ['bold', 700]] as const) {
      expect(stripped).toMatch(new RegExp(`--fw-${k}:\\s*${v}`))
    }
  })
  it('正文字体栈：system-ui 打头，YaHei UI 在 YaHei 之前', () => {
    const m = stripped.match(/--font-body:\s*([^;]+);/)
    expect(m![1].trim().startsWith('system-ui')).toBe(true)
    expect(m![1].indexOf('"Microsoft YaHei UI"')).toBeLessThan(m![1].indexOf('"Microsoft YaHei",'))
  })
  it('标题类用 semibold', () => {
    expect(stripped).toMatch(/h1,\s*h2,\s*h3,\s*h4\s*\{[^}]*font-weight:\s*var\(--fw-semibold\)/)
    expect(block('.list-title')).toMatch(/font-weight:\s*var\(--fw-semibold\)/)
    expect(block('.settings-block h3')).toMatch(/font-weight:\s*var\(--fw-semibold\)/)
  })
  it('已读行发件人与主题是正常黑；未读用 bold / semibold', () => {
    expect(block('.mail-item .mi-sender')).toMatch(/color:\s*var\(--ink\)/)
    expect(block('.mail-item .mi-sender')).toMatch(/font-weight:\s*var\(--fw-medium\)/)
    expect(block('.mail-item .mi-subject')).toMatch(/color:\s*var\(--ink\)/)
    expect(stripped).toMatch(/\.mail-item\.unread \.mi-sender\s*\{[^}]*font-weight:\s*var\(--fw-bold\)/)
    expect(stripped).toMatch(/\.mail-item\.unread \.mi-subject\s*\{[^}]*font-weight:\s*var\(--fw-semibold\)/)
  })
  it('等宽字体只出现在白名单选择器', () => {
    const MONO_ALLOWED = new Set([
      '.compose-btn .kbd', '.search-input .kbd', '.search-help .sh-row code',
      '.reader-empty .shortcuts kbd', '.sc-kbd', '.settings-list-row .slr-mono',
      '.list-error-detail', '.attach-card .ac-ic',
    ])
    const hits: string[] = []
    for (const m of stripped.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      if (/font-family:\s*var\(--font-mono\)/.test(m[2])) hits.push(m[1].trim().replace(/\s+/g, ' '))
    }
    expect(hits.filter((s) => !MONO_ALLOWED.has(s))).toEqual([])
  })
  it('数字类改为比例字体 + 等宽数字', () => {
    for (const sel of ['.mail-item .mi-time', '.folder-row .f-count', '.slider-row .slider-val']) {
      expect(block(sel)).toMatch(/font-variant-numeric:\s*tabular-nums/)
      expect(block(sel)).not.toMatch(/--font-mono/)
    }
  })
})

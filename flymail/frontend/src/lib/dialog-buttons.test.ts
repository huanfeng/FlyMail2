/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve as resolvePath } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 对话框里 shadcn Button 的可读性。
 *
 * ── 缘起 ─────────────────────────────────────────────────────────────────
 *
 * 用户报「测试连接 / 取消 / 保存 这几个按钮几乎看不清，但又有底框」。
 * 按公式逐主题算下来是两处，成因都是 shadcn 与本项目**同名令牌语义相反**：
 *
 * 1. 悬停态（严重）：outline / ghost 变体写的是 `hover:bg-accent
 *    hover:text-accent-foreground`。而 --accent 在本项目是**饱和品牌色**，
 *    在 shadcn 语义里却是「很淡的悬停底」。照直映射的结果是鼠标一悬停，
 *    按钮底变成整块品牌色，文字却用 shadcn 那个静态的 --accent-foreground
 *    （亮色近黑、暗色近白）。修复前 18 组里 11 组低于 4.5:1，
 *    8 组暗色主题全部落在 1.60~2.45——浅底压近白字，读不出来。
 *
 * 2. 静息态：`bg-background` 被桥接成 var(--surface)，与对话框底色**完全相同**，
 *    按钮唯一的边界是那条 1px 边框，而它对底色的对比只有 1.39~1.64，
 *    18 组全部低于 WCAG 1.4.11 要求的 3:1。这正是「看得见一圈线，但不像按钮」。
 *
 * 3. 真正的病根（上面两条修完用户仍报「没解决」才找到的）：index.css 里
 *    **不分层**地写了 `button { background: none; border: 0; padding: 0 }`。
 *    Tailwind v4 的工具类在 @layer utilities 里，而不分层的规则在层叠里
 *    无条件压过所有分层规则、与权重无关——于是 bg-primary / border / px-3
 *    全部作废，四个按钮（含主按钮）都是透明无边框的裸文字。前两条只是在
 *    调一个根本没画出来的边框的颜色。见文件末尾「层叠」那组断言。
 *
 * ── 这条测试钉的是结果，不是写法 ───────────────────────────────────────────
 *
 * 它从 CSS 里把工具类**实际会展开成**的令牌解析出来再算颜色。
 * 换一个令牌、换一种桥接方式，只要 18 组主题仍然达标就照样通过。
 *
 * ⚠ 解析路径必须走 `@theme inline`，不能直接读 --accent：
 * Tailwind 的 `bg-accent` 展开成 `var(--color-accent)` 在 @theme inline 里
 * 映射到的那个东西。而且 --accent 是在 `[data-theme][data-mode]`（特异性 0,3,0）
 * 里定义的，:root（0,1,0）根本盖不住——修复一度就改错了位置。
 */

const css = readFileSync(resolvePath(process.cwd(), 'src/index.css'), 'utf-8').replace(
  // 剥注释：本文件解释这些令牌的那几段注释里就带着选择器与变量名
  /\/\*[\s\S]*?\*\//g,
  '',
)

/** 收集某类选择器下的自定义属性声明（同特异性按文档顺序，后者覆盖前者）。 */
function declsOf(re: RegExp): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of css.matchAll(re)) {
    for (const d of m[1].matchAll(/--([\w-]+):\s*([^;]+);/g)) out[d[1]] = d[2].trim()
  }
  return out
}

const themeAlias = declsOf(/@theme inline\s*\{([\s\S]*?)\n\}/g) // --color-* → var(--x)
const rootVars = declsOf(/(?:^|\})\s*:root\s*\{([^}]*)\}/g)
const darkVars = declsOf(/(?:^|\})\s*\.dark\s*\{([^}]*)\}/g)

/** 18 组主题各自的令牌表（特异性最高，盖过 :root 与 .dark）。 */
function themes(): Map<string, Record<string, string>> {
  const out = new Map<string, Record<string, string>>()
  for (const m of css.matchAll(/\[data-theme="(\w+)"\]\[data-mode="(\w+)"\]\s*\{([^}]*)\}/g)) {
    const vars: Record<string, string> = {}
    for (const d of m[3].matchAll(/--([\w-]+):\s*([^;]+);/g)) vars[d[1]] = d[2].trim()
    out.set(`${m[1]}/${m[2]}`, vars)
  }
  return out
}

/** 某个 scoped 规则里某个属性的值，例如 outline 按钮的 border-color。 */
function ruleProp(selector: string, prop: string): string | null {
  const re = new RegExp(
    `${selector.replace(/[[\]().*+?^$|\\{}]/g, '\\$&')}\\s*\\{([^}]*)\\}`,
    'g',
  )
  for (const m of css.matchAll(re)) {
    const p = new RegExp(`${prop}:\\s*([^;]+);`).exec(m[1])
    if (p) return p[1].trim()
  }
  return null
}

function varsFor(name: string, t: Record<string, string>): Record<string, string> {
  // 特异性从低到高：:root → .dark（仅暗色）→ 主题块
  return { ...rootVars, ...(name.endsWith('/dark') ? darkVars : {}), ...t }
}

function deref(expr: string, vars: Record<string, string>): string {
  let v = expr
  for (let i = 0; i < 12 && /var\(--[\w-]+\)/.test(v); i++) {
    v = v.replace(/var\(--([\w-]+)\)/g, (_, n: string) => vars[n] ?? themeAlias[n] ?? 'UNDEF')
  }
  return v.trim()
}

// ── 颜色 ────────────────────────────────────────────────────────────────────

/** oklch(L 0 0) 灰阶 → sRGB。shadcn 基础令牌都是无彩度的，这已足够精确。 */
function oklchGray(s: string): [number, number, number, number] | null {
  const m = /^oklch\(\s*([\d.]+)\s+0\s+0(?:\s*\/\s*[\d.%]+)?\s*\)$/.exec(s)
  if (!m) return null
  const lin = Number(m[1]) ** 3
  const c = lin <= 0.0031308 ? lin * 12.92 : 1.055 * lin ** (1 / 2.4) - 0.055
  const v = Math.max(0, Math.min(255, Math.round(c * 255)))
  return [v, v, v, 1]
}

function rgba(x: string): [number, number, number, number] | null {
  const s = String(x).trim()
  const g = oklchGray(s)
  if (g) return g
  const h = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(s.replace('#', ''))
  if (h) {
    const n = h[1].length === 3 ? h[1].split('').map((c) => c + c).join('') : h[1]
    return [
      parseInt(n.slice(0, 2), 16),
      parseInt(n.slice(2, 4), 16),
      parseInt(n.slice(4, 6), 16),
      1,
    ]
  }
  const m = /^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,/\s]+([\d.]+))?\s*\)$/i.exec(s)
  if (m) return [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]]
  return null
}

/**
 * 把带 alpha 的前景合成到底色上。
 *
 * ⚠ 不做这一步就会漏掉整条缺陷：--rule-strong 是 rgba(...,0.17)，
 * 直接算对比会得到 n/a，而"算不出来"在草率的实现里很容易被当成"没问题"。
 */
function flatten(fg: string, bg: string): [number, number, number] | null {
  const a = rgba(fg)
  const b = rgba(bg)
  if (!a || !b) return null
  return [0, 1, 2].map((i) => Math.round(a[i] * a[3] + b[i] * (1 - a[3]))) as [number, number, number]
}

const srgb = (c: number) => (c / 255 <= 0.03928 ? c / 255 / 12.92 : ((c / 255 + 0.055) / 1.055) ** 2.4)
const lum = (a: [number, number, number]) => 0.2126 * srgb(a[0]) + 0.7152 * srgb(a[1]) + 0.0722 * srgb(a[2])

function contrast(fg: string, bg: string): number | null {
  const f = flatten(fg, bg)
  const b = rgba(bg)
  if (!f || !b) return null
  const [hi, lo] = [lum(f), lum([b[0], b[1], b[2]])].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

// ── 断言 ────────────────────────────────────────────────────────────────────

describe('对话框按钮的可读性（全部 18 组主题）', () => {
  const all = themes()

  it('前提：解析到了 18 组主题与 @theme 别名', () => {
    // 没有这条，下面任何"全部达标"都可能只是因为一组都没扫到
    expect(all.size).toBe(18)
    expect(themeAlias['color-accent'], '@theme inline 里没有 --color-accent').toBeTruthy()
  })

  it('悬停态文字对底色 ≥ 4.5:1', () => {
    // bg-accent / text-accent-foreground 展开成 @theme inline 里映射到的那个令牌
    const bgExpr = themeAlias['color-accent']
    const fgExpr = themeAlias['color-accent-foreground']
    const bad: string[] = []
    for (const [name, t] of all) {
      const vars = varsFor(name, t)
      // ⚠ 悬停底色可能是**半透明覆盖层**（--bg-hover 是 rgba(...,0.035)）。
      // 必须先把它合成到按钮自身的底色上，再拿结果去算文字对比。
      // 直接拿半透明值当底色算，得到的是它与自己的对比 = 1.00——
      // 一个看起来"全都不达标"的假结论。本文件第一版就是这么错的。
      const surface = deref('var(--background)', vars)
      const overlay = deref(bgExpr, vars)
      const flat = flatten(overlay, surface)
      expect(flat, `${name}: 悬停底色算不出来（${overlay} on ${surface}）`).not.toBeNull()
      const bg = '#' + flat!.map((c) => c.toString(16).padStart(2, '0')).join('')
      const fg = deref(fgExpr, vars)
      const c = contrast(fg, bg)
      expect(c, `${name}: 悬停文字色算不出来（${fg} on ${bg}）`).not.toBeNull()
      if (c != null && c < 4.5) bad.push(`${name} ${c.toFixed(2)} (${fg} on ${bg})`)
    }
    expect(bad, '悬停时按钮文字读不出来').toEqual([])
  })

  it('outline 按钮的边框对相邻底色 ≥ 3:1（WCAG 1.4.11）', () => {
    // 静息态的 bg-background 与对话框底色相同，边框是唯一的边界
    const borderExpr = ruleProp("[data-slot='button'][data-variant='outline']", 'border-color')
    expect(borderExpr, '没有给 outline 按钮单独设边框色，它会退回 1.4 对比的 --border').not.toBeNull()

    const bad: string[] = []
    for (const [name, t] of all) {
      const vars = varsFor(name, t)
      const border = deref(borderExpr!, vars)
      const bg = deref('var(--surface)', vars)
      const c = contrast(border, bg)
      expect(c, `${name}: 边框色算不出来（${border} on ${bg}）`).not.toBeNull()
      if (c != null && c < 3) bad.push(`${name} ${c.toFixed(2)}`)
    }
    expect(bad, '按钮边界看不清——「几乎看不清，但又有底框」').toEqual([])
  })

  it('主按钮（保存）文字对底色 ≥ 4.5:1', () => {
    // 这条修复前就是达标的，留着是回归护栏：--primary 的桥接很容易在
    // 调主题色时被顺手改坏，而那时没人会想到去看保存按钮。
    const bad: string[] = []
    for (const [name, t] of all) {
      const vars = varsFor(name, t)
      const bg = deref('var(--primary)', vars)
      const fg = deref('var(--primary-foreground)', vars)
      const c = contrast(fg, bg)
      expect(c, `${name}: 主按钮颜色算不出来（${fg} on ${bg}）`).not.toBeNull()
      if (c != null && c < 4.5) bad.push(`${name} ${c.toFixed(2)}`)
    }
    expect(bad).toEqual([])
  })
})

/**
 * 层叠：元素级重置不能裸写在 @layer 之外。
 *
 * 上面三组对比度断言全部通过的那个版本，按钮在浏览器里仍然是裸文字——
 * 它们算的是「工具类展开成的颜色」，默认了工具类会生效。而只要 index.css
 * 里有一条不分层的 `button { border: 0 }`，工具类就一个都不会生效：
 * CSS 层叠里，未分层样式 > 任何 @layer 内的样式，权重再高也没用。
 *
 * jsdom 不实现 @layer 的层叠，所以这里退一步钉住结构：任何**未分层**的规则，
 * 只要选择器里有裸的 button / input / textarea / select 类型选择器，就不许
 * 动 background / border / padding / color 这四样——它们正是 shadcn 变体
 * 靠工具类设置的东西。放进 @layer base 就不受此限。
 */
describe('元素级重置不得压掉工具类（层叠）', () => {
  /** 未分层的规则列表：[选择器, 声明块]。 */
  function unlayeredRules(): Array<[string, string]> {
    const out: Array<[string, string]> = []
    // 逐字符扫描，记录当前所在的 @layer / @theme / @keyframes 嵌套深度
    let i = 0
    const stack: Array<'layer' | 'other'> = []
    let selStart = 0
    while (i < css.length) {
      const ch = css[i]
      if (ch === '{') {
        const head = css.slice(selStart, i).trim()
        // 找到与之配对的 '}'，判断这是声明块还是嵌套块
        let depth = 1
        let j = i + 1
        for (; j < css.length && depth > 0; j++) {
          if (css[j] === '{') depth++
          else if (css[j] === '}') depth--
        }
        const body = css.slice(i + 1, j - 1)
        const isBlockAt = /^@(layer|theme|media|supports|keyframes|container)/.test(head)
        const nested = /\{/.test(body)
        if (isBlockAt || nested) {
          stack.push(/^@(layer|theme)\b/.test(head) ? 'layer' : 'other')
          i++
          selStart = i
          continue
        }
        if (!stack.includes('layer') && !head.startsWith('@')) out.push([head, body])
        i = j
        selStart = i
        continue
      }
      if (ch === '}') {
        stack.pop()
        i++
        selStart = i
        continue
      }
      i++
    }
    return out
  }

  const bareElement = /(^|[\s,>+~])(button|input|textarea|select)(?=$|[\s,:>+~])/

  it('前提：能解析出未分层规则，且 base 层里确实有 button 重置', () => {
    const rules = unlayeredRules()
    expect(rules.length).toBeGreaterThan(50)
    expect(css, 'button 的基础重置应在 @layer base 里').toMatch(
      // 同一个 @layer base 块里前面还可以有别的声明块（如 input, button, textarea {…}）
      /@layer base\s*\{(?:[^{}]*\{[^}]*\})*[^{}]*button\s*\{[^}]*background:\s*none/,
    )
  })

  it('未分层规则里裸的 button/input/textarea/select 不许设 background/border/padding/color', () => {
    const bad: string[] = []
    for (const [sel, body] of unlayeredRules()) {
      const selectors = sel.split(',').map((x) => x.trim())
      // 只看**整个选择器就是裸元素**的那种（如 `button`、`input, button`），
      // 带类名/属性限定的（`.mode-toggle button`）作用范围是明确的，不在此列。
      if (!selectors.some((x) => /^(button|input|textarea|select)$/.test(x))) continue
      const hit = /(^|;)\s*(background|border|padding|color)(-[\w-]+)?\s*:/.exec(body)
      if (hit) bad.push(`${sel} { …${hit[2]}… }`)
    }
    expect(
      bad,
      '这些裸写的元素重置会无条件压过 @layer utilities，让 shadcn Button 的工具类全部失效',
    ).toEqual([])
  })

  it('（自检）裸元素选择器的判定能识别 `input, button, textarea`', () => {
    expect(bareElement.test('input, button, textarea')).toBe(true)
    expect(bareElement.test('.mode-toggle button')).toBe(true)
    expect(bareElement.test('.pill-btn')).toBe(false)
  })
})

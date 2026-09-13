/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 焦点指示器的对比度。
 *
 * WCAG 2.1 的 1.4.11（非文本对比）要求焦点指示器对相邻底色至少 3:1。
 * 焦点环原先用 `--accent`，按公式逐主题算下来在**亮色**主题普遍不够：
 *
 *   butter/light  环对页面底色 2.17，对选中行的 accent-wash 只有 1.94
 *   aqua/light    2.67 / 2.33
 *   coral/light   2.99 / 2.67
 *   warm/light    3.00 / 2.68
 *   rose/light    3.30 / 2.75
 *   mint/light    3.31 / 2.99
 *
 * 六组亮色主题的选中行上，键盘焦点基本看不见。
 * （审查清单第 20 条猜的是"深色主题对比不足"——实测正好相反，暗色全部在 5.8 以上。
 *   这也是"按公式算一遍"和"看一眼觉得够"的差别。）
 *
 * 换成同色相的可及调 `--accent-ink` 之后，18 组 × 四种底色最低 4.65。
 *
 * ⚠ 这条测试钉的是**结果**不是写法：它从 CSS 里把焦点规则实际用的那个令牌解析
 * 出来再算，换成别的令牌只要仍然达标就照样通过。第四轮那条主按钮对比度测试
 * 最初钉的是写法，后来才改成钉结果——同一个教训不重复第二遍。
 */
/**
 * 读 CSS 原文并**剥掉注释**。
 *
 * ⚠ 不剥的话正则会把注释里提到的选择器当成真规则：本文件的第一版就栽在这上面——
 * 一段解释 `:focus-visible` 的注释被 `/:focus-visible[^{]*\{/` 匹配上，
 * `[^{]*` 一路吃到注释之后的下一个 `{`，于是把一个完全无关的规则体当成了焦点规则，
 * 报出「--bg-alt 对比度 1.00」这种看不懂的失败。
 * 这个仓库的 CSS 注释写得很密，剥注释不是可选项。
 */
const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf-8').replace(
  /\/\*[\s\S]*?\*\//g,
  '',
)

/** 每个主题块里的令牌表 */
function themeBlocks(): Map<string, Record<string, string>> {
  const out = new Map<string, Record<string, string>>()
  const re = /\[data-theme="(\w+)"\]\[data-mode="(\w+)"\]\s*\{([^}]*)\}/g
  let m: RegExpExecArray | null
  while ((m = re.exec(css)) != null) {
    const vars: Record<string, string> = {}
    for (const v of m[3].matchAll(/--([\w-]+):\s*([^;]+);/g)) vars[v[1]] = v[2].trim()
    out.set(`${m[1]}/${m[2]}`, vars)
  }
  return out
}

function srgb(c: number): number {
  const s = c / 255
  return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
}

function luminance(hex: string): number {
  const h = hex.replace('#', '')
  const n = h.length === 3 ? h.split('').map((x) => x + x).join('') : h
  return (
    0.2126 * srgb(parseInt(n.slice(0, 2), 16)) +
    0.7152 * srgb(parseInt(n.slice(2, 4), 16)) +
    0.0722 * srgb(parseInt(n.slice(4, 6), 16))
  )
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

/**
 * 所有 :focus-visible 规则里，用来画焦点表达的令牌名（去重）。
 *
 * ⚠ 不能只扫 `outline:`。仓库里三个拖拽手柄（.col-resize / .list-col-resize /
 * .slide-resize）的焦点表达是**整条底色**——它们是 3px 宽的分隔条，画不下描边。
 * 初版正则只认 outline，于是这三条一直在用不达标的 --accent 而测试全绿：
 * 换个 CSS 属性就绕过了整条守卫。box-shadow 一并扫上，那是第三种常见写法。
 */
function focusRingTokens(): string[] {
  const names = new Set<string>()
  for (const m of css.matchAll(/:focus-visible[^{]*\{([^}]*)\}/g)) {
    for (const o of m[1].matchAll(/(?:outline|background|box-shadow):[^;]*var\(--([\w-]+)\)/g)) {
      names.add(o[1])
    }
  }
  return [...names]
}

/**
 * `--focus-ring` 在两种模式下各自指向哪个令牌。
 *
 * 焦点环不能一刀切：亮暗两边的失败方式是**相反**的（见 index.css 里那段注释，
 * 以及下面那条结构断言）。所以这里要先解析一层间接，再去算颜色。
 */
function ringTokenByMode(): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of css.matchAll(
    /\[data-theme\]\[data-mode=['"](\w+)['"]\]\s*\{\s*--focus-ring:\s*var\(--([\w-]+)\)/g,
  )) {
    out[m[1]] = m[2]
  }
  return out
}

/** hex → HSL 的色相与彩度（色相单位为度，彩度 0~1） */
function hsl(hex: string): { hue: number; chroma: number } {
  const h = hex.replace('#', '')
  const n = h.length === 3 ? h.split('').map((x) => x + x).join('') : h
  const r = parseInt(n.slice(0, 2), 16) / 255
  const g = parseInt(n.slice(2, 4), 16) / 255
  const b = parseInt(n.slice(4, 6), 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const chroma = max - min
  if (chroma === 0) return { hue: 0, chroma: 0 }
  let hue: number
  if (max === r) hue = ((g - b) / chroma) % 6
  else if (max === g) hue = (b - r) / chroma + 2
  else hue = (r - g) / chroma + 4
  hue *= 60
  return { hue: (hue + 360) % 360, chroma }
}

/** 两个色相的环形差（度） */
function hueDelta(a: number, b: number): number {
  const d = Math.abs(a - b) % 360
  return d > 180 ? 360 - d : d
}

/** WCAG 1.4.11：非文本对比的门槛 */
const MIN_NON_TEXT = 3

describe('焦点指示器的对比度', () => {
  it('读到了 index.css，且确实解析出了焦点规则与主题块', () => {
    // 解析失败时下面每条都会"通过"（空集合的 for 循环不跑任何断言），
    // 那是最坏的假绿灯。这条让根因先说话。
    expect(css.length).toBeGreaterThan(10_000)
    // 钉死 18 而不是 >=18：`>=` 挡不住「正则少解析到一半」这种退化，
    // 而那正是本文件最容易出的假绿灯——少解析的那些主题一条断言都不会跑。
    expect(themeBlocks().size).toBe(18)
    expect(focusRingTokens()).toEqual(['focus-ring'])
    expect(Object.keys(ringTokenByMode()).sort()).toEqual(['dark', 'light'])
  })

  it('焦点环按亮暗分别取值——一刀切在任一边都会退化', () => {
    // 这条钉的是**结构**而不是数值，因为两边的失败方式相反、而没有一个
    // 站得住的数值门槛能同时表达它们：
    //
    //   亮色用 --accent   → 对底色不足（butter/light 对选中行 1.94，六组不及格）
    //   暗色用 --accent-ink → 对底色够，但它在暗色里接近正文色
    //                        （slate/dark 对 --ink 只有 1.16），焦点环变成
    //                        "和正文同亮的随机白边"，强调色这条辨识线索没了
    //
    // 想给后者加个 ring/ink 的数值门槛，实测最小值是 1.25（slate/light，
    // 与模式无关、一直如此），门槛只能定在 1.20——4% 的余量会被任何一次
    // 色板微调误伤。一个会乱叫的守卫最后会被删掉，所以这里改钉结构。
    const byMode = ringTokenByMode()
    expect(byMode.light).not.toBe(byMode.dark)
  })

  it('焦点环是主题强调色的一种调，而不是另起一个颜色', () => {
    // 这条守的是「强调色 = 焦点」那条辨识线索，也就是第 6 条复审真正指出的东西。
    //
    // ⚠ 为什么不用「环色与正文 --ink 的对比度」来守：
    //   1. 两个目标在数学上对立——亮色主题里 --ink 深、底色浅，要让环远离 ink
    //      就得调亮，一调亮就靠近底色，上面那条 3:1 立刻掉。实测这个反相关很干净：
    //      vs-ink 最高的 butter/light（2.83）恰好是 vs 底色最低的那组（4.65），
    //      而 vs-ink 最低的 slate/light（1.25）vs 底色高达 10.58。
    //   2. 门槛无处可放：要 18 组全过得 ≤1.25，那几乎拦不住任何东西；
    //      设 1.3 就只有 slate/light 失败，而 slate 是**灰阶主题**，
    //      它的强调色按定义就是灰，永远不可能在明度上远离灰色的正文——
    //      那不是缺陷，是主题的设计意图。
    //   3. 真正要守的是**色相/彩度**而不是明度：暗色换回 --accent 之后
    //      vs-ink 只有 1.39~2.09，并不比被换掉的 1.16 高多少，但它看起来明显更好，
    //      因为它是黄的而正文是灰白的。明度比量不到这个差别。
    //
    // 所以直接钉色相：调深调浅随意（只要仍满足上面那条 3:1），
    // 但换成一个与主题强调色无关的颜色（比如全局统一成蓝）就会响。
    const byMode = ringTokenByMode()
    const failures: string[] = []

    for (const [theme, vars] of themeBlocks()) {
      const ring = hsl(vars[byMode[theme.split('/')[1]]])
      const accent = hsl(vars.accent)
      // 灰阶主题（slate）的强调色本就无彩，色相是没有意义的量。
      //
      // ⚠ 边界说明，别让下一个人自己去推：这条豁免意味着**这条断言对 slate
      //   实际不设防**——两边都是灰、都走这一支。slate 那一格只靠上面那条
      //   结构断言在守。这是灰阶主题的固有情况，不是漏洞。
      if (ring.chroma < 0.08 && accent.chroma < 0.08) continue
      const d = hueDelta(ring.hue, accent.hue)
      if (d > 15) failures.push(`${theme}: 环色相 ${ring.hue.toFixed(0)}° 与强调色 ${accent.hue.toFixed(0)}° 差 ${d.toFixed(0)}°`)
    }

    expect(failures, '焦点环偏离了主题强调色的色相：\n' + failures.join('\n')).toEqual([])
  })

  it('焦点环对四种可能的底色都不低于 3:1', () => {
    // 四种底色分别是：页面底(bg)、卡片/列表行(surface)、次级面(bg-alt)、
    // 选中行(accent-wash)。列表行的焦点环是内缩的（outline-offset: -2px），
    // 直接压在行底色上，所以 accent-wash 这一项不是理论情况。
    const grounds = ['bg', 'surface', 'bg-alt', 'accent-wash'] as const
    const byMode = ringTokenByMode()
    const failures: string[] = []

    for (const [theme, vars] of themeBlocks()) {
      const mode = theme.split('/')[1]
      const token = byMode[mode]
      expect(token, `没有解析到 ${mode} 模式的 --focus-ring`).toBeTruthy()
      const ring = vars[token]
      expect(ring, `${theme} 缺少令牌 --${token}`).toBeTruthy()
      for (const g of grounds) {
        const ratio = contrast(ring, vars[g])
        if (ratio < MIN_NON_TEXT) {
          failures.push(`${theme} --${token} 对 --${g}: ${ratio.toFixed(2)}`)
        }
      }
    }

    expect(failures, `焦点环对比度不足（WCAG 1.4.11 要求 ${MIN_NON_TEXT}:1）：\n` + failures.join('\n')).toEqual([])
  })
})

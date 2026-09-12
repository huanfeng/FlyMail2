/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'
import { TONES } from './theme'

/**
 * 色板只有一份的守卫。
 *
 * 曾经有三份：`index.css` 的 `[data-theme][data-mode]` 令牌（权威）、
 * `SettingsDialog` 主题预览卡里的 54 个硬编码 hex、`lib/theme.ts` 的 `TONES.swatch`。
 * 改一次主题要改三处，必然漂移——而漂移了也没有任何东西会报错：
 * 预览卡显示的颜色与点下去真正生效的颜色不一致，只有肉眼能发现。
 *
 * 现在预览卡靠给自己挂 `data-theme`/`data-mode` 让令牌在那个子树内重新生效，
 * `TONES` 不再带颜色值。这几条测试钉住这个结构：
 * 令牌齐全（预览才有色）、源码里没有色板副本、按属性选择的那两条自洽。
 */

/**
 * index.css 的原文。
 *
 * 只能用 node:fs 读：vitest 默认 `css: false`，会把 CSS 模块 stub 成空——
 * `import css from '../index.css?raw'` 与 `import.meta.glob(..., '?raw')`
 * 实测都拿到空串，`?inline` 同样。空串会让下面的解析安静地得出「零个令牌块」，
 * 正是前几轮反复栽过的那种假绿灯，所以下面额外断言了一次文件非空。
 *
 * 三斜线引用只让这一个测试文件看得见 node 类型，应用代码的 tsconfig 保持不带。
 */
// 相对 vitest 的工作目录（frontend/）取，import.meta.url 在 vitest 下不是 file: 协议。
const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf-8')

/** 从 CSS 里解析出全部 `[data-theme="x"][data-mode="y"]` 块 */
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

/** src 下的源码（不含测试），用来确认色板没有第二份副本 */
const sources = import.meta.glob('../**/*.{ts,tsx}', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>


/** sRGB 相对亮度（WCAG 2.1 定义） */
function luminance(hex: string): number {
  const h = hex.replace('#', '')
  const ch = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255)
  const lin = ch.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]
}

/** 两色的对比度（1 ~ 21） */
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe('主题令牌', () => {
  const blocks = themeBlocks()

  it('读到了 index.css 本身', () => {
    // 读不到时上面的解析会得出 0 个块，下面每条断言照样失败，
    // 但失败信息会指向主题而不是指向「文件没读到」。这条让根因先说话。
    expect(css.length).toBeGreaterThan(10_000)
  })

  it('9 套色调 × 亮暗两套，每套都定义了主题卡依赖的令牌', () => {
    // 预览卡不写颜色，完全靠这几个令牌；缺一个就是一块透明或继承自外层的错误颜色，
    // 而那在界面上看起来只是「这套主题的预览有点怪」，不会有任何报错。
    const need = ['bg', 'bg-alt', 'accent', 'rule', 'rule-strong']
    for (const tone of TONES) {
      for (const mode of ['light', 'dark']) {
        const vars = blocks.get(`${tone.id}/${mode}`)
        expect(vars, `缺少 [data-theme="${tone.id}"][data-mode="${mode}"] 令牌块`).toBeDefined()
        for (const n of need) {
          expect(vars?.[n], `${tone.id}/${mode} 缺 --${n}`).toBeTruthy()
        }
      }
    }
    expect(blocks.size).toBe(TONES.length * 2)
  })

  it('源码里没有色板的第二份副本', () => {
    // 判据是「特征色」：只在一两套主题里出现的 hex。像 #ffffff（9 套亮色的 --surface）
    // 这种通用中性色排除在外——它出现在源码里不代表有人抄了色板。
    const count = new Map<string, number>()
    for (const vars of blocks.values()) {
      for (const v of Object.values(vars)) {
        for (const h of v.matchAll(/#[0-9a-fA-F]{6}/g)) {
          const k = h[0].toLowerCase()
          count.set(k, (count.get(k) ?? 0) + 1)
        }
      }
    }
    const signature = new Set([...count].filter(([, n]) => n <= 2).map(([h]) => h))
    expect(signature.size).toBeGreaterThan(50) // 判据本身没失效

    const offenders: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      if (/\.test\.tsx?$/.test(path)) continue
      for (const h of text.matchAll(/#[0-9a-fA-F]{6}/g)) {
        if (signature.has(h[0].toLowerCase())) offenders.push(`${path}: ${h[0]}`)
      }
    }
    expect(offenders, '主题色应只在 index.css 里定义，界面要预览就挂 data-theme/data-mode').toEqual([])
  })

  it('index.html 的 theme-color 等于默认调的底色', () => {
    // 色板的第四份副本——而且已经漂了：亮那个写的是 warm 的 #fbfaf7（默认调是 slate），
    // 暗那个 #1a1917 在整张 9×2 的令牌表里根本不存在。
    // 它没法走令牌（meta 在 CSS 变量之外，浏览器读它时引导脚本还没跑），
    // 所以只能靠这条测试把它和令牌绑在一起。
    //
    // 上面那条「源码里没有色板副本」盖不到这里：它的 glob 只扫 src 下的 ts/tsx。
    const html = readFileSync(resolve(process.cwd(), 'index.html'), 'utf-8')
    const pick = (scheme: string) =>
      new RegExp(
        `<meta name="theme-color" content="(#[0-9a-fA-F]{6})" media="\\(prefers-color-scheme: ${scheme}\\)"`,
      ).exec(html)?.[1]

    // 默认色调在三处必须一致：index.html 的引导脚本、lib/theme.ts 的 getTheme()、这里
    expect(html, 'index.html 的引导脚本不再默认 slate 了？').toContain("|| 'slate'")

    for (const [scheme, mode] of [
      ['light', 'light'],
      ['dark', 'dark'],
    ] as const) {
      const want = blocks.get(`slate/${mode}`)?.bg
      expect(pick(scheme)?.toLowerCase(), `theme-color(${scheme}) 应等于 slate/${mode} 的 --bg`).toBe(
        want?.toLowerCase(),
      )
    }
  })

  it('主按钮的底色与字色在 18 套主题下都达到 WCAG AA', () => {
    // 上一条禁的是一种**写法**，这一条钉的是**结果**——写法可以再变，4.5:1 不能破。
    //
    // --primary / --primary-foreground 的定义在文件顶部的 shadcn 桥里：
    // 亮色 = --accent-ink 配白字，暗色 = --accent 配 --bg。
    // 曾经登录按钮直接用 --accent 配死白字，18 组里有 17 组不到 4.5（最差 1.67）。
    const AA = 4.5
    const failures: string[] = []
    for (const [key, vars] of blocks) {
      const mode = key.split('/')[1]
      const bg = mode === 'light' ? vars['accent-ink'] : vars['accent']
      const fg = mode === 'light' ? '#ffffff' : vars['bg']
      expect(bg, `${key} 缺 --${mode === 'light' ? 'accent-ink' : 'accent'}`).toBeTruthy()
      const r = contrast(bg, fg)
      if (r < AA) failures.push(`${key}: ${bg} 配 ${fg} = ${r.toFixed(2)}`)
    }
    expect(failures, `主按钮对比度不足 ${AA}`).toEqual([])
  })

  it('没有哪个控件拿 --accent 当底色又写死白字', () => {
    // --accent 在暗色各调里是**亮调**色（slate 暗是 #b6bdc8），配死白字只有 1.9:1。
    // 文件顶部的 shadcn 令牌桥专为「品牌色作按钮底」分了亮暗两套
    // （亮色 --accent-ink 配白字、暗色 --accent 配深底色字），底色一律走 --primary。
    // 这条曾经在四个地方各犯一次：.brand-mark / .compose-btn / .pill-btn.primary /
    // 新写的 .login-submit——每一处单独看都「和旁边那个写法一致」。
    const offenders: string[] = []
    for (const m of css.matchAll(/([^{}/]+)\{([^}]*)\}/g)) {
      const body = m[2]
      if (/background:\s*var\(--accent\)/.test(body) && /color:\s*(white|#fff)/i.test(body)) {
        offenders.push(m[1].trim().split('\n').pop() ?? '')
      }
    }
    expect(offenders, '底色用 --accent 时请改用 var(--primary) / var(--primary-foreground)').toEqual([])
  })

  it('亮暗两套都按属性声明 color-scheme', () => {
    // color-scheme 是继承属性，子树挂 [data-mode="light"] 时若这条只写在 :root 上，
    // 就只会继承祖先的值。今天没有可见差异（预览卡的 mode 总等于当前模式），
    // 这条钉的是「挂上这两个属性就得到一整套主题」这个承诺本身。
    expect(css).toMatch(/\[data-mode="light"\]\s*\{[^}]*color-scheme:\s*light/)
    expect(css).toMatch(/\[data-mode="dark"\]\s*\{[^}]*color-scheme:\s*dark/)
  })
})

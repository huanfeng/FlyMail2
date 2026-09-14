/// <reference types="node" />
import { readdirSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 设置里按钮的风格对齐。
 *
 * 缘起：七个「＋ 新增」按钮长出了**三种**图标-文字间距写法——
 *
 *   `<Icon/> {文本}`                     一个空格（5 处）
 *   `<Icon/><span marginLeft:4>文本</span>`  4px（别名那处）
 *   `<Icon/>{文本}`                      完全贴死（「添加账户」）
 *
 * 根因不在这七处，而在 `.pill-btn` 本身：它当时没有 `display`，是普通行内流，
 * 所以间距只能靠各自手写。而行内流还有个更难看的后果——容器一窄，
 * 图标和文字会**换行**，按钮变成「＋」上面、标题下面两行。
 *
 * 现在间距由 `.pill-btn { display:inline-flex; gap:6px }` 统一提供，于是：
 *
 *   1. CSS 那三个声明成了必需品，掉一个就散架 → 下面第一组用例
 *   2. JSX 里**不能**再留手写空格：flex 会把空格文本节点当成独立 flex item，
 *      渲染成 gap + 空格 + gap，比原来还宽 → 下面第二组用例
 *
 * 这两条肉眼都很难在 review 里发现（差几个像素），所以钉成测试。
 */

const root = process.cwd()

const css = readFileSync(resolve(root, 'src/index.css'), 'utf-8').replace(
  // 剥注释。不剥的话本文件解释 `.pill-btn` 的那段注释会被当成真规则匹配上
  // （focus-ring.test.ts 的第一版就栽在这上面）。
  /\/\*[\s\S]*?\*\//g,
  '',
)

/** 取某个选择器的规则体（首个匹配）。 */
function ruleBody(selector: string): string {
  const re = new RegExp(`(?:^|[},])\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`, 'm')
  return re.exec(css)?.[1] ?? ''
}

describe('.pill-btn 的间距来源', () => {
  const body = ruleBody('.pill-btn')

  it('规则存在', () => {
    expect(body, '.pill-btn 规则没解析到，下面的断言全部失去意义').not.toBe('')
  })

  it('是 flex 容器——否则图标与文字之间没有任何间距来源', () => {
    expect(body).toMatch(/display:\s*inline-flex/)
    expect(body, '缺 align-items 时图标与文字基线对不齐').toMatch(/align-items:\s*center/)
  })

  it('有 gap——七处调用方都依赖它，没有一处自己写间距', () => {
    expect(body).toMatch(/gap:\s*\d/)
  })

  it('不换行——这是「＋」和标题分成两行的直接原因', () => {
    expect(body, '容器变窄时按钮会折成两行').toMatch(/white-space:\s*nowrap/)
  })
})

describe('设置里的按钮不再自己写间距', () => {
  const dir = resolve(root, 'src/components/settings')
  const files = readdirSync(dir).filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx'))

  it('扫到了文件', () => {
    expect(files.length).toBeGreaterThan(5)
  })

  for (const f of files) {
    const src = readFileSync(resolve(dir, f), 'utf-8')

    it(`${f}：Icon 与文本之间没有手写空格`, () => {
      // `<Icon … /> {` —— 中间那个空格在 JSX 里是**文本节点**，
      // flex 布局下它会占掉一整个 flex item 的位置（gap + 空格 + gap）。
      // 跨行的 `<Icon/>\n{t(…)}` 不在此列：含换行的空白 JSX 会整个丢弃。
      const hits = [...src.matchAll(/<Icon\b[^>]*\/>[ \t]+\{/g)]
      expect(
        hits.map((m) => m[0]),
        '图标后面跟了手写空格，与 gap 叠加成双份间距',
      ).toEqual([])
    })

    it(`${f}：没有用 marginLeft 给按钮文字拉间距`, () => {
      // 别名那处原来是 `<span style={{ marginLeft: 4 }}>`，与另外六处的间距
      // 差 2px——正是「有些按钮看起来不一样」的来源之一。
      const hits = [...src.matchAll(/<Icon\b[^>]*\/>\s*<span style=\{\{\s*marginLeft/g)]
      expect(hits.map((m) => m[0]), '又出现了手写间距').toEqual([])
    })
  }
})

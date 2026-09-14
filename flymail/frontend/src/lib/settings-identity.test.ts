/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeAll, describe, expect, it } from 'vitest'

/**
 * 设置里「身份行」（头像 + 名称 + 副行）的外观必须真的生效。
 *
 * ── 缘起 ─────────────────────────────────────────────────────────────────
 *
 * `.ac-avatar / .ac-name / .ac-mail` 这套类名在设置里被五个地方复用，
 * 而 CSS 规则当时限定在 `.account-card` 下。其中两处根本不在 `.account-card` 内：
 *
 *   「我的资料」  裸用       → 用户报的「头像有些异常」
 *   「监控」      在 .form-card 里 → 同一个病，顺带查出来的
 *
 * 这两处一条规则都匹配不上，于是只剩 TSX 里的内联尺寸：**直角**方块、首字母
 * **左上角**对齐（没有 place-items）、**深色字**压在 accent 底色上（没有 color:white）。
 *
 * ── 为什么这样测 ─────────────────────────────────────────────────────────
 *
 * 不钉选择器写法（`.sd-body .ac-avatar` 这串字符），钉**结果**：把真实的
 * index.css 注进 jsdom，按组件真实的 DOM 结构渲染，再问 getComputedStyle。
 * 换个作用域写法只要仍然生效就照样通过；而任何一次"收窄限定符"都会当场变红。
 *
 * 两组断言互相挂钩，缺一条就守不住：
 *   1. 结构：TSX 里确实是 `.settings-identity > .ac-avatar`
 *   2. 样式：这个结构在真实 CSS 下算出来是圆角/居中/白字
 * 只有 2 的话，改了 TSX 结构测试仍然绿；只有 1 的话，CSS 收窄了测试仍然绿。
 */

const root = process.cwd()
const css = readFileSync(resolve(root, 'src/index.css'), 'utf-8')
const settingsSrc = readFileSync(resolve(root, 'src/components/settings/SettingsDialog.tsx'), 'utf-8')

/** 在 .sd-body 下渲染一段结构，返回算好样式的元素。 */
function computed(html: string, sel: string): CSSStyleDeclaration {
  const host = document.createElement('div')
  host.className = 'sd-body'
  host.innerHTML = html
  document.body.appendChild(host)
  return getComputedStyle(host.querySelector(sel)!)
}

beforeAll(() => {
  const style = document.createElement('style')
  style.textContent = css
  document.head.appendChild(style)
})

describe('真实 CSS 在 jsdom 里确实加载了', () => {
  it('对照组：随便一条已知规则要算得出来', () => {
    // 没有这条对照，jsdom 若因为某个现代语法整段丢弃样式表，
    // 下面所有断言都会因为"什么都没匹配上"而以错误的理由失败——
    // 更坏的情况是某天有人把断言写成 `not.toBe('0px')` 之类，变成永远绿。
    const cs = computed('<div class="ac-name">x</div>', '.ac-name')
    expect(cs.fontWeight, 'index.css 没被 jsdom 解析，本文件的结论全部无效').toBe('500')
  })
})

describe('「我的资料」的头像', () => {
  // 与 SettingsDialog.tsx 里 ProfileSection 的结构保持一致
  const markup = `
    <div class="settings-identity">
      <div class="ac-avatar">F</div>
      <div class="ac-text">
        <div class="ac-name">admin</div>
        <div class="ac-mail">a@b.com</div>
      </div>
    </div>`

  it('TSX 里用的就是这个结构', () => {
    // 挂钩断言：结构一旦改了，下面基于 markup 的样式结论就不再代表真实界面
    expect(settingsSrc).toContain('className="settings-identity"')
    expect(settingsSrc).toContain('<div className="ac-avatar" aria-hidden="true">')
  })

  it('是圆角，不是直角方块', () => {
    expect(computed(markup, '.ac-avatar').borderRadius).not.toBe('')
    expect(computed(markup, '.ac-avatar').borderRadius).not.toBe('0px')
  })

  it('首字母居中，不是左上角对齐', () => {
    expect(computed(markup, '.ac-avatar').display, '不是 grid，place-items 无从生效').toBe('grid')
  })

  it('字是白的——底色是 accent，深色字在上面几乎看不清', () => {
    expect(computed(markup, '.ac-avatar').color).toBe('rgb(255, 255, 255)')
  })

  it('尺寸来自 CSS 而不是内联样式', () => {
    // 内联样式给得了尺寸，给不了形状。「尺寸内联 + 形状指望 CSS」这个组合
    // 正是当初看起来"只是有点怪"、查下去才发现整条规则没匹配上的原因。
    expect(computed(markup, '.ac-avatar').width).toBe('48px')

    // 只禁**尺寸**内联，不禁 background：账户卡那处内联的是每个账户各自的
    // 配色，那是数据驱动的、本来就该在 TSX 里。一刀切会把它一起判死。
    const inlineSize = [...settingsSrc.matchAll(/className="ac-avatar"[\s\S]{0,120}?style=\{\{([^}]*)\}\}/g)]
      .map((m) => m[1])
      .filter((s) => /\b(width|height|fontSize)\b/.test(s))
    expect(inlineSize, '又在用内联尺寸补 CSS 的缺').toEqual([])
  })
})

describe('「监控」里的身份行（同一个病，一起修）', () => {
  // 它在 .form-card 里，不在 .account-card 里
  const markup = `<div class="form-card"><div class="ac-avatar">M</div></div>`

  it('也能拿到圆角与居中', () => {
    const cs = computed(markup, '.ac-avatar')
    expect(cs.display).toBe('grid')
    expect(cs.borderRadius).not.toBe('0px')
  })
})

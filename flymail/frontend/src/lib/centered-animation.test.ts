/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 自己用 transform 居中的浮层，动画不能把那个 transform 顶掉。
 *
 * `transform` 是**整条**被关键帧替换的，不是各分量分别插值。`@keyframes pop`
 * 的末态是 `transform: none`，对靠父级 grid/flex 居中的浮层没问题——`pop` 的
 * 另外五个使用者（.settings-dialog / .notif-dialog / .compose-win /
 * .compose-bar / .shortcuts-card）都是那样，所以这个坑一直没碰上。
 *
 * 而 `.confirm-dialog` 是 `position: fixed; left/top: 50%` 再靠
 * `translate(-50%,-50%)` 拉回来的。套上 `pop` 之后，那 150ms 里 transform 被
 * 换成关键帧的值，居中偏移消失——对话框的左上角先落在视口正中，动画结束才
 * 跳回去，肉眼可见。
 *
 * 这条测试把「谁用 transform 居中」和「它的动画末态保不保留那个 transform」
 * 对起来查，不依赖布局引擎。
 */
const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf-8')

/** 取出某个选择器的声明块（只取第一个匹配，够用：这些选择器各只出现一次） */
function ruleBody(selector: string): string {
  const re = new RegExp(`(^|\\})\\s*${selector.replace('.', '\\.')}\\s*\\{`, 'm')
  const m = re.exec(css)
  expect(m, `找不到规则 ${selector}`).not.toBeNull()
  const open = css.indexOf('{', m!.index + m![0].length - 1)
  const close = css.indexOf('}', open)
  return css.slice(open + 1, close)
}

/** 取出某个 @keyframes 的整段文本 */
function keyframes(name: string): string {
  const start = css.indexOf(`@keyframes ${name}`)
  expect(start, `找不到 @keyframes ${name}`).toBeGreaterThan(-1)
  const open = css.indexOf('{', start)
  let depth = 0
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}') {
      depth--
      if (depth === 0) return css.slice(open + 1, i)
    }
  }
  throw new Error(`@keyframes ${name} 没有闭合`)
}

describe('用 transform 居中的浮层与它的入场动画', () => {
  it('.confirm-dialog 确实靠 translate(-50%,-50%) 居中', () => {
    // 这是下一条断言成立的前提。哪天改成 grid 居中了，下一条就该一并放宽。
    expect(ruleBody('.confirm-dialog')).toContain('translate(-50%, -50%)')
  })

  it('它用的关键帧末态保留了那个居中偏移', () => {
    const body = ruleBody('.confirm-dialog')
    const anim = /animation:\s*([\w-]+)/.exec(body)
    expect(anim, '.confirm-dialog 没有 animation 声明').not.toBeNull()

    const frames = keyframes(anim![1])
    const to = /(?:to|100%)\s*\{([^}]*)\}/.exec(frames)
    expect(to, `@keyframes ${anim![1]} 没有 to/100% 帧`).not.toBeNull()
    expect(
      to![1],
      `@keyframes ${anim![1]} 的末态把居中的 transform 顶掉了——` +
        '对话框会先在视口正中露出左上角再跳回来',
    ).toContain('translate(-50%, -50%)')
  })

  it('pop 仍然是给「靠父级居中」的浮层用的，末态归零不算 bug', () => {
    // 记录这条判据的适用边界：pop 本身没问题，问题是谁用它。
    expect(keyframes('pop')).toContain('transform: none')
  })
})

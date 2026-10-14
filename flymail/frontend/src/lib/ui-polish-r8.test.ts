/// <reference types="node" />
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve as resolvePath } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * 第八轮 UI 打磨的 CSS 结构性约束（设计见 docs/flymail/ui-polish-r8.md）。
 *
 * 这些断言钉的是「这条规则存在且落在正确的媒体/容器块里」，不是像素；
 * 像素由 Task 14 的截图回归负责。
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

describe('R8-1 容器查询基建与搜索框 kbd', () => {
  it('列表栏与阅读区声明为容器', () => {
    expect(block('.col.list')).toMatch(/container:\s*list\s*\/\s*inline-size/)
    expect(block('.col.reader')).toMatch(/container:\s*reader\s*\/\s*inline-size/)
    expect(block('.reader-slide')).toMatch(/container:\s*reader\s*\/\s*inline-size/)
  })
  it('搜索框 kbd 不折行，且输入框可收缩让位', () => {
    expect(block('.search-input .kbd')).toMatch(/white-space:\s*nowrap/)
    // <input> 的固有最小宽度（size=20 ≈ 150px）会把 kbd 挤出框外，必须允许它收缩
    expect(block('.search-input input')).toMatch(/min-width:\s*0/)
  })
  it('粗指针下 kbd 提示一律隐藏（搜索框与写邮件按钮）', () => {
    const m = stripped.match(/@media\s*\(pointer:\s*coarse\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.search-input \.kbd[^{]*\{[^}]*display:\s*none/)
    expect(m![1]).toMatch(/\.compose-btn \.kbd[^{]*\{[^}]*display:\s*none/)
  })
  it('窄列表容器下只隐藏 kbd，不动清除按钮', () => {
    const m = stripped.match(/@container list \(max-width: 340px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.search-input \.kbd\s*\{[^}]*display:\s*none/)
    expect(m![1]).not.toMatch(/icon-btn/)
  })
})

describe('R8-2 阅读区工具条图标模式', () => {
  it('窄阅读容器下隐藏按钮文字', () => {
    const m = stripped.match(/@container reader \(max-width: 560px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.tb-btn > span\s*\{[^}]*display:\s*none/)
  })
})

describe('R8-3 会话卡窄容器', () => {
  it('480px 以下隐藏文件夹 chip', () => {
    const m = stripped.match(/@container reader \(max-width: 480px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.ti-folder\s*\{[^}]*display:\s*none/)
  })
})

describe('R8-5 健康横幅内联', () => {
  it('不再固定定位，无 z-index', () => {
    const b = block('.health-banner')
    expect(b).not.toMatch(/position:\s*fixed/)
    expect(b).not.toMatch(/z-index/)
    expect(b).toMatch(/border-bottom/)
  })
  it('列表栈撑满列栏', () => {
    expect(block('.list-stack')).toMatch(/flex-direction:\s*column/)
    expect(block('.list-stack-main')).toMatch(/min-height:\s*0/)
  })
})

describe('R8-6 移动端写邮件 FAB', () => {
  it('桌面隐藏、768 以下显示且为可触达尺寸', () => {
    expect(block('.compose-fab')).toMatch(/display:\s*none/)
    const m = stripped.match(/@media \(max-width: 768px\)\s*\{([\s\S]*?)\n\}/g) ?? []
    const hit = m.find((blk) => /\.compose-fab\s*\{[^}]*display:\s*(flex|grid)/.test(blk))
    expect(hit).toBeTruthy()
    expect(hit!).toMatch(/\.compose-fab\s*\{[^}]*(width|height):\s*56px/)
  })
})

describe('R8-8 头像色相令牌', () => {
  it('8 个令牌齐全且白字对比 ≥ 4.5', () => {
    for (let i = 1; i <= 8; i++) {
      const m = stripped.match(new RegExp(`--avatar-${i}:\\s*oklch\\(([\\d.]+)\\s+([\\d.]+)\\s+([\\d.]+)\\)`))
      expect(m, `--avatar-${i}`).not.toBeNull()
      // oklch 的 L 到相对亮度：Y ≈ L³（中性色精确成立；C 0.11 时按 OKLab→sRGB 实算误差约 0.015，
      // 8 个色相实算对比在 5.1~5.8 之间，对 4.5 门槛留有余量；hue 210 略出 sRGB 色域，由浏览器做色域映射）
      const L = Number(m![1])
      const Y = L * L * L
      const contrast = (1.0 + 0.05) / (Y + 0.05)
      expect(contrast, `--avatar-${i} 白字对比`).toBeGreaterThanOrEqual(4.5)
      expect(stripped).toMatch(new RegExp(`\\.avatar-sq\\[data-hue="${i}"\\]`))
    }
  })
})

describe('R8-10 断点收口', () => {
  it('只剩 1024 / 768 / 480 三档', () => {
    const widths = Array.from(stripped.matchAll(/@media[^{]*max-width:\s*(\d+)px/g)).map((m) => Number(m[1]))
    expect(new Set(widths)).toEqual(new Set([1024, 768, 480]))
  })
  it('1024 档收窄侧栏与列表给阅读区让位', () => {
    const m = stripped.match(/@media \(max-width: 1024px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.col\.sidebar\s*\{[^}]*min\(var\(--sidebar-w\),\s*200px\)/)
    expect(m![1]).toMatch(/\.col\.list\s*\{[^}]*min\(var\(--list-w\),\s*340px\)/)
  })
})

describe('R8-4 文件夹显示名单一出口', () => {
  it('组件里不再直接拼 t(`folder.${type}`)——都走 folderLabel/folderLabels', () => {
    const root = resolvePath(__dirname, '../components')
    const hits: string[] = []
    const walk = (dir: string) => {
      for (const name of readdirSync(dir)) {
        const p = `${dir}/${name}`
        if (statSync(p).isDirectory()) walk(p)
        else if (/\.tsx?$/.test(name) && !/\.test\./.test(name)) {
          const src = readFileSync(p, 'utf8')
          if (/t\(`folder\.\$\{/.test(src)) hits.push(p.slice(root.length + 1))
        }
      }
    }
    walk(root)
    expect(hits).toEqual([])
  })
})

describe('R8-6b 移动端 FAB 不盖住最后一行', () => {
  it('768 以下列表滚动区底部留白 ≥ 80px', () => {
    const blocks = stripped.match(/@media \(max-width: 768px\)\s*\{([\s\S]*?)\n\}/g) ?? []
    const hit = blocks.find((b) => /\.mail-list\s*\{[^}]*padding-bottom:\s*(\d+)px/.test(b))
    expect(hit).toBeTruthy()
    const px = Number(/\.mail-list\s*\{[^}]*padding-bottom:\s*(\d+)px/.exec(hit!)![1])
    expect(px).toBeGreaterThanOrEqual(80)
  })
})

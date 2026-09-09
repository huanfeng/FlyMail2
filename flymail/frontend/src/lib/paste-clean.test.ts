import { describe, it, expect } from 'vitest'
import { cleanPastedHtml, filterStyle } from '@/lib/paste-clean'

// 这一层的验收点只有两条，缺一不可：
//   保住格式 —— 从 Outlook / Gmail 粘过来的字号、颜色不能丢；
//   丢掉垃圾 —— mso-* / <o:p> / 条件注释不能跟着邮件发出去。
// 单看任何一条都能写出"通过"的实现（全留 or 全删），所以每组用例都两头都断言。

describe('filterStyle', () => {
  it('保留白名单里的声明', () => {
    expect(filterStyle('color: #ff0000; font-size: 14pt')).toBe('color: #ff0000; font-size: 14pt')
  })

  it('丢掉 mso-* 私有属性', () => {
    const out = filterStyle('mso-fareast-font-family: 等线; color: red; mso-ansi-language: EN-US')
    expect(out).toBe('color: red')
    expect(out).not.toContain('mso-')
  })

  it('丢掉 CSS 表达式与 javascript: 取值', () => {
    expect(filterStyle('color: expression(alert(1))')).toBe('')
    expect(filterStyle('background-color: javascript:alert(1)')).toBe('')
  })

  it('丢掉不在白名单里的排版属性（position/width 等）', () => {
    expect(filterStyle('position: absolute; width: 999px; color: blue')).toBe('color: blue')
  })
})

describe('cleanPastedHtml — Outlook 富文本', () => {
  // 典型的 Word/Outlook 剪贴板产物：条件注释 + <o:p> + 一坨 mso-* + class
  const OUTLOOK = `
    <!--[if gte mso 9]><xml><w:WordDocument><w:View>Normal</w:View></w:WordDocument></xml><![endif]-->
    <style><!-- p.MsoNormal { mso-style-parent: ""; font-size: 10.0pt; } --></style>
    <p class="MsoNormal" style="margin:0cm;mso-pagination:widow-orphan">
      <span lang="EN-US" style="font-size:14.0pt;color:#FF0000;mso-fareast-font-family:等线">
        重要通知
      </span>
      <o:p></o:p>
    </p>`

  const cleaned = cleanPastedHtml(OUTLOOK)

  it('保留字号与颜色', () => {
    expect(cleaned).toContain('font-size: 14.0pt')
    expect(cleaned).toContain('color: #FF0000')
  })

  it('保留正文文字', () => {
    expect(cleaned).toContain('重要通知')
  })

  it('不含 mso-* 私有属性', () => {
    expect(cleaned).not.toContain('mso-')
  })

  it('不含 <o:p> 等命名空间标签', () => {
    expect(cleaned.toLowerCase()).not.toContain('<o:p')
    expect(cleaned.toLowerCase()).not.toContain('<w:')
  })

  it('不含条件注释与 <style> 块内容', () => {
    expect(cleaned).not.toContain('<!--')
    expect(cleaned).not.toContain('MsoNormal { ')
    expect(cleaned).not.toContain('<style')
  })

  it('丢掉 class / lang 属性', () => {
    expect(cleaned).not.toContain('class=')
    expect(cleaned).not.toContain('lang=')
  })
})

describe('cleanPastedHtml — 老式标签与危险内容', () => {
  it('<font> 翻译成 span[style]，颜色/字体/字号都不丢', () => {
    const out = cleanPastedHtml('<font color="#00ff00" face="Arial" size="5">大字</font>')
    expect(out).toContain('color: #00ff00')
    expect(out).toContain('font-family: Arial')
    expect(out).toContain('font-size: 24px')
    expect(out.toLowerCase()).not.toContain('<font')
    expect(out).toContain('大字')
  })

  it('align 属性翻译成 text-align', () => {
    expect(cleanPastedHtml('<p align="center">居中</p>')).toContain('text-align: center')
  })

  it('删掉 <script> 及其内容', () => {
    const out = cleanPastedHtml('<p>正文</p><script>alert(1)</script>')
    expect(out).toContain('正文')
    expect(out).not.toContain('alert(1)')
  })

  it('去掉 javascript: 链接但保留链接文字', () => {
    const out = cleanPastedHtml('<a href="javascript:alert(1)">点我</a>')
    expect(out).not.toContain('javascript:')
    expect(out).toContain('点我')
  })

  it('保留 https 链接与图片地址', () => {
    const out = cleanPastedHtml('<a href="https://x.com/a">x</a><img src="https://x.com/1.png">')
    expect(out).toContain('https://x.com/a')
    expect(out).toContain('https://x.com/1.png')
  })

  it('保留表格结构与合并单元格属性', () => {
    const out = cleanPastedHtml('<table><tr><td colspan="2">格</td></tr></table>')
    expect(out).toContain('<td')
    expect(out).toContain('colspan="2"')
  })

  it('拆掉不带任何属性的空 span 壳子', () => {
    expect(cleanPastedHtml('<span><span>文字</span></span>')).toBe('文字')
  })

  it('空输入返回空串', () => {
    expect(cleanPastedHtml('')).toBe('')
  })
})

// schema 级集成测试：把内容真正灌进 ProseMirror，再 getHTML 出来。
//
// 为什么值得单独测：cleanPastedHtml 只保证"清洗后的字符串里还有 font-size"，
// 但真正决定用户看到什么的是 schema——span[style] 如果没被 TextStyle 认领，
// 清洗得再干净也会在 parse 这一步被整个丢掉。M13 验收标准第一条
// 「从 Outlook / Gmail 粘贴的富文本保留格式」考的就是这两步的**串联**。

import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { composerExtensions } from '@/components/mail/composer/schema'
import { buildForward, buildReply } from '@/lib/compose-prefill'
import { cleanPastedHtml } from '@/lib/paste-clean'
import { wrapSignature } from '@/lib/signature'
import type { MessageDetail } from '@/lib/types'

let current: Editor | null = null

/** 灌一段 HTML 进编辑器，返回它序列化回来的 HTML */
function roundTrip(html: string): string {
  current?.destroy()
  current = new Editor({
    element: document.createElement('div'),
    extensions: composerExtensions(),
    content: html,
  })
  return current.getHTML()
}

afterEach(() => {
  current?.destroy()
  current = null
})

function detail(over: Partial<MessageDetail> = {}): MessageDetail {
  return {
    id: 1, account_id: 1, folder_id: 1, uid: 1,
    subject: '季度报告', from_name: '张三', from_addr: 'z@x.com',
    to: [], date: '2026-06-01', size: 1, seen: true, flagged: false,
    has_attachment: false, snippet: '',
    text_body: '', html_body: '<p>原文正文</p>',
    attachments: [], body_synced: true,
    message_id: '<m1@x.com>', references: '',
    remote_count: 0, remote_allowed: false,
    ...over,
  }
}

describe('粘贴保真（清洗 + schema 串联）', () => {
  const OUTLOOK = `
    <!--[if gte mso 9]><xml><w:WordDocument/></xml><![endif]-->
    <p class="MsoNormal" style="mso-pagination:widow-orphan">
      <span lang="EN-US" style="font-size:18px;color:#ff0000;mso-fareast-font-family:等线">红色大字</span>
      <o:p></o:p>
    </p>`

  const out = roundTrip(cleanPastedHtml(OUTLOOK))

  it('字号活着走完了整条链路', () => {
    expect(out).toContain('font-size: 18px')
  })

  it('颜色活着走完了整条链路', () => {
    // 浏览器会把颜色归一成 rgb()，这是 DOM 的行为不是丢失
    expect(out).toContain('color: rgb(255, 0, 0)')
  })

  it('文字没丢', () => {
    expect(out).toContain('红色大字')
  })

  it('mso 私有属性没跟进来', () => {
    expect(out).not.toContain('mso-')
    expect(out.toLowerCase()).not.toContain('<o:p')
  })

  it('高亮（背景色）也能保留', () => {
    const r = roundTrip(cleanPastedHtml('<span style="background-color:#fff3a3">标注</span>'))
    expect(r).toContain('background-color: rgb(255, 243, 163)')
    expect(r).toContain('标注')
  })

  it('加粗 / 下划线 / 删除线用 style 表达时同样识别', () => {
    const r = roundTrip(cleanPastedHtml(
      '<p><span style="font-weight:700">粗</span>' +
      '<span style="text-decoration:underline">下</span>' +
      '<span style="font-style:italic">斜</span></p>',
    ))
    expect(r).toMatch(/<strong>粗<\/strong>/)
    expect(r).toMatch(/<u>下<\/u>/)
    expect(r).toMatch(/<em>斜<\/em>/)
  })

  it('表格结构进得去也出得来', () => {
    const r = roundTrip(cleanPastedHtml('<table><tr><td>甲</td><td>乙</td></tr></table>'))
    expect(r).toContain('<table')
    expect(r).toContain('甲')
    expect(r).toContain('乙')
  })

  it('schema 丢弃它不认识的节点（比如 iframe）', () => {
    const r = roundTrip(cleanPastedHtml('<p>正文</p><iframe src="https://evil"></iframe>'))
    expect(r).toContain('正文')
    expect(r).not.toContain('iframe')
  })
})

describe('引用块', () => {
  it('回复预填被解析成 quoteBlock，且序列化回来仍带标记', () => {
    const r = roundTrip(buildReply(detail()).bodyHtml as string)
    expect(r).toContain('data-quote-block="true"')
    expect(r).toContain('原文正文')
  })

  it('引用内容完整保留（发送时无论展开与否都要带上）', () => {
    const r = roundTrip(buildReply(detail({ html_body: '<p>第一段</p><p>第二段</p>' })).bodyHtml as string)
    expect(r).toContain('第一段')
    expect(r).toContain('第二段')
  })

  it('引用头里的 <地址> 不会被当成标签吃掉', () => {
    const r = roundTrip(buildReply(detail()).bodyHtml as string)
    expect(r).toContain('z@x.com')
  })

  it('转发预填同样进 quoteBlock', () => {
    const r = roundTrip(buildForward(detail()).bodyHtml as string)
    expect(r).toContain('data-quote-block="true"')
    expect(r).toContain('原文正文')
  })

  it('普通 blockquote 不会被误判成可折叠引用', () => {
    const r = roundTrip('<blockquote><p>普通引用</p></blockquote>')
    expect(r).toContain('普通引用')
    expect(r).not.toContain('data-quote-block')
  })
})

describe('签名块', () => {
  it('signature 节点能往返', () => {
    const r = roundTrip(`<p>正文</p>${wrapSignature('<p>—— 小明</p>')}`)
    expect(r).toContain('data-signature="true"')
    expect(r).toContain('—— 小明')
    expect(r).toContain('正文')
  })

  it('签名里的内联图（data: URI）不被 schema 丢掉', () => {
    const png = 'data:image/png;base64,iVBORw0KGgo='
    const r = roundTrip(wrapSignature(`<p><img src="${png}"></p>`))
    expect(r).toContain(png)
  })
})

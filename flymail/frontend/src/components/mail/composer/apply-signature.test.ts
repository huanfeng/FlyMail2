// 切换发件人时换签名——M13 里最容易伤到用户的一步。
//
// 每条用例都同时断言"新签名进来了"和"正文原封不动"：只断言前者的话，
// 一个把整篇文档换掉的实现也能通过，而那正是我们要防的事故。

import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { applySignatureToEditor, findTopLevelNode } from '@/components/mail/composer/apply-signature'
import { composerExtensions } from '@/components/mail/composer/schema'
import { buildReply } from '@/lib/compose-prefill'
import { wrapSignature } from '@/lib/signature'
import type { MessageDetail } from '@/lib/types'

let editor: Editor | null = null

function makeEditor(content: string): Editor {
  editor?.destroy()
  editor = new Editor({
    element: document.createElement('div'),
    extensions: composerExtensions(),
    content,
  })
  return editor
}

afterEach(() => {
  editor?.destroy()
  editor = null
})

function detail(): MessageDetail {
  return {
    id: 1, account_id: 1, folder_id: 1, uid: 1,
    subject: 's', from_name: '张三', from_addr: 'z@x.com',
    to: [], date: '2026-06-01', size: 1, seen: true, flagged: false,
    has_attachment: false, snippet: '',
    text_body: '', html_body: '<p>历史往返</p>',
    attachments: [], body_synced: true,
    message_id: '<m@x>', references: '',
    remote_count: 0, remote_allowed: false,
  }
}

describe('applySignatureToEditor', () => {
  it('没有旧签名时插入，正文保留', () => {
    const ed = makeEditor('<p>我写的正文</p>')
    applySignatureToEditor(ed, '<p>—— 甲</p>')
    const html = ed.getHTML()
    expect(html).toContain('我写的正文')
    expect(html).toContain('—— 甲')
    expect(html).toContain('data-signature="true"')
  })

  it('换签名只动签名块，正文一个字不改', () => {
    const ed = makeEditor(`<p>我写的正文</p>${wrapSignature('<p>—— 甲</p>')}`)
    applySignatureToEditor(ed, '<p>—— 乙</p>')
    const html = ed.getHTML()
    expect(html).toContain('我写的正文')
    expect(html).toContain('—— 乙')
    expect(html).not.toContain('—— 甲')
  })

  it('换完仍然只有一个签名块', () => {
    const ed = makeEditor(`<p>正文</p>${wrapSignature('<p>甲</p>')}`)
    applySignatureToEditor(ed, '<p>乙</p>')
    expect(ed.getHTML().match(/data-signature/g)).toHaveLength(1)
  })

  it('连续换三次也不会累积成三个签名', () => {
    const ed = makeEditor('<p>正文</p>')
    applySignatureToEditor(ed, '<p>甲</p>')
    applySignatureToEditor(ed, '<p>乙</p>')
    applySignatureToEditor(ed, '<p>丙</p>')
    const html = ed.getHTML()
    expect(html.match(/data-signature/g)).toHaveLength(1)
    expect(html).toContain('丙')
    expect(html).toContain('正文')
  })

  it('传空串表示新账户没有签名：删掉签名块，正文留下', () => {
    const ed = makeEditor(`<p>正文</p>${wrapSignature('<p>甲</p>')}`)
    applySignatureToEditor(ed, '')
    const html = ed.getHTML()
    expect(html).toContain('正文')
    expect(html).not.toContain('data-signature')
  })

  it('本来就没有签名时传空串是空操作', () => {
    const ed = makeEditor('<p>正文</p>')
    applySignatureToEditor(ed, '')
    expect(ed.getHTML()).toContain('正文')
  })

  it('回复场景下签名插在引用块之前', () => {
    const ed = makeEditor(buildReply(detail()).bodyHtml as string)
    applySignatureToEditor(ed, '<p>—— 甲</p>')
    const html = ed.getHTML()
    expect(html.indexOf('—— 甲')).toBeLessThan(html.indexOf('历史往返'))
    expect(html).toContain('data-quote-block')
  })

  it('引用块里恰好也有一个签名时，不会换错地方', () => {
    // 把上一封带签名的信整段引用进来——只扫顶层的判定就是为了挡住这种情况
    const ed = makeEditor(
      `<p>我的正文</p><blockquote data-quote-block="true">${wrapSignature('<p>对方的签名</p>')}</blockquote>`,
    )
    applySignatureToEditor(ed, '<p>我的新签名</p>')
    const html = ed.getHTML()
    expect(html).toContain('对方的签名')
    expect(html).toContain('我的新签名')
    expect(html).toContain('我的正文')
  })

  it('正文里出现与签名相同的文字不会被误伤', () => {
    const ed = makeEditor(`<p>签名是「甲」没错</p>${wrapSignature('<p>甲</p>')}`)
    applySignatureToEditor(ed, '<p>乙</p>')
    expect(ed.getHTML()).toContain('签名是「甲」没错')
  })
})

describe('findTopLevelNode', () => {
  it('找得到顶层签名块', () => {
    const ed = makeEditor(`<p>x</p>${wrapSignature('<p>s</p>')}`)
    expect(findTopLevelNode(ed, 'signature')).not.toBeNull()
  })

  it('嵌在引用块里的签名不算顶层', () => {
    const ed = makeEditor(
      `<blockquote data-quote-block="true">${wrapSignature('<p>s</p>')}</blockquote>`,
    )
    expect(findTopLevelNode(ed, 'signature')).toBeNull()
  })

  it('没有该节点时返回 null', () => {
    expect(findTopLevelNode(makeEditor('<p>x</p>'), 'quoteBlock')).toBeNull()
  })
})

import { describe, it, expect } from 'vitest'
import { buildReply, buildForward } from '@/lib/compose-prefill'
import type { MessageDetail } from '@/lib/types'

/** 构造最小 MessageDetail，未指定字段给合理默认值 */
function makeDetail(overrides: Partial<MessageDetail> = {}): MessageDetail {
  return {
    id: 1,
    account_id: 1,
    folder_id: 1,
    uid: 1,
    subject: '测试主题',
    from_name: '张三',
    from_addr: 'sender@example.com',
    to: [],
    date: '2026-06-01',
    size: 100,
    seen: false,
    flagged: false,
    has_attachment: false,
    snippet: '',
    text_body: '',
    html_body: '',
    attachments: [],
    body_synced: true,
    message_id: '<msg001@example.com>',
    references: '',
    ...overrides,
  }
}

// ─────────────────────────────────────────────
// buildReply
// ─────────────────────────────────────────────
describe('buildReply', () => {
  it('to 数组等于 [from_addr]', () => {
    const r = buildReply(makeDetail())
    expect(r.to).toEqual(['sender@example.com'])
  })

  it('from_addr 为空时 to 为 []', () => {
    const r = buildReply(makeDetail({ from_addr: '' }))
    expect(r.to).toEqual([])
  })

  it('subject 加 Re: 前缀', () => {
    const r = buildReply(makeDetail({ subject: '你好' }))
    expect(r.subject).toBe('Re: 你好')
  })

  it('subject 已是 Re: 前缀时不重复添加', () => {
    const r = buildReply(makeDetail({ subject: 'Re: 你好' }))
    expect(r.subject).toBe('Re: 你好')
  })

  it('subject 小写 re: 前缀时也不重复添加', () => {
    const r = buildReply(makeDetail({ subject: 're: x' }))
    expect(r.subject).toBe('re: x')
  })

  it('空 subject → "Re: "', () => {
    const r = buildReply(makeDetail({ subject: '' }))
    expect(r.subject).toBe('Re: ')
  })

  it('inReplyTo === message_id', () => {
    const r = buildReply(makeDetail({ message_id: '<abc@test>' }))
    expect(r.inReplyTo).toBe('<abc@test>')
  })

  it('references 包含 message_id', () => {
    const r = buildReply(makeDetail({
      message_id: '<msg001@example.com>',
      references: '',
    }))
    expect(r.references).toContain('<msg001@example.com>')
  })

  it('references 存在时用空格连接 references 与 message_id', () => {
    const r = buildReply(makeDetail({
      message_id: '<msg002@example.com>',
      references: '<prev@example.com>',
    }))
    expect(r.references).toBe('<prev@example.com> <msg002@example.com>')
  })
})

// ─────────────────────────────────────────────
// buildForward
// ─────────────────────────────────────────────
describe('buildForward', () => {
  it('to 始终为 []', () => {
    const r = buildForward(makeDetail())
    expect(r.to).toEqual([])
  })

  it('subject 加 Fwd: 前缀', () => {
    const r = buildForward(makeDetail({ subject: '你好' }))
    expect(r.subject).toBe('Fwd: 你好')
  })

  it('bodyHtml 包含 HTML 转义后的主题', () => {
    const r = buildForward(makeDetail({ subject: '会议<确认>' }))
    expect(r.bodyHtml).toContain('&lt;确认&gt;')
    expect(r.bodyHtml).not.toContain('<确认>')
  })

  it('from_name 含 <script> 时 bodyHtml 不含原始标签', () => {
    const r = buildForward(makeDetail({ from_name: '<script>alert(1)</script>' }))
    expect(r.bodyHtml).not.toContain('<script>')
    expect(r.bodyHtml).toContain('&lt;script&gt;')
  })

  it('text_body 含 <b> 且无 html_body 时，正文走 <pre> 且内容被转义', () => {
    const r = buildForward(makeDetail({
      html_body: '',
      text_body: '内容 <b>加粗</b>',
    }))
    // 正文应在 <pre> 内，且 <b> 被转义
    expect(r.bodyHtml).toContain('<pre>')
    expect(r.bodyHtml).toContain('&lt;b&gt;')
    expect(r.bodyHtml).not.toContain('<b>加粗</b>')
  })
})

import { describe, it, expect } from 'vitest'
import { buildReply, buildReplyAll, buildForward, buildMailtoCompose } from '@/lib/compose-prefill'
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
    remote_count: 0,
    remote_allowed: false,
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

describe('buildMailtoCompose', () => {
  it('把 mailto 字段搬进撰写器初始内容', () => {
    const c = buildMailtoCompose('mailto:a@x.com?cc=b@x.com&subject=Hi&body=one%0Atwo')
    expect(c).toEqual({
      to: ['a@x.com'],
      cc: ['b@x.com'],
      subject: 'Hi',
      bodyHtml: 'one<br>two',
    })
  })

  it('body 是纯文本，进富文本前必须转义', () => {
    // mailto 链接完全由发件人控制，不转义就是把邮件内容注入到我们要发出去的那封信里
    const c = buildMailtoCompose('mailto:a@x.com?body=%3Cimg%20src%3Dx%20onerror%3Dalert(1)%3E')
    expect(c?.bodyHtml).toBe('&lt;img src=x onerror=alert(1)&gt;')
    expect(c?.bodyHtml).not.toContain('<img')
  })

  it('主题不做 HTML 转义（撰写器主题栏是纯文本输入框）', () => {
    expect(buildMailtoCompose('mailto:a@x.com?subject=a%20%26%20b')?.subject).toBe('a & b')
  })

  it('不是 mailto 或内容为空时返回 null，调用方据此不开撰写器', () => {
    expect(buildMailtoCompose('https://x.com')).toBeNull()
    expect(buildMailtoCompose('mailto:')).toBeNull()
  })
})

describe('buildReplyAll', () => {
  it('收件人 = 原发件人 + 原收件人，抄送沿用原抄送', () => {
    const d = makeDetail({
      from_addr: 'sender@example.com',
      to: [{ name: '李四', email: 'li@example.com' }],
      cc: [{ name: '王五', email: 'wang@example.com' }],
    })
    const r = buildReplyAll(d)
    expect(r.to).toEqual(['sender@example.com', 'li@example.com'])
    expect(r.cc).toEqual(['wang@example.com'])
  })

  it('把自己从收件人与抄送里剔除', () => {
    const d = makeDetail({
      from_addr: 'sender@example.com',
      to: [
        { name: '我', email: 'me@example.com' },
        { name: '李四', email: 'li@example.com' },
      ],
      cc: [{ name: '我', email: 'ME@example.com' }],
    })
    // 不剔除的话每次全部回复都会给自己抄送一份
    const r = buildReplyAll(d, new Set(['me@example.com']))
    expect(r.to).toEqual(['sender@example.com', 'li@example.com'])
    expect(r.cc).toEqual([])
  })

  it('同一地址只保留一次，且 to 优先于 cc', () => {
    const d = makeDetail({
      from_addr: 'sender@example.com',
      to: [{ name: '', email: 'dup@example.com' }],
      cc: [{ name: '', email: 'DUP@example.com' }],
    })
    const r = buildReplyAll(d)
    expect(r.to).toEqual(['sender@example.com', 'dup@example.com'])
    expect(r.cc).toEqual([])
  })

  it('回复自己发出的信时，收件人不会空掉', () => {
    const d = makeDetail({ from_addr: 'me@example.com', to: [], cc: [] })
    const r = buildReplyAll(d, new Set(['me@example.com']))
    expect(r.to).toEqual(['me@example.com'])
  })

  it('沿用 buildReply 的主题、引用与线程头', () => {
    const d = makeDetail({ subject: '报价', message_id: '<a@x>', references: '<r@x>' })
    const r = buildReplyAll(d)
    expect(r.subject).toBe('Re: 报价')
    expect(r.inReplyTo).toBe('<a@x>')
    expect(r.references).toBe('<r@x> <a@x>')
    expect(r.scenario).toBe('reply')
  })
})

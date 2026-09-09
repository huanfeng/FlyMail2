import { parseMailto } from '@/lib/mailto'
import type { MessageDetail } from '@/lib/types'
import type { ComposeInitial } from '@/components/mail/ComposeDialog'

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function quoteHeader(d: MessageDetail): string {
  const who = d.from_name ? `${d.from_name} <${d.from_addr}>` : d.from_addr
  return `在 ${d.date} ，${who} 写道：`
}

function originalBody(d: MessageDetail): string {
  return d.html_body || (d.text_body ? `<pre>${escapeHtml(d.text_body)}</pre>` : '')
}

function rePrefix(subject: string, p: string): string {
  return new RegExp('^' + p, 'i').test(subject.trim()) ? subject : `${p}${subject}`
}

export function buildReply(d: MessageDetail): ComposeInitial {
  return {
    to: d.from_addr ? [d.from_addr] : [],
    subject: rePrefix(d.subject || '', 'Re: '),
    bodyHtml: `<br><br><blockquote style="border-left:2px solid #ccc;padding-left:10px;color:#666">${quoteHeader(d)}<br>${originalBody(d)}</blockquote>`,
    inReplyTo: d.message_id,
    references: [d.references, d.message_id].filter(Boolean).join(' '),
  }
}

export function buildForward(d: MessageDetail): ComposeInitial {
  const head = `---------- 转发邮件 ----------<br>主题: ${escapeHtml(d.subject || '')}<br>发件人: ${escapeHtml(d.from_name || d.from_addr || '')}<br>日期: ${escapeHtml(d.date)}<br><br>`
  return {
    to: [],
    subject: rePrefix(d.subject || '', 'Fwd: '),
    bodyHtml: `<br><br>${head}${originalBody(d)}`,
  }
}

/**
 * mailto: 链接 → 撰写器初始内容。
 *
 * body 是纯文本（RFC 6068 没有 HTML 正文这一说），进富文本编辑器前必须转义并把换行
 * 换成 <br>：正文 iframe 里的 mailto 链接完全由发件人控制，直接当 HTML 塞进撰写器
 * 等于把邮件内容注入到我们自己要发出去的那封信里。
 */
export function buildMailtoCompose(href: string): ComposeInitial | null {
  const f = parseMailto(href)
  if (!f) return null
  return {
    to: f.to,
    cc: f.cc,
    subject: f.subject,
    bodyHtml: f.body ? escapeHtml(f.body).replace(/\r?\n/g, '<br>') : '',
  }
}

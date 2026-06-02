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

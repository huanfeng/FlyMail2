import { QUOTE_BLOCK_ATTR } from '@/components/mail/composer/schema'
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

/**
 * 引用块。
 *
 * 标记属性 `data-quote-block` 是给撰写器的 quoteBlock 节点认的——认出来才能折叠。
 * 收件方的邮件客户端不认识这个属性，看到的就是一个普通的 blockquote，
 * 行内样式因此必须留着（renderHTML 会再补一遍，这里是给不过编辑器的路径兜底）。
 */
function quoteBlock(inner: string): string {
  return `<blockquote ${QUOTE_BLOCK_ATTR}="true" style="border-left:2px solid #ccc;padding-left:10px;color:#666">${inner}</blockquote>`
}

export function buildReply(d: MessageDetail): ComposeInitial {
  return {
    to: d.from_addr ? [d.from_addr] : [],
    subject: rePrefix(d.subject || '', 'Re: '),
    // 前面留一个空段落：光标落点在这里，用户直接就能开始写
    bodyHtml: `<p></p>${quoteBlock(`<p>${escapeHtml(quoteHeader(d))}</p>${originalBody(d)}`)}`,
    inReplyTo: d.message_id,
    references: [d.references, d.message_id].filter(Boolean).join(' '),
    scenario: 'reply',
  }
}

export function buildForward(d: MessageDetail): ComposeInitial {
  const head = `---------- 转发邮件 ----------<br>主题: ${escapeHtml(d.subject || '')}<br>发件人: ${escapeHtml(d.from_name || d.from_addr || '')}<br>日期: ${escapeHtml(d.date)}`
  return {
    to: [],
    subject: rePrefix(d.subject || '', 'Fwd: '),
    bodyHtml: `<p></p>${quoteBlock(`<p>${head}</p>${originalBody(d)}`)}`,
    // 转发和回复一样是"接着一封已有的信写"，签名按回复场景走
    scenario: 'reply',
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

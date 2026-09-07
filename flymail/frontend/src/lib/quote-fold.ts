// 引用内容折叠：把「本次新写的内容」与「被引用的历史往返」切开。
//
// 为什么要做：一条 10 封的讨论里，第 10 封往往把前 9 封原样带在下面。会话手风琴
// 展开后，同样的文字会在屏幕上重复出现十次——引用不折叠，会话视图反而比单封更难读。
//
// 为什么是纯函数：文本切分与 HTML 标记都只依赖输入字符串，放在这里才能被单测覆盖。
// 渲染层（MessageBody / MailBodyFrame）只负责「折叠时套上哪份 CSS、展开时怎么显示」。
//
// ⚠ 贯穿全文件的一条原则：**误折（把正文藏起来）比漏折严重得多**。
// 漏折只是多看几屏历史，误折是内容凭空消失、且用户不知道有个按钮能找回来。
// 所以每条判据都额外要求「切点之后再无正文」，宁可放过也不错杀。

// ─────────────────────────────────────────────────────────────────────────────
// 纯文本正文
// ─────────────────────────────────────────────────────────────────────────────

/** 文本正文的切分结果。quoted 为空串表示这封信没有可折叠的引用。 */
export interface TextQuoteSplit {
  /** 本次新写的内容（折叠时显示这一段） */
  visible: string
  /** 被引用的历史内容；空串表示不折叠 */
  quoted: string
}

/** 是否为 `>` 引用行（含 `>>` 嵌套与 `> ` 空格变体） */
function isQuotedLine(line: string): boolean {
  return /^\s{0,3}>/.test(line)
}

/** 空行（只有空白） */
function isBlank(line: string): boolean {
  return line.trim() === ''
}

/** 「发件人:」这一类转发头块的首行 */
const FROM_HEADER_RE = /^\s*(?:发件人|發件人|From)\s*[：:]\s*\S/

/** 「-----原始邮件-----」这一类无歧义的分隔线 */
const ORIGINAL_MARKER_RE =
  /^\s*-{2,}\s*(?:Original Message|Forwarded message|原始邮件|原始郵件|邮件原件|郵件原件)\s*-{2,}\s*$/i

/** 「在 …… 写道：」/「…… wrote:」这一类以冒号收尾的引用头 */
const WROTE_HEADER_RE = /(?:\bwrote\s*:|(?:写道|寫道)\s*[：:])\s*$/i

/** Outlook 的下划线分隔线 */
const UNDERLINE_RE = /^_{5,}\s*$/

/**
 * 切点之后是不是「只剩引用」。
 *
 * `>` 引用块与 `写道：` 头都只在正文尾部才算引用：开发者收到的邮件里，
 * 行首 `>` 大量出现在终端记录、diff、Markdown 引言里（GitHub 通知的纯文本版就是
 * 「一段 `>` 引用的代码 + 下面继续写正文 + 页脚」），按「见到第一个 `>` 就切」
 * 会把人家真正想说的话连同页脚一起藏掉。
 */
function tailIsOnlyQuoted(lines: string[], from: number): boolean {
  for (let i = from; i < lines.length; i++) {
    if (isBlank(lines[i]) || isQuotedLine(lines[i])) continue
    return false
  }
  return true
}

/** 从 i 起的第一行非空行；没有则返回 null */
function nextNonBlank(lines: string[], from: number): string | null {
  for (let i = from; i < lines.length; i++) {
    if (!isBlank(lines[i])) return lines[i]
  }
  return null
}

/**
 * 把切点往上挪到引用头的真正第一行。
 *
 * Gmail 会把「On <日期> <发件人> wrote:」折成两三行，只有末行带 wrote:。
 * 不往上找就会在正文末尾留下一句半截的日期，比不折还难看。
 */
function extendQuoteHeaderStart(lines: string[], cut: number): number {
  if (!/\bwrote\s*:\s*$/i.test(lines[cut])) return cut
  for (let i = cut - 1; i >= 0 && i >= cut - 3; i--) {
    if (isBlank(lines[i])) break
    if (/^\s*On\b/i.test(lines[i])) return i
  }
  return cut
}

/**
 * 找到引用的起始行号；没有可折叠的引用时返回 -1。
 *
 * 逐行看，每种判据各带一条「这真的是引用吗」的旁证；不成立就继续往下找，
 * 而不是就此认定整封信都不折——正文中段的一段 Markdown 引言不该挡住
 * 底下那个货真价实的「在 …… 写道：」。
 */
function findQuoteCut(lines: string[]): number {
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // `>` 引用块：其后不能再有正文
    if (isQuotedLine(line)) {
      if (tailIsOnlyQuoted(lines, i)) return i
      continue
    }

    // 「-----原始邮件-----」：措辞本身没有歧义，不再要旁证
    if (ORIGINAL_MARKER_RE.test(line)) return i

    // 「…… 写道：」/「…… wrote:」：其后必须全是 `>` 引用或空行。
    // 正文里转述一句「他在邮件里写道：」恰好断在行尾时，下面接着的是正文，判据不成立。
    if (WROTE_HEADER_RE.test(line)) {
      if (tailIsOnlyQuoted(lines, i + 1)) return i
      continue
    }

    // Outlook 的下划线分隔线：必须紧跟着发件人头块。
    // 整行下划线在简报、周报里就是一条普通分隔线，单看它什么也说明不了。
    if (UNDERLINE_RE.test(line)) {
      const next = nextNonBlank(lines, i + 1)
      if (next != null && FROM_HEADER_RE.test(next)) return i
      continue
    }

    // 「发件人:」头块首行
    if (FROM_HEADER_RE.test(line)) return i
  }
  return -1
}

/**
 * 把纯文本正文切成「新内容 + 引用」。
 *
 * 两种情况刻意不折叠，因为折了只会让人看到一片空白：
 *   1. 切点在第 0 行（整封信就是一段转发/引用）；
 *   2. 切点之前只有空行。
 */
export function splitTextQuote(text: string): TextQuoteSplit {
  const none: TextQuoteSplit = { visible: text, quoted: '' }
  if (!text) return none

  const lines = text.split('\n')
  let cut = findQuoteCut(lines)
  if (cut < 0) return none
  cut = extendQuoteHeaderStart(lines, cut)
  if (cut <= 0) return none

  const head = lines.slice(0, cut)
  const tail = lines.slice(cut)
  // 引用头之前通常空一行，跟着正文一起显示只会多出一段空白
  while (head.length > 0 && isBlank(head[head.length - 1])) head.pop()

  const visible = head.join('\n')
  const quoted = tail.join('\n')
  if (visible.trim() === '' || quoted.trim() === '') return none
  return { visible, quoted }
}

// ─────────────────────────────────────────────────────────────────────────────
// HTML 正文
// ─────────────────────────────────────────────────────────────────────────────

/** 被标记的引用容器带上的属性名（配合 QUOTE_HIDE_CSS 隐藏） */
export const QUOTE_ATTR = 'data-fm-quote'

/**
 * 折叠态注入 iframe 的样式。
 *
 * 折叠不靠把 HTML 切成两半——邮件 HTML 的标签嵌套极不规范，按字符串切开
 * 几乎必然切出一堆未闭合标签。改为「整份文档照常渲染，只把引用容器藏起来」，
 * 展开时不注入这段样式即可，两种状态用的是同一份 HTML。
 */
export const QUOTE_HIDE_CSS = `[${QUOTE_ATTR}]{display:none!important}`

/** markHtmlQuotes 的结果 */
export interface HtmlQuoteMark {
  /** 引用容器已加上 QUOTE_ATTR 的 HTML；无引用时返回剥净标记的原文 */
  html: string
  /** 是否找到了可折叠的引用（决定要不要显示「显示引用内容」按钮） */
  hasQuote: boolean
}

/**
 * 判断一个起始标签是不是引用容器。
 *
 * blockquote 是通用做法；其余是各家客户端的私有约定，
 * 少认一个只是少折一层，认错一个却会把正文藏掉，所以宁可保守。
 */
function isQuoteTag(name: string, attrs: string): boolean {
  const tag = name.toLowerCase()
  if (tag === 'blockquote') return true
  if (tag !== 'div') return false
  // Gmail / Thunderbird / Yahoo 的引用容器类名
  if (/\bclass\s*=\s*["'][^"']*\b(?:gmail_quote|gmail_quote_container|moz-cite-prefix|yahoo_quoted)\b/i.test(attrs)) {
    return true
  }
  // Outlook 网页版把「原邮件」分隔块放在这两个 id 下
  return /\bid\s*=\s*["']?(?:divRplyFwdMsg|mail-editor-reference-message-container)\b/i.test(attrs)
}

/** 片段里是否还有肉眼可见的内容（用于判断折叠后会不会剩一片空白） */
function hasVisibleContent(fragment: string): boolean {
  const stripped = fragment
    .replace(/<(style|script)\b[\s\S]*?<\/\1>/gi, '')
    .replace(/<!--[\s\S]*?-->/g, '')
  // 图片本身就是内容，哪怕一个字都没有
  if (/<img\b/i.test(stripped)) return true
  const text = stripped
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&#(?:160|xa0);/gi, ' ')
  return text.trim().length > 0
}

/** 匹配任意起始/结束标签；attrs 捕获组用于识别 class / id */
const TAG_RE = /<(\/?)([a-zA-Z][a-zA-Z0-9-]*)((?:[^>"']|"[^"]*"|'[^']*')*)>/g

/**
 * 剥掉输入里已有的 QUOTE_ATTR。
 *
 * ⚠ 安全相关：这个属性是邮件作者能自己写进 HTML 的。若不剥，一封信里只要
 * 另有一个真的 blockquote 让折叠生效，作者预置在正文里的 `data-fm-quote`
 * 元素就会跟着一起被藏起来——等于让发件人决定收件人默认看不到哪一段。
 * 只在起始标签内部剥，不动正文文字。
 */
function stripQuoteAttr(html: string): string {
  const attrRe = new RegExp(`\\s${QUOTE_ATTR}(?:\\s*=\\s*(?:"[^"]*"|'[^']*'|[^\\s>]*))?`, 'gi')
  TAG_RE.lastIndex = 0
  let out = ''
  let last = 0
  let m: RegExpExecArray | null
  while ((m = TAG_RE.exec(html)) !== null) {
    const tag = m[0]
    if (!attrRe.test(tag)) continue
    attrRe.lastIndex = 0
    out += html.slice(last, m.index) + tag.replace(attrRe, '')
    last = m.index + tag.length
  }
  return last === 0 ? html : out + html.slice(last)
}

/** 一个引用容器在源码里的位置 */
interface QuoteHit {
  /** 起始标签的 `<` 下标 */
  start: number
  /** 起始标签里标签名结束的位置（属性插在这后面） */
  nameEnd: number
  /** 容器结束标签之后的下标；找不到配对时为 html.length */
  end: number
}

/**
 * 从 startTagEnd 开始找 tag 的配对结束标签，返回它之后的下标。
 *
 * 邮件 HTML 常有未闭合标签，找不到配对时按「一直延伸到文末」处理——
 * 那正好等价于「这个引用在正文尾部」，与我们只折尾部引用的判据一致。
 */
function findContainerEnd(html: string, startTagEnd: number, tag: string): number {
  const want = tag.toLowerCase()
  const re = new RegExp(TAG_RE.source, 'g')
  re.lastIndex = startTagEnd
  let depth = 1
  let m: RegExpExecArray | null
  while ((m = re.exec(html)) !== null) {
    if (m[2].toLowerCase() !== want) continue
    if (m[1] === '/') {
      depth--
      if (depth === 0) return m.index + m[0].length
    } else if (!m[3].trimEnd().endsWith('/')) {
      depth++
    }
  }
  return html.length
}

/**
 * 给 HTML 正文里位于**尾部**的引用容器打上 QUOTE_ATTR 标记。
 *
 * 两条保守判据，都是为了不让正文消失：
 *   1. 只折叠其后再无可见内容的容器——正文中段用 `<blockquote>` 排版的引言
 *      （引用一段规范、一句客户原话）是内容，不是历史往返；
 *   2. 第一个被折叠的容器之前必须有可见内容——整封被 gmail_quote 包起来的
 *      转发信折了等于一片空白。
 */
export function markHtmlQuotes(html: string): HtmlQuoteMark {
  if (!html) return { html, hasQuote: false }
  // 先剥掉伪造的标记，后面所有下标都基于剥净后的串
  const src = stripQuoteAttr(html)
  const none: HtmlQuoteMark = { html: src, hasQuote: false }

  const hits: QuoteHit[] = []
  TAG_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TAG_RE.exec(src)) !== null) {
    if (m[1] === '/' || !isQuoteTag(m[2], m[3])) continue
    const startTagEnd = m.index + m[0].length
    hits.push({
      start: m.index,
      nameEnd: m.index + 1 + m[2].length,
      end: findContainerEnd(src, startTagEnd, m[2]),
    })
  }
  if (hits.length === 0) return none

  // 从后往前收：一个容器可折叠，当且仅当它与「已收下的最靠前那个容器」之间
  // 没有可见内容。这样连着的几段引用能一起折，中间夹着正文的那些则被挡住。
  const accepted: QuoteHit[] = []
  let tailStart = src.length
  for (let i = hits.length - 1; i >= 0; i--) {
    const hit = hits[i]
    const between = hit.end >= tailStart ? '' : src.slice(hit.end, tailStart)
    if (hasVisibleContent(between)) continue
    accepted.push(hit)
    tailStart = Math.min(tailStart, hit.start)
  }
  if (accepted.length === 0) return none
  if (!hasVisibleContent(src.slice(0, tailStart))) return none

  // accepted 是倒序收集的，正好可以从后往前插入属性，前面的下标不会被撑偏
  let out = src
  for (const hit of accepted) {
    out = `${out.slice(0, hit.nameEnd)} ${QUOTE_ATTR}${out.slice(hit.nameEnd)}`
  }
  return { html: out, hasQuote: true }
}

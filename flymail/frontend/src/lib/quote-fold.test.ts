import { describe, it, expect } from 'vitest'
import { QUOTE_ATTR, markHtmlQuotes, splitTextQuote } from '@/lib/quote-fold'

/** 数一段 HTML 里被打上折叠标记的容器个数 */
function markCount(html: string): number {
  return html.split(`${QUOTE_ATTR}`).length - 1
}

describe('splitTextQuote', () => {
  it('把尾部的 > 引用块切出来', () => {
    const text = ['收到，明天见。', '', '> 原文第一行', '>> 更早的一层', '> 原文第二行'].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('收到，明天见。')
    expect(r.quoted).toBe('> 原文第一行\n>> 更早的一层\n> 原文第二行')
  })

  it('识别中文「写道：」引用头', () => {
    const text = ['好的。', '', '在 2026年9月1日 10:00，张三 <a@b.com> 写道：', '> 原文内容'].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('好的。')
    expect(r.quoted.startsWith('在 2026年9月1日')).toBe(true)
  })

  it('识别英文 wrote: 引用头（含 Gmail 折行的第二行）', () => {
    const text = [
      'Sounds good.',
      '',
      'On Mon, Sep 1, 2026 at 10:00 AM',
      'Foo <a@b.com> wrote:',
      '> original',
    ].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('Sounds good.')
    // 折行时只有末行带 wrote:，切点要回溯到 On 开头那一行
    expect(r.quoted).toBe('On Mon, Sep 1, 2026 at 10:00 AM\nFoo <a@b.com> wrote:\n> original')
  })

  it('识别 -----Original Message----- 分隔线', () => {
    const text = ['FYI', '-----Original Message-----', 'From: a@b.com'].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('FYI')
    expect(r.quoted).toContain('Original Message')
  })

  it('识别「发件人:」引用头块', () => {
    const text = ['请查收。', '', '发件人: 张三 <a@b.com>', '主题: 报价'].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('请查收。')
    expect(r.quoted).toContain('发件人:')
  })

  it('整封信都是引用时不折叠', () => {
    const text = ['> 原文第一行', '> 原文第二行'].join('\n')
    expect(splitTextQuote(text)).toEqual({ visible: text, quoted: '' })
  })

  it('引用之前只有空行时不折叠', () => {
    const text = ['', '   ', '> 原文'].join('\n')
    expect(splitTextQuote(text).quoted).toBe('')
  })

  it('没有引用时原样返回', () => {
    const text = '就一句话。'
    expect(splitTextQuote(text)).toEqual({ visible: text, quoted: '' })
  })

  it('正文里行内出现「写道：」不误判', () => {
    const text = '他昨天在邮件里写道：这样不行，我们再议。'
    expect(splitTextQuote(text).quoted).toBe('')
  })

  it('空串安全', () => {
    expect(splitTextQuote('')).toEqual({ visible: '', quoted: '' })
  })

  // ── 尾部判据：> 引用块只在正文末尾才算引用 ────────────────────────────────
  it('GitHub 通知式纯文本（引用在中间、下面还有正文和页脚）不折叠', () => {
    const text = [
      '@zhangsan commented on this pull request.',
      '',
      '> func handler(w http.ResponseWriter) {',
      '>   panic("boom")',
      '> }',
      '',
      'This looks wrong to me, please guard it.',
      '',
      '—',
      'Reply to this email directly or view it on GitHub.',
      'You are receiving this because you were mentioned.',
    ].join('\n')
    expect(splitTextQuote(text).quoted).toBe('')
  })

  it('终端记录里的 > 提示符（后面还有正文）不折叠', () => {
    const text = ['复现步骤：', '> npm run build', '输出如上，麻烦看看。'].join('\n')
    expect(splitTextQuote(text).quoted).toBe('')
  })

  it('中段有 Markdown 引言、尾部有真引用时，切在真引用处', () => {
    const text = [
      '规范里写着：',
      '> MUST NOT retry on 4xx',
      '所以这里不能重试。',
      '',
      '在 2026年9月1日，张三 写道：',
      '> 要不要加个重试？',
    ].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toContain('所以这里不能重试。')
    expect(r.visible).toContain('MUST NOT retry')
    expect(r.quoted.startsWith('在 2026年9月1日')).toBe(true)
  })

  it('「wrote:」之后还有正文时不折叠（正文转述）', () => {
    const text = [
      'Here is what the doc says about who wrote:',
      'the author list is maintained separately.',
      'Please double check before shipping.',
    ].join('\n')
    expect(splitTextQuote(text).quoted).toBe('')
  })

  // ── 尾部判据：下划线分隔线必须紧跟发件人头块 ──────────────────────────────
  it('Outlook 下划线分隔线 + 发件人头块 → 折叠', () => {
    const text = ['好的，收到。', '', '________________________________', '发件人: 张三 <a@b.com>', '主题: 报价'].join('\n')
    const r = splitTextQuote(text)
    expect(r.visible).toBe('好的，收到。')
    expect(r.quoted.startsWith('____')).toBe(true)
  })

  it('简报里的下划线分隔线（后面不是发件人头）不折叠', () => {
    const text = ['本周要点：', '________________________________', '一、发布了 1.2 版本', '二、修了三个 bug'].join('\n')
    expect(splitTextQuote(text).quoted).toBe('')
  })
})

describe('markHtmlQuotes', () => {
  it('标记尾部的 blockquote', () => {
    const r = markHtmlQuotes('<p>收到</p><blockquote class="x">原文</blockquote>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toBe(`<p>收到</p><blockquote ${QUOTE_ATTR} class="x">原文</blockquote>`)
  })

  it('标记 gmail_quote 容器', () => {
    const r = markHtmlQuotes('<div>ok</div><div class="gmail_quote gmail_quote_container">原文</div>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toContain(`<div ${QUOTE_ATTR} class="gmail_quote`)
  })

  it('标记 Thunderbird 的 moz-cite-prefix', () => {
    const r = markHtmlQuotes('<p>hi</p><div class="moz-cite-prefix">张三 写道：</div>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toContain(`<div ${QUOTE_ATTR} class="moz-cite-prefix">`)
  })

  it('标记 Yahoo 的 yahoo_quoted', () => {
    const r = markHtmlQuotes('<p>hi</p><div class="yahoo_quoted">原文</div>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toContain(`<div ${QUOTE_ATTR} class="yahoo_quoted">`)
  })

  it('标记 Outlook 的 divRplyFwdMsg', () => {
    const r = markHtmlQuotes('<p>hi</p><div id="divRplyFwdMsg">原文</div>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toContain(`<div ${QUOTE_ATTR} id="divRplyFwdMsg">`)
  })

  it('尾部连着的多个引用容器全部标记', () => {
    const r = markHtmlQuotes('<p>hi</p><blockquote>a</blockquote><blockquote>b</blockquote>')
    expect(markCount(r.html)).toBe(2)
  })

  it('嵌套 blockquote 内外都标记（藏外层即可，标记内层无害）', () => {
    const r = markHtmlQuotes('<p>hi</p><blockquote>外<blockquote>内</blockquote></blockquote>')
    expect(r.hasQuote).toBe(true)
    expect(markCount(r.html)).toBe(2)
  })

  it('引用之前没有可见内容时不标记', () => {
    const html = '<div class="gmail_quote"><p>整封都是转发</p></div>'
    expect(markHtmlQuotes(html)).toEqual({ html, hasQuote: false })
  })

  it('引用之前只有图片也算可见内容', () => {
    const r = markHtmlQuotes('<img src="cid:x"><blockquote>原文</blockquote>')
    expect(r.hasQuote).toBe(true)
  })

  it('引用之前只有 style 块时不算可见内容', () => {
    const html = '<style>p{color:red}</style><blockquote>原文</blockquote>'
    expect(markHtmlQuotes(html).hasQuote).toBe(false)
  })

  it('&nbsp; 不算可见内容', () => {
    expect(markHtmlQuotes('<p>&nbsp;</p><blockquote>原文</blockquote>').hasQuote).toBe(false)
  })

  it('没有引用容器时原样返回', () => {
    const html = '<p>hello</p>'
    expect(markHtmlQuotes(html)).toEqual({ html, hasQuote: false })
  })

  it('属性值里带 > 时不错切标签', () => {
    const r = markHtmlQuotes('<p title="a>b">hi</p><blockquote>原文</blockquote>')
    expect(r.hasQuote).toBe(true)
    expect(r.html).toContain(`<blockquote ${QUOTE_ATTR}>`)
  })

  it('空串安全', () => {
    expect(markHtmlQuotes('')).toEqual({ html: '', hasQuote: false })
  })

  // ── 尾部判据：正文中段的 blockquote 是内容，不是历史往返 ────────────────────
  it('正文中段用 blockquote 排版的引言不折叠', () => {
    const html = '<p>规范里写着：</p><blockquote>MUST NOT retry</blockquote><p>所以不能重试。</p>'
    expect(markHtmlQuotes(html)).toEqual({ html, hasQuote: false })
  })

  it('中段有引言、尾部有真引用时只折尾部那个', () => {
    const html =
      '<p>规范里写着：</p><blockquote>MUST NOT retry</blockquote><p>所以不能重试。</p>' +
      '<div class="gmail_quote">张三 写道：</div><blockquote>要不要加重试？</blockquote>'
    const r = markHtmlQuotes(html)
    expect(r.hasQuote).toBe(true)
    expect(markCount(r.html)).toBe(2)
    // 中段那个引言保持原样，没有被打上标记
    expect(r.html).toContain('<blockquote>MUST NOT retry</blockquote>')
  })

  // ── 安全：邮件作者不能自己往正文里预置折叠标记 ────────────────────────────
  it('输入里预置的 data-fm-quote 会被剥掉，不随折叠一起消失', () => {
    const html = `<p ${QUOTE_ATTR}>这段是正文，作者想让它默认不可见</p><blockquote>原文</blockquote>`
    const r = markHtmlQuotes(html)
    expect(r.hasQuote).toBe(true)
    // 只有真正的引用容器被标记，伪造的那个 <p> 已被剥净
    expect(markCount(r.html)).toBe(1)
    expect(r.html).toContain('<p>这段是正文')
  })

  it('带值的伪造标记同样被剥掉', () => {
    const html = `<div ${QUOTE_ATTR}="1">正文</div><div ${QUOTE_ATTR}='x'>正文2</div><blockquote>原文</blockquote>`
    const r = markHtmlQuotes(html)
    expect(markCount(r.html)).toBe(1)
  })

  it('没有真引用时也把伪造标记剥掉（否则下一封的样式会误伤）', () => {
    const html = `<p ${QUOTE_ATTR}>正文</p>`
    const r = markHtmlQuotes(html)
    expect(r.hasQuote).toBe(false)
    expect(markCount(r.html)).toBe(0)
  })
})

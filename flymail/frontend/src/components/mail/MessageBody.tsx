// 单封邮件的正文渲染：远程内容拦截条 + HTML 沙箱 iframe / 纯文本 + 引用折叠 + 附件卡片。
//
// 从 Reader.tsx 抽出来的唯一理由是会话手风琴：一条会话里每个展开项都要渲染一份正文，
// 而正文渲染带着 CSP、远程图拦截、iframe 高度测量这一整套不能复制第二份的逻辑。
// 单封视图与会话视图从此共用同一份实现，改一处两边同时生效。
//
// ⚠ 调用方必须以 key={detail.id} 挂载：showImages / showQuote 是「这一封」的状态，
// 换邮件不重置就会把上一封的选择带过去。

import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { formatBytes } from '@/lib/format'
import { openExternal } from '@/lib/platform'
import { QUOTE_HIDE_CSS, markHtmlQuotes, splitTextQuote } from '@/lib/quote-fold'
import type { MessageDetail } from '@/lib/types'
import {
  attachmentUrl,
  downloadAttachment,
  isPreviewable,
  rewriteCidLinks,
} from '@/lib/attachments'

// ── 工具函数 ─────────────────────────────────────────────

/** 把 HTML 中远程图片的 src 替换为 data-blocked-src，返回处理后的 HTML 和被拦截数量 */
function blockRemoteImages(html: string): { html: string; blocked: number } {
  let blocked = 0
  const out = html.replace(/<img\b[^>]*>/gi, (tag) =>
    tag.replace(/\ssrc\s*=\s*("|')(https?:\/\/[^"']*)\1/gi, (_m, q, url) => {
      blocked++
      return ` data-blocked-src=${q}${url}${q}`
    }),
  )
  return { html: out, blocked }
}

/**
 * 移除所有指向远程的 <link>——不分 rel。
 *
 * 一是隐私：邮件打开瞬间向第三方发请求，等同于一张"这封信被读了"的回执。
 * 此前只挡了 <img>，<link> 照发不误，是同一道防线上的缺口。
 *
 * ⚠ 二是更要命的一点：iframe 的 load 事件要等文档内所有子资源结束才触发。
 * 邮件里常见的 <link rel=preconnect href=https://fonts.googleapis.com> 这类资源预热，
 * 在连不通该域名的网络下会一直挂到连接超时（实测把 load 拖了 21 秒），
 * 而正文的高度测量与显示都挂在 load 上——邮件就卡成一个不显示或只有兜底高度的小框。
 * 所以这里按 href 协议判断，不按 rel 白名单：
 * 邮件正文里的 <link> 没有任何需要保留远程引用的正当用途。
 */
function stripRemoteLinks(html: string): string {
  return html.replace(/<link\b[^>]*>/gi, (tag) =>
    /href\s*=\s*("|')?https?:/i.test(tag) ? '' : tag,
  )
}

/**
 * 内容安全策略：从根上掐断正文向外发起的请求，而不是逐个正则去剥。
 *
 * 正则总会漏——CSS 的 @import、background:url()、<video poster>、srcset……
 * CSP 是浏览器层面的兜底，把"允许什么"一次讲清楚。
 * 注意它管不住 Resource Hints（preconnect/dns-prefetch 不受 CSP 约束），
 * 那部分仍然要靠 stripRemoteLinks 从 HTML 里删掉，两者互补。
 *
 * @param allowRemote 用户点了"显示远程内容"，此时才放行外部图片与字体
 */
function cspMeta(allowRemote: boolean): string {
  const policy = [
    "default-src 'none'",
    // 'self' 是父文档的 origin：内联 cid 图片改写后指向我们自己的附件接口
    allowRemote ? "img-src 'self' data: blob: https: http:" : "img-src 'self' data: blob:",
    allowRemote ? "style-src 'unsafe-inline' https:" : "style-src 'unsafe-inline'",
    allowRemote ? "font-src data: https:" : "font-src data:",
    // 邮件 JS 已由 sandbox 禁用，这里再声明一次作为纵深防御
    "script-src 'none'",
    "connect-src 'none'",
    "frame-src 'none'",
    "object-src 'none'",
    "form-action 'none'",
  ].join('; ')
  return `<meta http-equiv="Content-Security-Policy" content="${policy}">`
}

/**
 * 注入到 HTML 正文 iframe 里的基础样式。
 *
 * 邮件 HTML 几乎都是「假设自己在一个白底、有默认字体的文档里」写的：不注入任何样式时，
 * iframe 用的是浏览器缺省样式（Times New Roman、body margin 8px、图片原始尺寸），
 * 于是渲染结果和其它邮件客户端明显不同——这正是"CSS 处理不正常"的来源。
 *
 * 背景固定为白色而不跟随应用主题：邮件里的前景色是写死的（大量深色文字、
 * 甚至写死 color:#000 的签名），在深色背景上会直接变成黑底黑字。
 * 主流桌面客户端同样把 HTML 正文渲染在白底卡片里。
 */
const MAIL_BODY_CSS = `
html { -webkit-text-size-adjust: 100%; }
body {
  margin: 0; padding: 14px 16px;
  /* ⚠ 有 padding 就必须配 border-box：邮件模板极常见 body{width:100% !important}
     （GitHub、各类营销邮件都这么写），content-box 下 100% 再加上这里的左右内边距
     就会横向溢出正好 32px，正文凭空多出一条横向滚动条。 */
  box-sizing: border-box;
  background: #ffffff; color: #1f2328;
  font-family: -apple-system, "Segoe UI", "Microsoft YaHei", Roboto, Helvetica, Arial, sans-serif;
  font-size: 14px; line-height: 1.6;
  overflow-wrap: break-word; word-break: break-word;
}
/* ⚠ 千万不要在 body 上写 overflow：CSS 规范会把 body 的 overflow 传播到视口，
   body 自身的计算值则变成 visible。这会让 body.scrollHeight / documentElement.scrollHeight
   的语义随之改变，正是高度量不准的经典来源。宽内容溢出交给 iframe 视口默认的滚动行为。 */
img { max-width: 100%; height: auto; border: 0; }
table { max-width: 100%; }
a { color: #0969da; }
pre { white-space: pre-wrap; word-break: break-word; }
pre, code { font-family: ui-monospace, Consolas, "Courier New", monospace; }
blockquote {
  margin: 8px 0 8px 2px; padding-left: 12px;
  border-left: 3px solid #d0d7de; color: #57606a;
}
hr { border: 0; border-top: 1px solid #d8dee4; margin: 16px 0; }
`

/** iframe 兜底高度：测量失败时至少给出可读的一屏，而不是缩成一个小格子 */
const MIN_BODY_HEIGHT = 240

/**
 * 量出 iframe 内文档的真实内容高度。
 *
 * ⚠ 不能用 body.scrollHeight —— 它至少等于视口高度，而 iframe 的视口高度正是我们
 * 要计算的那个值。内容比当前 iframe 矮时，它会把上一封邮件留下的高度原样返回，
 * 于是这封邮件"永远沿用上一封的大小"。先把 iframe 高度清零再测也不行：
 * 清的是父文档里的 iframe 元素，子文档的重新布局不会在同一个同步块里完成。
 *
 * 改为在内容末尾放一个哨兵元素，问浏览器把它排到了哪里：位置由布局引擎给出，
 * 只取决于内容本身，与视口多高无关，从根上绕开了这个循环依赖。
 * 具体的两路度量与各自的盲区见函数内注释。
 */
function measureContentHeight(doc: Document): number {
  const body = doc.body
  const root = doc.documentElement
  if (!body) return 0
  const view = doc.defaultView
  // getBoundingClientRect 相对视口，用户若滚动过 iframe 内部需补回滚动量
  const scrollTop = root?.scrollTop || body.scrollTop || 0
  const px = (v: string | null | undefined): number => {
    const n = parseFloat(v || '0')
    return Number.isFinite(n) ? n : 0
  }

  // body 自己的下内边距/外边距：任何一种量法都不含它
  let bodyTail = 0
  if (view) {
    const cs = view.getComputedStyle(body)
    bodyTail = px(cs.paddingBottom) + px(cs.marginBottom)
  }

  // ── 来源一：末尾哨兵（主）────────────────────────────────────────────
  // 在内容末尾放一个零高元素，问浏览器把它排到了哪里——位置由布局引擎算出，
  // 天然包含前面所有元素的下外边距；clear:both 让它落到所有浮动之下，
  // 于是「浮动不撑高父容器」这个盲区也一并消失。
  // 复用同一个哨兵：反复插入会惊动 ResizeObserver。
  let sentinel = body.querySelector<HTMLElement>(':scope > [data-fm-measure]')
  if (!sentinel) {
    sentinel = doc.createElement('div')
    sentinel.setAttribute('data-fm-measure', '')
    sentinel.style.cssText =
      'display:block;height:0;clear:both;font-size:0;line-height:0;border:0;padding:0;margin:0;'
    body.appendChild(sentinel)
  }
  const bySentinel = sentinel.getBoundingClientRect().top + scrollTop + bodyTail

  // ── 来源二：子元素底边 + 各自外边距（交叉校验）──────────────────────
  // 哨兵挡不住绝对定位/负 margin 造成的溢出，这一路作为补充。
  // 两条路的盲区不重叠，取大者。
  let byChildren = 0
  for (const child of Array.from(body.children)) {
    if (child === sentinel) continue
    const rect = child.getBoundingClientRect()
    if (rect.height <= 0) continue
    const mb = view ? px(view.getComputedStyle(child).marginBottom) : 0
    byChildren = Math.max(byChildren, rect.bottom + scrollTop + mb)
  }
  if (byChildren > 0) byChildren += bodyTail

  const best = Math.max(bySentinel, byChildren)
  // 两路都没量到（纯文本正文、文档尚未布局）才退回 scrollHeight。
  // 它带着视口下限，只能当最后的兜底，不能当主力。
  return best > 0 ? best : Math.max(body.scrollHeight, root?.scrollHeight ?? 0)
}

/**
 * 把邮件 HTML 包成一个自带基础样式的完整文档，供 iframe srcDoc 使用。
 *
 * 引用折叠只是多注入一条隐藏规则：整份文档照常渲染，只把打了标记的引用容器藏起来。
 * 按字符串把 HTML 切成两半几乎必然切出未闭合标签，见 lib/quote-fold.ts。
 */
function wrapBodyHtml(html: string, allowRemote: boolean, foldQuote: boolean): string {
  // base target="_blank" 是兜底：正常路径是父窗口拦截点击后交给系统浏览器（见 bindExternalLinks），
  // 万一拦截未生效，也不至于让链接把 iframe 里的邮件内容顶掉。
  return (
    `<meta charset="utf-8">${cspMeta(allowRemote)}` +
    `<base target="_blank"><style>${MAIL_BODY_CSS}${foldQuote ? QUOTE_HIDE_CSS : ''}</style>${html}`
  )
}

/**
 * 邮件正文里的链接一律交给系统浏览器，绝不在 WebView2 内导航——
 * 那会把整个应用页面替换成外站，而 iframe 和应用都没有后退入口。
 *
 * 监听器由父窗口注册：sandbox 不含 allow-scripts，邮件自带的脚本仍然不会执行，
 * 但 allow-same-origin 让父窗口能操作这份文档，回调运行在父窗口的上下文里。
 */
function bindExternalLinks(doc: Document) {
  doc.addEventListener('click', (e) => {
    const anchor = (e.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null
    if (!anchor) return
    // 文档内锚点（#section）保持默认滚动行为
    if ((anchor.getAttribute('href') ?? '').startsWith('#')) return
    e.preventDefault()
    // 取 .href 而非 getAttribute：由浏览器把相对地址解析成绝对地址。
    // mailto: 同样交给系统默认邮件程序处理。
    openExternal(anchor.href)
  })
}

interface MailBodyFrameProps {
  /** 已做过 cid 改写 / 远程内容拦截 / 引用标记的邮件 HTML */
  html: string
  title: string
  /** 用户已选择显示远程内容：放宽 CSP，允许外部图片与字体 */
  allowRemote: boolean
  /** 折叠引用：注入一条把 [data-fm-quote] 藏起来的样式 */
  foldQuote: boolean
}

/**
 * HTML 正文的沙箱 iframe，负责把自己的高度贴合内容。
 *
 * ⚠ 必须由调用方以 key={detail.id} 挂载 —— 一份文档一个实例。
 *
 * 这一点是这个组件存在的全部理由：高度、observer、"是否已量准"这些状态，
 * 归属的是**某一份 iframe 文档**，而不是 Reader。此前它们放在 Reader 里，
 * 就得手工在换邮件时重置，而重置（useEffect）和 iframe 的 load 事件都是异步的、
 * 没有确定顺序：load 先跑就会被随后的重置抹掉，那一代文档的 load 又已经用掉了，
 * 正文于是永久隐藏——表现为"切换几次之后就不显示了"。
 * 交给 key 做隔离，重置逻辑连同它引出的代次守卫一起消失。
 */
export function MailBodyFrame({ html, title, allowRemote, foldQuote }: MailBodyFrameProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const resizeObsRef = useRef<ResizeObserver | null>(null)
  const aliveRef = useRef(true)
  // 首次测量完成前先藏起来（仍占位，不引起外层跳动），
  // 免得看到"小框闪一下再弹到正确高度"。
  const [measured, setMeasured] = useState(false)

  useEffect(() => {
    aliveRef.current = true
    // 兜底：无论 load 是否如期而至，到点都量一次并显示。
    // 隐藏和测量都挂在 load 上，而 iframe 的 load 并不保证会来——加载被中止、
    // 事件被吞掉都会让它缺席，结果是正文既不显示、也停在兜底的最小高度。
    // 「永远不显示」「永远量不准」都是最坏的失败模式，所以这里给一条不依赖事件的出口：
    // 文档此时多半早已渲染完，只是那个事件没送到。
    const timer = setTimeout(() => {
      fit(true)
      setMeasured(true)
    }, 400)
    return () => {
      aliveRef.current = false
      clearTimeout(timer)
      resizeObsRef.current?.disconnect()
      resizeObsRef.current = null
    }
  }, [])

  /**
   * @param rebase true = 文档刚换（允许变矮）；false = 同一份文档内的持续校正，
   *   只允许增高——迟到的回调可能量到尚未填上内容的文档，
   *   放任它缩小就会把已经正确的高度打回最小值。
   */
  function fit(rebase = false) {
    const el = iframeRef.current
    if (!el || !aliveRef.current) return
    try {
      const doc = el.contentDocument
      if (!doc?.body) return
      const content = measureContentHeight(doc)
      const target = Math.max(content + 8, MIN_BODY_HEIGHT)
      const current = el.offsetHeight
      const applied = rebase ? Math.abs(target - current) > 1 : target > current + 1
      if (applied) el.style.height = `${target}px`

    } catch {
      // 跨域（理论上不会发生，allow-same-origin 下同源）：保持回退 minHeight
    }
  }

  function handleLoad() {
    fit(true)
    setMeasured(true)
    // 下一帧再定一次基准：load 时内联图片解码、表格列宽二次计算都可能还没落定
    requestAnimationFrame(() => fit(true))
    try {
      const doc = iframeRef.current?.contentDocument
      const body = doc?.body
      if (doc) bindExternalLinks(doc)
      resizeObsRef.current?.disconnect()
      if (body && typeof ResizeObserver !== 'undefined') {
        const ro = new ResizeObserver(() => fit(false))
        ro.observe(body)
        resizeObsRef.current = ro
      }
      // 字体就绪后再测一次：字体换上后行高会变，load 时量到的是回退字体的高度。
      // Promise 无法取消，靠 aliveRef 判断组件是否已随邮件切换卸载。
      doc?.fonts?.ready?.then(() => fit(false)).catch(() => {})
    } catch {
      /* ignore */
    }
  }

  return (
    <iframe
      ref={iframeRef}
      // 包一层基础样式（字体/间距/图片自适应），否则渲染出来是浏览器缺省样式，
      // 与其它邮件客户端观感差别很大。链接点击由 bindExternalLinks 接管。
      srcDoc={wrapBodyHtml(html, allowRemote, foldQuote)}
      // allow-same-origin：仅为让父窗口量取高度、绑定链接拦截；
      // 不含 allow-scripts，邮件自带 JS 仍然禁用。
      sandbox="allow-same-origin allow-popups"
      title={title}
      style={{
        width: '100%',
        minHeight: MIN_BODY_HEIGHT,
        border: 'none',
        display: 'block',
        // 正文强制白底（见 MAIL_BODY_CSS），深色主题下给它一个卡片边界，
        // 免得一整块白直接贴在深色面板上
        borderRadius: 8,
        background: '#fff',
        // ⚠ 不要设 overflow:hidden——那等同 scrolling="no"。
        // 高度测量对某些邮件结构可能偏小，iframe 自身的滚动是最后一道兜底，
        // 禁掉它就会把内容彻底锁死在一个小格子里，连滚都滚不动。
        visibility: measured ? 'visible' : 'hidden',
      }}
      onLoad={handleLoad}
    />
  )
}

/** 从附件 content_type 或文件名推断显示用的短类型标签（最多 3 字符） */
function attachTypeLabel(filename: string, contentType: string): string {
  // 优先从 content_type 取
  const mime = contentType.toLowerCase()
  if (mime.startsWith('image/')) return 'IMG'
  if (mime === 'application/pdf') return 'PDF'
  if (mime.includes('word') || mime.includes('document')) return 'DOC'
  if (mime.includes('sheet') || mime.includes('excel')) return 'XLS'
  if (mime.includes('zip') || mime.includes('compress')) return 'ZIP'
  // 从文件名扩展名取
  const ext = filename.split('.').pop() ?? ''
  return ext.slice(0, 3).toUpperCase() || 'FIL'
}

/** 从 localStorage 读取"默认加载远程图片"设置 */
function getRemoteImageDefault(): boolean {
  return localStorage.getItem('flymail_load_remote_images') === 'true'
}

// ── 正文组件 ─────────────────────────────────────────────

interface MessageBodyProps {
  detail: MessageDetail
}

/**
 * 一封邮件的正文区（不含发件人头与工具栏）。
 *
 * 单封视图与会话手风琴共用：前者一屏一个实例，后者每个展开项一个实例。
 */
export function MessageBody({ detail }: MessageBodyProps) {
  const { t } = useTranslation()
  const [showImages, setShowImages] = useState(() => getRemoteImageDefault())
  // 引用默认折叠：一条 10 封的会话里，同样的历史文字会被带十遍
  const [showQuote, setShowQuote] = useState(false)

  // 先改写 cid 内联图引用，再做远程图拦截，最后标记引用容器
  // cidHtml：保留 cid 改写、跳过远程图拦截（showImages=true 时使用）
  // processedHtml：cid 改写 + 远程图拦截（showImages=false 时使用）
  const htmlBody = detail.html_body ?? ''
  const msgId = detail.id
  const attachments = detail.attachments
  const { processedHtml, blockedCount, cidHtml } = useMemo(() => {
    const atts = attachments ?? []
    if (!htmlBody) return { processedHtml: '', blockedCount: 0, cidHtml: '' }
    const replaced = rewriteCidLinks(htmlBody, msgId, atts)
    const { html, blocked } = blockRemoteImages(replaced)
    // 「显示远程内容」关闭时，外链样式表和远程图片一起挡掉
    return { processedHtml: stripRemoteLinks(html), blockedCount: blocked, cidHtml: replaced }
  }, [htmlBody, msgId, attachments])

  const baseHtml = showImages ? cidHtml : processedHtml
  const { html: markedHtml, hasQuote: htmlHasQuote } = useMemo(
    () => markHtmlQuotes(baseHtml),
    [baseHtml],
  )

  // 纯文本正文的引用切分（只有没有 HTML 正文时才用到）
  const textBody = detail.text_body ?? ''
  const textSplit = useMemo(() => splitTextQuote(textBody), [textBody])

  const hasQuote = htmlBody ? htmlHasQuote : textSplit.quoted.length > 0

  // 非内联附件列表（保留原始索引以对应后端 :idx 参数）
  const visibleAttachments = attachments
    ?.map((att, idx) => ({ att, idx }))
    .filter(({ att }) => !att.is_inline) ?? []

  return (
    <div className="thread-body">
      {/* 远程图拦截提示条 */}
      {blockedCount > 0 && !showImages && (
        <div className="remote-img-bar">
          <span style={{ flex: 1 }}>{t('reader.showImages')}</span>
          <button
            type="button"
            className="pill-btn"
            onClick={() => setShowImages(true)}
            style={{ color: 'var(--accent-ink)' }}
          >
            {t('reader.showImagesBtn')}
          </button>
        </div>
      )}

      {/* HTML 正文：沙箱 iframe，防止脚本/样式逃逸。
          ⚠ key 用 detail.id 而不是外部的 messageId——两者在切换邮件的瞬间并不相等：
          messageId 已指向新邮件，detail 还是 keepPreviousData 留下的上一封。
          用 messageId 会渲染出"新 key 配旧内容"：组件立刻重挂载并开始加载旧 HTML，
          几毫秒后 srcdoc 被就地换成新 HTML，前一次加载随之中止——
          而中止的加载不触发 load，正文就永远停在隐藏状态。
          跟随 detail.id 则保证每个实例从挂载到卸载只加载一份文档。 */}
      {htmlBody ? (
        <MailBodyFrame
          key={detail.id}
          html={markedHtml}
          allowRemote={showImages}
          foldQuote={hasQuote && !showQuote}
          title={detail.subject}
        />
      ) : textBody ? (
        <>
          {/* 纯文本：按段落分割渲染 */}
          {(showQuote ? textBody : textSplit.visible).split('\n').map((line, i) =>
            line.trim() === '' ? null : (
              // eslint-disable-next-line react/no-array-index-key
              <p key={i} style={{ whiteSpace: 'pre-wrap' }}>
                {line}
              </p>
            ),
          )}
        </>
      ) : (
        <p style={{ color: 'var(--ink-3)' }}>{t('reader.noBody')}</p>
      )}

      {/* 引用折叠开关：HTML 与纯文本共用一个入口。
          折叠态下 HTML 走 CSS 隐藏、纯文本走字符串切分，对用户是同一个动作。 */}
      {hasQuote && (
        <button
          type="button"
          className="quote-toggle"
          onClick={() => setShowQuote((o) => !o)}
          aria-expanded={showQuote}
        >
          <Icon name={showQuote ? 'chevron-up' : 'more'} size={13} />
          <span>{showQuote ? t('reader.quote.hide') : t('reader.quote.show')}</span>
        </button>
      )}

      {/* 附件卡片 */}
      {visibleAttachments.length > 0 && (
        <div className="thread-attach">
          {visibleAttachments.map(({ att, idx }) => {
            const previewable = isPreviewable(att)
            const typeLabel = attachTypeLabel(att.filename, att.content_type)
            return (
              <div
                key={`${att.filename}-${idx}`}
                className="attach-card"
                style={{ cursor: 'pointer' }}
                onClick={() => void downloadAttachment(detail.id, idx, att.filename)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    void downloadAttachment(detail.id, idx, att.filename)
                  }
                }}
              >
                {/* 类型角标 */}
                <div className="ac-ic">{typeLabel}</div>
                <div>
                  <div className="ac-name">{att.filename}</div>
                  <div className="ac-meta">{formatBytes(att.size)}</div>
                </div>
                {/* 可预览时额外展示预览链接 */}
                {previewable && (
                  <a
                    href={attachmentUrl(detail.id, idx)}
                    target="_blank"
                    rel="noopener noreferrer"
                    // 阻止点击冒泡到外层 onClick（下载）
                    onClick={(e) => e.stopPropagation()}
                    style={{
                      marginLeft: 6,
                      fontSize: 11.5,
                      color: 'var(--accent-ink)',
                      textDecoration: 'none',
                    }}
                  >
                    {t('reader.preview')}
                  </a>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Star } from 'lucide-react'
import { useMessageDetail, useMarkRead, useToggleFlag, useDeleteMessage, useMoveMessage, useFolders } from '@/lib/queries'
import type { Address, MessageDetail } from '@/lib/types'
import {
  attachmentUrl,
  downloadAttachment,
  isPreviewable,
  rewriteCidLinks,
} from '@/lib/attachments'
import { Icon } from '@/components/ui/Icon'
import { formatBytes } from '@/lib/format'
import { isDesktop, openExternal } from '@/lib/platform'

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

/** 把邮件 HTML 包成一个自带基础样式的完整文档，供 iframe srcDoc 使用 */
function wrapBodyHtml(html: string, allowRemote: boolean): string {
  // base target="_blank" 是兜底：正常路径是父窗口拦截点击后交给系统浏览器（见 bindExternalLinks），
  // 万一拦截未生效，也不至于让链接把 iframe 里的邮件内容顶掉。
  return (
    `<meta charset="utf-8">${cspMeta(allowRemote)}` +
    `<base target="_blank"><style>${MAIL_BODY_CSS}</style>${html}`
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
  /** 已做过 cid 改写 / 远程内容拦截的邮件 HTML */
  html: string
  title: string
  /** 用户已选择显示远程内容：放宽 CSP，允许外部图片/字体 */
  allowRemote: boolean
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
function MailBodyFrame({ html, title, allowRemote }: MailBodyFrameProps) {
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
      srcDoc={wrapBodyHtml(html, allowRemote)}
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

/** 格式化日期字符串 */
function formatDate(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleString()
  } catch {
    return dateStr
  }
}

/** 把 Address 数组渲染为 "name <email>" 逗号连接字符串 */
function formatAddresses(addrs: Address[]): string {
  return addrs
    .map((a) => (a.name ? `${a.name} <${a.email}>` : a.email))
    .join(', ')
}

/** 取发件人首字母（用于方形头像） */
function senderInitial(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
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

// ── 骨架屏 ────────────────────────────────────────────────

/**
 * 加载中的骨架占位。
 *
 * ⚠ 根节点必须与正常态一样是 .col.reader：它带着 flex:1 1 auto / min-width:300px，
 * 换成普通 div 会退回 flex:0 1 auto（宽度由内容决定），第三栏在
 * 「骨架 → 正文」之间先塌缩再弹回，表现为每次点开邮件整个布局抖一下。
 */
function ReaderSkeleton() {
  return (
    <section className="col reader animate-pulse" style={{ background: 'var(--bg)' }}>
      {/* 工具条骨架 */}
      <div
        className="reader-toolbar"
        style={{ borderBottom: '1px solid var(--rule)', background: 'var(--surface)' }}
      >
        {[60, 60, 50, 70].map((w, i) => (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={i}
            className="h-7 rounded-md"
            style={{ width: w, background: 'var(--bg-sunk)' }}
          />
        ))}
      </div>
      {/* 正文骨架 */}
      <div className="reader-scroll">
        <div className="reader-inner">
          {/* 主题 */}
          <div className="h-8 rounded mb-5" style={{ width: '55%', background: 'var(--bg-sunk)' }} />
          {/* thread-head */}
          <div className="flex items-center gap-3 mb-5">
            <div className="h-10 w-10 rounded-lg flex-shrink-0" style={{ background: 'var(--bg-sunk)' }} />
            <div className="flex flex-col gap-2 flex-1">
              <div className="h-3.5 rounded" style={{ width: 140, background: 'var(--bg-sunk)' }} />
              <div className="h-3 rounded" style={{ width: 200, background: 'var(--bg-sunk)' }} />
            </div>
            <div className="h-3 rounded" style={{ width: 80, background: 'var(--bg-sunk)' }} />
          </div>
          {/* 正文行 */}
          {[90, 75, 88, 65, 80].map((w, i) => (
            <div
              // eslint-disable-next-line react/no-array-index-key
              key={i}
              className="h-3.5 rounded mb-3"
              style={{ width: `${w}%`, background: 'var(--bg-sunk)' }}
            />
          ))}
        </div>
      </div>
    </section>
  )
}

// ── 主组件 Props ─────────────────────────────────────────

interface ReaderProps {
  messageId: number | null
  onReply?: (d: MessageDetail) => void
  onForward?: (d: MessageDetail) => void
  /** 删除/移动成功后回调（用于清空当前选中邮件） */
  onClose?: () => void
}

/** 从 localStorage 读取"默认加载远程图片"设置 */
function getRemoteImageDefault(): boolean {
  return localStorage.getItem('flymail_load_remote_images') === 'true'
}

/**
 * flag 连续为 true 超过 delay 毫秒后才返回 true。
 *
 * 用于抑制瞬时 loading 造成的闪烁：本地接口通常几毫秒返回，
 * 立刻挂骨架屏只会让人看到一两帧的跳变，比不显示更糟。
 */
function useDelayedFlag(flag: boolean, delay: number): boolean {
  const [on, setOn] = useState(false)
  useEffect(() => {
    if (!flag) {
      setOn(false)
      return
    }
    const timer = setTimeout(() => setOn(true), delay)
    return () => clearTimeout(timer)
  }, [flag, delay])
  return on
}

// ── 主组件 ───────────────────────────────────────────────

export function Reader({ messageId, onReply, onForward, onClose }: ReaderProps) {
  const { t } = useTranslation()
  const [showImages, setShowImages] = useState(() => getRemoteImageDefault())
  // 移动到文件夹的下拉菜单开关
  const [moveOpen, setMoveOpen] = useState(false)

  // messageId 切换时，将 showImages 重置为 localStorage 中的默认值
  useEffect(() => {
    setShowImages(getRemoteImageDefault())
  }, [messageId])

  const { data: detail, isLoading, isError, error } = useMessageDetail(messageId)
  const toggleFlag = useToggleFlag()
  const markRead = useMarkRead()
  const deleteMessage = useDeleteMessage()
  const moveMessage = useMoveMessage()
  // 移动目标：当前邮件所属账户的文件夹（detail 未就绪时为 null）
  const { data: accountFolders = [] } = useFolders(detail?.account_id ?? null)

  // 删除当前邮件（移到回收站/永久删除由后端判定），成功后清空选中
  function handleDelete() {
    if (messageId == null) return
    if (!window.confirm(t('reader.deleteConfirm'))) return
    deleteMessage.mutate(messageId, { onSuccess: () => onClose?.() })
  }

  // 移动当前邮件到目标文件夹，成功后清空选中
  function handleMove(folderId: number) {
    if (messageId == null) return
    setMoveOpen(false)
    moveMessage.mutate({ id: messageId, folderId }, { onSuccess: () => onClose?.() })
  }

  // 先改写 cid 内联图引用，再做远程图拦截
  // cidHtml：保留 cid 改写、跳过远程图拦截（showImages=true 时使用）
  // processedHtml：cid 改写 + 远程图拦截（showImages=false 时使用）
  const htmlBody = detail?.html_body ?? ''
  const msgId = detail?.id ?? 0
  const { processedHtml, blockedCount, cidHtml } = useMemo(() => {
    const atts = detail?.attachments ?? []
    if (!htmlBody) return { processedHtml: '', blockedCount: 0, cidHtml: '' }
    const replaced = rewriteCidLinks(htmlBody, msgId, atts)
    const { html, blocked } = blockRemoteImages(replaced)
    // 「显示远程内容」关闭时，外链样式表和远程图片一起挡掉
    return { processedHtml: stripRemoteLinks(html), blockedCount: blocked, cidHtml: replaced }
  }, [htmlBody, msgId, detail?.attachments])

  // ── 加载态：宁可短暂留住上一封，也不要闪一帧骨架 ──────────
  // keepPreviousData 让切换瞬间仍有内容可渲染，但那是上一封邮件（id 对不上）。
  // 只有当这个错位状态持续超过 150ms（正文要现从服务器抓）才切骨架屏；
  // 本地已有正文时几毫秒就换好了，全程不闪。
  const stale = detail != null && detail.id !== messageId
  const showSkeleton = useDelayedFlag(isLoading || stale, 150)


  // ── 空态：未选中邮件 ──────────────────────────────────
  if (messageId == null) {
    return (
      <section className="col reader">
        <div className="reader-empty">
          <div className="empty-inner">
            <h3>{t('reader.welcome')}</h3>
            <p>{t('reader.welcomeHint')}</p>
            {/* 快捷键提示表 */}
            <div className="shortcuts">
              <kbd>J / K</kbd><span>{t('reader.scKbNav')}</span>
              <kbd>C</kbd><span>{t('reader.scKbCompose')}</span>
              <kbd>R</kbd><span>{t('reader.scKbReply')}</span>
              <kbd>S</kbd><span>{t('reader.scKbStar')}</span>
              <kbd>⌘K</kbd><span>{t('reader.scKbSearch')}</span>
            </div>
          </div>
        </div>
      </section>
    )
  }

  // ── 加载中：骨架屏（仅在慢加载时出现，见 showSkeleton）──────
  if (showSkeleton) {
    return <ReaderSkeleton />
  }
  // 首次打开且还没有任何数据可渲染：给一块同尺寸的空栏占位，
  // 150ms 内数据到达就直接出内容，超时才由 showSkeleton 换成骨架。
  if (!detail && !isError) {
    return <section className="col reader" />
  }

  // ── 加载失败 ──────────────────────────────────────────
  if (isError) {
    const msg = error instanceof Error ? error.message : String(error ?? '')
    return (
      <section className="col reader">
        <div className="reader-empty">
          <div className="empty-inner">
            <h3 style={{ color: 'var(--ink-2)' }}>{t('reader.loadError')}</h3>
            <p>{t('reader.loadErrorHint')}</p>
            {msg && (
              <p
                style={{
                  marginTop: 8,
                  fontFamily: 'var(--font-mono)',
                  fontSize: 11,
                  color: 'var(--ink-4)',
                  wordBreak: 'break-all',
                }}
              >
                {msg}
              </p>
            )}
          </div>
        </div>
      </section>
    )
  }

  // detail 此时保证非 null
  if (!detail) return null

  // showImages=true：用 cidHtml（不拦截远程图）
  // showImages=false：用 processedHtml（拦截远程图）
  const bodyHtml = showImages ? cidHtml : processedHtml

  // 非内联附件列表（保留原始索引以对应后端 :idx 参数）
  const visibleAttachments = detail.attachments
    ?.map((att, idx) => ({ att, idx }))
    .filter(({ att }) => !att.is_inline) ?? []

  // 发件人信息
  const senderName = detail.from_name || detail.from_addr
  const initial = senderInitial(detail.from_name, detail.from_addr)

  // 收件人 / 抄送显示文字
  const toText = formatAddresses(detail.to ?? [])
  const ccText = detail.cc && detail.cc.length > 0 ? formatAddresses(detail.cc) : ''

  return (
    <section className="col reader">
      {/* ── 工具条 ───────────────────────────────────────
           stale 期间（屏幕上还是上一封、messageId 已指向新邮件）整条禁用点击，
           否则会出现"看着旧邮件、把操作打到新邮件上"。 */}
      <div
        className="reader-toolbar"
        style={stale ? { pointerEvents: 'none' } : undefined}
      >
        {/* 回复 */}
        {onReply && (
          <button
            type="button"
            className="tb-btn"
            onClick={() => onReply(detail)}
            title={t('reader.reply')}
          >
            <Icon name="reply" size={14} />
            <span>{t('reader.reply')}</span>
          </button>
        )}
        {/* 转发 */}
        {onForward && (
          <button
            type="button"
            className="tb-btn"
            onClick={() => onForward(detail)}
            title={t('reader.forward')}
          >
            <Icon name="forward" size={14} />
            <span>{t('reader.forward')}</span>
          </button>
        )}

        <div className="tb-sep" />

        {/* 星标切换 */}
        <button
          type="button"
          className="tb-btn"
          onClick={() => toggleFlag.mutate({ id: messageId, flagged: !detail.flagged })}
          title={detail.flagged ? t('reader.unstar') : t('reader.star')}
          style={detail.flagged ? { color: 'var(--accent-color)' } : undefined}
        >
          {/* 用 lucide Star 保留填充效果，与 Icon 组件并存 */}
          <Star
            size={14}
            fill={detail.flagged ? 'var(--accent-color)' : 'none'}
            stroke={detail.flagged ? 'var(--accent-color)' : 'currentColor'}
          />
          <span>{detail.flagged ? t('reader.unstar') : t('reader.star')}</span>
        </button>

        {/* 标为未读 */}
        <button
          type="button"
          className="tb-btn"
          onClick={() => markRead.mutate({ id: messageId, read: false })}
          title={t('reader.markUnread')}
        >
          <Icon name="inbox" size={14} />
          <span>{t('reader.markUnread')}</span>
        </button>

        <div className="tb-sep" />

        {/* 移动到文件夹（下拉） */}
        <div className="tb-menu-wrap">
          <button
            type="button"
            className="tb-btn"
            onClick={() => setMoveOpen((o) => !o)}
            title={t('reader.move')}
            disabled={moveMessage.isPending}
          >
            <Icon name="folder" size={14} />
            <span>{t('reader.move')}</span>
          </button>
          {moveOpen && (
            <>
              {/* 点击空白处关闭的透明遮罩 */}
              <div
                onClick={() => setMoveOpen(false)}
                style={{ position: 'fixed', inset: 0, zIndex: 40 }}
              />
              <div
                style={{
                  position: 'absolute',
                  top: '100%',
                  left: 0,
                  marginTop: 4,
                  zIndex: 41,
                  minWidth: 180,
                  maxHeight: 280,
                  overflowY: 'auto',
                  background: 'var(--surface)',
                  border: '1px solid var(--rule)',
                  borderRadius: 8,
                  boxShadow: '0 8px 24px rgba(0,0,0,0.12)',
                  padding: 4,
                }}
              >
                {accountFolders
                  .filter((f) => f.selectable && f.id !== detail.folder_id)
                  .map((f) => (
                    <button
                      key={f.id}
                      type="button"
                      onClick={() => handleMove(f.id)}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        width: '100%',
                        padding: '7px 10px',
                        border: 'none',
                        background: 'transparent',
                        borderRadius: 6,
                        fontSize: 13,
                        color: 'var(--ink)',
                        cursor: 'pointer',
                        textAlign: 'left',
                      }}
                      onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-alt)' }}
                      onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                    >
                      <Icon name="folder" size={13} />
                      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)}
                      </span>
                    </button>
                  ))}
              </div>
            </>
          )}
        </div>

        {/* 删除 */}
        <button
          type="button"
          className="tb-btn"
          onClick={handleDelete}
          title={t('reader.delete')}
          disabled={deleteMessage.isPending}
          style={{ color: 'var(--destructive)' }}
        >
          <Icon name="trash" size={14} />
          <span>{t('reader.delete')}</span>
        </button>
      </div>

      {/* ── 正文滚动区 ──────────────────────────────────── */}
      <div className="reader-scroll">
        <div className="reader-inner">
          {/* 主题大标题 */}
          <h1 className="reader-subject">
            {detail.subject || t('list.noSubject')}
          </h1>

          {/* meta 行：收件人账号/日期等小标签 */}
          <div className="reader-meta-row">
            <span className="mi-tag" style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
              <span
                className="dot"
                style={{
                  width: 7,
                  height: 7,
                  borderRadius: '50%',
                  background: 'var(--accent-color)',
                  display: 'inline-block',
                }}
              />
              {formatDate(detail.date)}
            </span>
          </div>

          {/* ── 单封消息 thread-msg ──────────────────────── */}
          <div className="thread-msg">
            {/* 消息头：头像 + 发件人 + 时间 */}
            <div className="thread-head">
              {/* 方形首字母头像 */}
              <div
                className="avatar-sq"
                style={{ background: 'var(--accent-color)', color: 'white' }}
              >
                {initial}
              </div>

              {/* 发件人 + 收件人 */}
              <div>
                <div className="th-from">
                  {senderName}
                  {detail.from_name && (
                    <span
                      style={{
                        color: 'var(--ink-3)',
                        fontWeight: 400,
                        fontSize: 12,
                        fontFamily: 'var(--font-mono)',
                        marginLeft: 6,
                      }}
                    >
                      &lt;{detail.from_addr}&gt;
                    </span>
                  )}
                </div>
                <div className="th-to">
                  {t('reader.sendTo')} {toText}
                  {ccText && (
                    <span style={{ marginLeft: 6 }}>
                      · {t('reader.cc')} {ccText}
                    </span>
                  )}
                </div>
              </div>

              {/* 时间戳 */}
              <div className="th-time">{formatDate(detail.date)}</div>
            </div>

            {/* 消息正文 */}
            <div className="thread-body">
              {/* 远程图拦截提示条 */}
              {blockedCount > 0 && !showImages && (
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    padding: '8px 12px',
                    marginBottom: 12,
                    borderRadius: 8,
                    background: 'var(--accent-wash)',
                    border: '1px solid var(--rule)',
                    fontSize: 12.5,
                    color: 'var(--ink-2)',
                  }}
                >
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
                  ⚠ key 用 detail.id 而不是 messageId——两者在切换邮件的瞬间并不相等：
                  messageId 已指向新邮件，detail 还是 keepPreviousData 留下的上一封。
                  用 messageId 会渲染出"新 key 配旧内容"：组件立刻重挂载并开始加载旧 HTML，
                  几毫秒后 srcdoc 被就地换成新 HTML，前一次加载随之中止——
                  而中止的加载不触发 load，正文就永远停在隐藏状态。
                  跟随 detail.id 则保证每个实例从挂载到卸载只加载一份文档。 */}
              {detail.html_body ? (
                <MailBodyFrame
                  key={detail.id}
                  html={bodyHtml}
                  allowRemote={showImages}
                  title={detail.subject}
                />
              ) : detail.text_body ? (
                // 纯文本：按段落分割渲染
                detail.text_body.split('\n').map((line, i) =>
                  line.trim() === '' ? null : (
                    // eslint-disable-next-line react/no-array-index-key
                    <p key={i} style={{ whiteSpace: 'pre-wrap' }}>
                      {line}
                    </p>
                  ),
                )
              ) : (
                <p style={{ color: 'var(--ink-3)' }}>{t('reader.noBody')}</p>
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
          </div>
          {/* 底部回复框已移除：入口在顶部工具栏已有一份，占着正文空间不划算 */}
        </div>
      </div>
    </section>
  )
}

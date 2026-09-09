// 单封邮件的正文渲染：远程内容拦截条 + HTML 沙箱 iframe / 纯文本 + 引用折叠 + 附件卡片。
//
// 从 Reader.tsx 抽出来的唯一理由是会话手风琴：一条会话里每个展开项都要渲染一份正文，
// 而正文渲染带着 CSP、远程内容拦截、iframe 高度测量这一整套不能复制第二份的逻辑。
// 单封视图与会话视图从此共用同一份实现，改一处两边同时生效。
//
// M12 之后净化与远程内容拦截都在服务端完成（internal/htmlsan）：这里不再有任何
// 正则剥离，只负责按 remote_count / remote_allowed 展示横幅，以及把用户的选择
// 翻译成「带 remote=1 重新取一份详情」。iframe 的沙箱与文档构建见 lib/mail-frame.ts。
//
// ⚠ 调用方必须以 key={detail.id} 挂载：showRemote / showQuote 是「这一封」的状态，
// 换邮件不重置就会把上一封的选择带过去。

import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { apiErrorMessage } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { openExternal } from '@/lib/platform'
import {
  MAIL_FRAME_SANDBOX,
  MIN_BODY_HEIGHT,
  buildFrameDocument,
  isFrameEvent,
  parseFrameMessage,
  secureRandomHex,
} from '@/lib/mail-frame'
import { QUOTE_HIDE_CSS, markHtmlQuotes, splitTextQuote } from '@/lib/quote-fold'
import { useAddTrustedSender, useMessageDetail } from '@/lib/queries'
import type { MessageDetail } from '@/lib/types'
import {
  attachmentUrl,
  downloadAttachment,
  isPreviewable,
  rewriteCidLinks,
} from '@/lib/attachments'

interface MailBodyFrameProps {
  /** 服务端已净化、前端做过 cid 改写与引用标记的邮件 HTML */
  html: string
  title: string
  /** 服务端已放行远程引用：放宽 CSP，允许外部图片与字体 */
  allowRemote: boolean
  /** 折叠引用：注入一条把 [data-fm-quote] 藏起来的样式 */
  foldQuote: boolean
  /** 正文里点到 mailto: 链接时的回调（打开应用自己的撰写器） */
  onMailto?: (href: string) => void
}

/**
 * HTML 正文的沙箱 iframe，负责把自己的高度贴合内容。
 *
 * ⚠ 必须由调用方以 key={detail.id} 挂载 —— 一份文档一个实例。
 *
 * 这一点是这个组件存在的全部理由：高度、"是否已量准"这些状态，
 * 归属的是**某一份 iframe 文档**，而不是 Reader。此前它们放在 Reader 里，
 * 就得手工在换邮件时重置，而重置（useEffect）和 iframe 的事件都是异步的、
 * 没有确定顺序，正文于是会永久隐藏——表现为"切换几次之后就不显示了"。
 *
 * M12 的改动集中在一点：iframe 不再同源（见 MAIL_FRAME_SANDBOX 的说明），
 * 父窗口拿不到 contentDocument，高度与链接改由文档内的注入脚本 postMessage 上报。
 * 这里只剩「收消息 → 校验 → 设高度 / 开链接」三件事。
 */
export function MailBodyFrame({ html, title, allowRemote, foldQuote, onMailto }: MailBodyFrameProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  // 首次测量完成前先藏起来（仍占位，不引起外层跳动），
  // 免得看到"小框闪一下再弹到正确高度"。
  const [measured, setMeasured] = useState(false)
  // token 与 nonce 一份实例一套：token 让同屏的多份正文（会话手风琴）各管各的框，
  // nonce 让 CSP 只放行我们注入的那一段脚本。切换引用折叠只是换 srcDoc，沿用同一套即可。
  // 拿不到密码学随机源时两者为 null，buildFrameDocument 会整段不注入脚本
  // （见 secureRandomHex 的说明），高度由下面的 400ms 兜底负责。
  const [frameIds] = useState(() => ({ token: secureRandomHex(16), nonce: secureRandomHex(12) }))

  // 回调放进 ref：message 监听器只在 token 变化时重建（实际是永不重建），
  // 不能因为父组件每次渲染传来新的函数引用就把监听器拆了重装。
  const onMailtoRef = useRef(onMailto)
  useEffect(() => {
    onMailtoRef.current = onMailto
  }, [onMailto])

  useEffect(() => {
    /**
     * @param rebase true = 文档刚加载完（允许变矮）；false = 同一份文档内的持续校正，
     *   只允许增高——迟到的回调可能量到尚未填上内容的文档，
     *   放任它缩小就会把已经正确的高度打回最小值。
     */
    function applyHeight(height: number, rebase: boolean) {
      const el = iframeRef.current
      if (!el) return
      const target = Math.max(Math.ceil(height) + 8, MIN_BODY_HEIGHT)
      const current = el.offsetHeight
      if (rebase ? Math.abs(target - current) > 1 : target > current + 1) {
        el.style.height = `${target}px`
      }
      setMeasured(true)
    }

    function onMessage(ev: MessageEvent) {
      // ⚠ 来源校验只能比窗口引用：iframe 是不透明源，ev.origin 恒为字符串 "null"，
      // 拿它做判断等于谁都能冒充。窗口引用比较不受不透明源影响。
      if (!isFrameEvent(ev, iframeRef.current?.contentWindow ?? null)) return
      const msg = parseFrameMessage(ev.data, frameIds.token)
      if (!msg) return
      if (msg.kind === 'height') {
        applyHeight(msg.height, msg.rebase)
      } else if (msg.kind === 'mailto') {
        onMailtoRef.current?.(msg.href)
      } else {
        // 外链交给系统浏览器：桌面端里直接导航会把整个应用页面顶成外站
        openExternal(msg.href)
      }
    }

    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [frameIds.token])

  useEffect(() => {
    // 兜底：注入脚本没能跑起来（上层 CSP 更严、脚本异常、环境不支持）时，
    // 正文不能就此永远隐藏。到点无条件显示，高度停在 MIN_BODY_HEIGHT——
    // iframe 自身的滚动条保证内容仍然读得到，而"永远不显示"是最坏的失败模式。
    const timer = setTimeout(() => setMeasured(true), 400)
    return () => clearTimeout(timer)
  }, [])

  return (
    <iframe
      ref={iframeRef}
      srcDoc={buildFrameDocument({
        html,
        allowRemote,
        foldQuote,
        quoteHideCss: QUOTE_HIDE_CSS,
        nonce: frameIds.nonce,
        token: frameIds.token,
      })}
      // ⚠⚠ 绝不允许在这里加回 allow-same-origin：它与 allow-scripts 同时出现
      // 就等于把沙箱拆掉（邮件脚本可读 parent.document 与 localStorage 里的 token）。
      // 完整理由写在 lib/mail-frame.ts 的 MAIL_FRAME_SANDBOX 注释里。
      sandbox={MAIL_FRAME_SANDBOX}
      title={title}
      // iframe 的 load 事件挂在父文档的元素上，跨源同样会触发：
      // 只用来兜底解除隐藏，真实高度一律等注入脚本的消息。
      onLoad={() => setMeasured(true)}
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

// ── 正文组件 ─────────────────────────────────────────────

interface MessageBodyProps {
  detail: MessageDetail
  /** 正文里点到 mailto: 链接时的回调；不传则该链接静默无效 */
  onMailto?: (href: string) => void
}

/**
 * 一封邮件的正文区（不含发件人头与工具栏）。
 *
 * 单封视图与会话手风琴共用：前者一屏一个实例，后者每个展开项一个实例。
 */
export function MessageBody({ detail, onMailto }: MessageBodyProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  // 用户在这一封上点了「显示图片」。默认值不看本地开关——
  // 「默认显示远程图片」已经由 useMessageDetail 翻译成请求上的 remote=1，
  // 开关打开时详情回来就是 remote_allowed=true，这里不需要也不应该再判一次。
  const [showRemote, setShowRemote] = useState(false)
  // 引用默认折叠：一条 10 封的会话里，同样的历史文字会被带十遍
  const [showQuote, setShowQuote] = useState(false)
  const [trustError, setTrustError] = useState<string | null>(null)
  const addTrusted = useAddTrustedSender()

  // 点了「显示图片」之后带 remote=1 再取一份：净化在服务端，
  // 「放行远程引用」这件事只有服务端说了算，前端没有一份保留了远程地址的正文可用。
  const remoteQuery = useMessageDetail(
    showRemote && !detail.remote_allowed ? detail.id : null,
    { remote: true },
  )
  // ⚠ 比一次 id：useMessageDetail 带 keepPreviousData，拿到的可能是上一封的残留
  const remoteDetail = remoteQuery.data
  const view = remoteDetail && remoteDetail.id === detail.id ? remoteDetail : detail

  // 内联 cid 图引用改写成本地附件接口；远程内容此时已由服务端处理完毕。
  // ⚙ 必须用 attachment_token：改写结果直接落进那份由发件人控制的文档，
  // 带 access token 的话可被邮件自带的 <style> 用属性选择器逐字符问出去（见 attachments.ts）。
  const htmlBody = view.html_body ?? ''
  const msgId = view.id
  const attachments = view.attachments
  const attachToken = view.attachment_token
  const preparedHtml = useMemo(() => {
    if (!htmlBody) return ''
    return rewriteCidLinks(htmlBody, msgId, attachments ?? [], attachToken)
  }, [htmlBody, msgId, attachments, attachToken])

  const { html: markedHtml, hasQuote: htmlHasQuote } = useMemo(
    () => markHtmlQuotes(preparedHtml),
    [preparedHtml],
  )

  // 纯文本正文的引用切分（只有没有 HTML 正文时才用到）
  const textBody = view.text_body ?? ''
  const textSplit = useMemo(() => splitTextQuote(textBody), [textBody])

  const hasQuote = htmlBody ? htmlHasQuote : textSplit.quoted.length > 0

  // 非内联附件列表（保留原始索引以对应后端 :idx 参数）
  const visibleAttachments = attachments
    ?.map((att, idx) => ({ att, idx }))
    .filter(({ att }) => !att.is_inline) ?? []

  // 服务端报告有远程引用、且这一封尚未放行 → 显示拦截横幅
  const blockedRemote = view.remote_count > 0 && !view.remote_allowed
  const senderAddr = view.from_addr?.trim() ?? ''

  function handleAlwaysShow() {
    if (!senderAddr) return
    setTrustError(null)
    addTrusted.mutate(senderAddr, {
      onSuccess: (res) => {
        // 无论是刚加进去还是本来就在名单里，用户要的都是「现在把图显示出来」
        setShowRemote(true)
        if (!res.existed) toast(t('reader.remote.trusted', { address: senderAddr }))
      },
      onError: (e) => setTrustError(apiErrorMessage(e, t('reader.remote.trustFailed'))),
    })
  }

  return (
    <div className="thread-body">
      {/* 远程内容拦截提示条 */}
      {blockedRemote && (
        <div className="remote-img-bar">
          <span style={{ flex: 1 }}>
            {t('reader.remote.blocked', { n: view.remote_count })}
          </span>
          <button
            type="button"
            className="pill-btn"
            onClick={() => setShowRemote(true)}
            disabled={remoteQuery.isFetching}
            style={{ color: 'var(--accent-ink)' }}
          >
            {t('reader.remote.show')}
          </button>
          {senderAddr && (
            <button
              type="button"
              className="pill-btn"
              onClick={handleAlwaysShow}
              disabled={addTrusted.isPending}
              title={t('reader.remote.alwaysHint', { address: senderAddr })}
            >
              {t('reader.remote.always')}
            </button>
          )}
        </div>
      )}
      {trustError && (
        <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginBottom: 10 }}>
          {trustError}
        </div>
      )}

      {/* HTML 正文：沙箱 iframe，防止脚本/样式逃逸。
          ⚠ key 用 view.id 而不是外部的 messageId——两者在切换邮件的瞬间并不相等：
          messageId 已指向新邮件，detail 还是 keepPreviousData 留下的上一封。
          用 messageId 会渲染出"新 key 配旧内容"：组件立刻重挂载并开始加载旧 HTML，
          几毫秒后 srcdoc 被就地换成新 HTML，前一次加载随之中止。
          跟随 view.id 则保证每个实例从挂载到卸载只处理一份文档。 */}
      {htmlBody ? (
        <MailBodyFrame
          key={view.id}
          html={markedHtml}
          allowRemote={view.remote_allowed}
          foldQuote={hasQuote && !showQuote}
          title={view.subject}
          onMailto={onMailto}
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
                onClick={() => void downloadAttachment(view.id, idx, att.filename)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    void downloadAttachment(view.id, idx, att.filename)
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
                    href={attachmentUrl(view.id, idx, { token: view.attachment_token })}
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

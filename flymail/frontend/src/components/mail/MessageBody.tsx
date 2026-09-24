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

import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { useConfirm } from '@/components/ui/Confirm'
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
import { getDarkBody, subscribePrivacyPrefs } from '@/lib/privacy-prefs'
import { getThemeMode, subscribeThemeMode } from '@/lib/theme'
import { QUOTE_HIDE_CSS, markHtmlQuotes, splitTextQuote } from '@/lib/quote-fold'
import { useAddTrustedSender, useMessageDetail, useMessageTranslation, useTranslateLanguages } from '@/lib/queries'
import type { MessageDetail, Translation } from '@/lib/types'
import {
  attachmentUrl,
  downloadAttachment,
  isPreviewable,
  rewriteCidLinks,
} from '@/lib/attachments'

/**
 * 附件卡超过这个数就折叠。
 *
 * 群发的会议纪要动辄带一二十个附件，无条件全量渲染会把正文顶到屏幕外——
 * 用户要滚过一整屏卡片才能看到邮件正文。6 张刚好占一屏的一小部分。
 */
const ATTACH_FOLD_AT = 6

/**
 * 超过这个大小的附件，下载前先问一句。
 *
 * 下载没有进度表达（blob 一次性拿完才落盘），200MB 的附件在慢网络上就是
 * 「点了之后十几分钟毫无动静」。移动网络下更是直接烧流量。
 */
const LARGE_ATTACH_BYTES = 50 * 1024 * 1024

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

  // 暗化正文：用户在隐私设置里打开、**且**当前处于暗色模式时才生效。
  //
  // 两个都必须是订阅式的：暗化样式随 srcDoc 一次性注入 iframe，
  // 组件不重新渲染就不会重建文档——用户在设置面板里拨了开关或切了亮暗，
  // 已经打开的那封邮件会停在旧样子，而他正看着它。
  const darkPref = useSyncExternalStore(subscribePrivacyPrefs, getDarkBody, () => false)
  const themeMode = useSyncExternalStore(
    subscribeThemeMode,
    getThemeMode,
    () => 'light' as const, // SSR/快照时按亮色，反相是加法不是默认
  )
  const darkBody = darkPref && themeMode === 'dark'

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
        darkBody,
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
/** 译文出处：「配置名 / 模型」；旧译文没有配置名时只给模型名 */
function translatedByLabel(tr: Pick<Translation, 'provider' | 'model'>): string {
  return tr.provider ? `${tr.provider} / ${tr.model}` : tr.model
}

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
  /**
   * 译文。非空时正文区整体显示这一份，而不是原文。
   *
   * 走"替换 view"这条路而不是另起一套渲染：远程图拦截、cid 改写、引用折叠、
   * iframe 高度测量这一整套逻辑对译文一字不差地同样适用，复制第二份的下场
   * 是两份实现慢慢长歪。
   */
  translation?: Translation | null
  /** 正在翻译（还没有译文可显示） */
  translating?: boolean
  /** 翻译失败的原因；非空时正文上方显示一条可重试的提示 */
  translateError?: string | null
  /** 重试 / 重新翻译（force），由调用方决定是否真的再花一次钱 */
  onRetranslate?: () => void
  /** 切回原文 */
  onShowOriginal?: () => void
}

/**
 * 一封邮件的正文区（不含发件人头与工具栏）。
 *
 * 单封视图与会话手风琴共用：前者一屏一个实例，后者每个展开项一个实例。
 */
export function MessageBody({
  detail,
  onMailto,
  translation,
  translating,
  translateError,
  onRetranslate,
  onShowOriginal,
}: MessageBodyProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const confirm = useConfirm()
  // 用户在这一封上点了「显示图片」。默认值不看本地开关——
  // 「默认显示远程图片」已经由 useMessageDetail 翻译成请求上的 remote=1，
  // 开关打开时详情回来就是 remote_allowed=true，这里不需要也不应该再判一次。
  const [showRemote, setShowRemote] = useState(false)
  // 引用默认折叠：一条 10 封的会话里，同样的历史文字会被带十遍
  const [showQuote, setShowQuote] = useState(false)
  const [trustError, setTrustError] = useState<string | null>(null)
  const addTrusted = useAddTrustedSender()
  // 语言清单只为把 source_lang 显示成人看得懂的名字；staleTime 是 Infinity，
  // 每个正文实例都调它不会产生额外请求。
  const { data: languages } = useTranslateLanguages()

  // 点了「显示图片」之后带 remote=1 再取一份：净化在服务端，
  // 「放行远程引用」这件事只有服务端说了算，前端没有一份保留了远程地址的正文可用。
  const remoteQuery = useMessageDetail(
    showRemote && !detail.remote_allowed ? detail.id : null,
    { remote: true },
  )
  // ⚠ 比一次 id：useMessageDetail 带 keepPreviousData，拿到的可能是上一封的残留
  const remoteDetail = remoteQuery.data
  const original = remoteDetail && remoteDetail.id === detail.id ? remoteDetail : detail

  // 译文也要跟着「显示图片」走：译文的远程引用同样是服务端按请求参数净化的，
  // 拦住的那一版里图片地址已经换成了占位符，前端手里没有能还原的东西。
  const translationRemoteQuery = useMessageTranslation(
    showRemote && translation != null && !translation.remote_allowed ? detail.id : null,
    translation?.target_lang ?? '',
    { remote: true },
  )
  const remoteTranslation = translationRemoteQuery.data
  const shownTranslation =
    remoteTranslation && remoteTranslation.message_id === detail.id ? remoteTranslation : translation

  // 译文视图 = 原文的结构与附件 + 译文的主题和正文。
  //
  // remote_count / remote_allowed 必须一并取译文那份：拦截横幅问的是
  // "现在屏幕上这份正文里有几个远程引用"，而不是原文里有几个。
  const view: MessageDetail = shownTranslation
    ? {
        ...original,
        subject: shownTranslation.subject || original.subject,
        text_body: shownTranslation.text_body,
        html_body: shownTranslation.html_body,
        remote_count: shownTranslation.remote_count,
        remote_allowed: shownTranslation.remote_allowed,
      }
    : original

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

  const [attachExpanded, setAttachExpanded] = useState(false)
  /** 正在下载的那个附件的原始索引；null = 没有下载在进行（驱动界面） */
  const [downloadingIdx, setDownloadingIdx] = useState<number | null>(null)
  // 同一个值的同步副本，专供闸门判断。
  // ⚠ 不能用上面那个 state 当闸门：它读的是闭包里的快照，同一 tick 内的两次
  // 调用都会通过——那不是互斥，只是"看起来像互斥"。
  const downloadingRef = useRef<number | null>(null)
  const folded = !attachExpanded && visibleAttachments.length > ATTACH_FOLD_AT
  const shownAttachments = folded ? visibleAttachments.slice(0, ATTACH_FOLD_AT) : visibleAttachments

  /**
   * 下载一个附件。
   *
   * 原先是 `void downloadAttachment(...)`——异常被整个吞掉，下载失败（令牌过期、
   * 附件已被服务端清理、网络断）时界面上**毫无反应**，与「点了没生效」无法区分。
   */
  async function handleDownload(att: { filename: string; size: number }, idx: number) {
    if (downloadingRef.current != null) return // 一次一个，避免重复点击叠加
    if (att.size > LARGE_ATTACH_BYTES) {
      const ok = await confirm({
        title: t('reader.largeAttach', { size: formatBytes(att.size) }),
        body: t('reader.largeAttachBody'),
        confirmLabel: t('reader.download'),
      })
      if (!ok) return
      // 再判一次：等确认框的这段时间闸门是开着的，用户可能已经点了别的附件
      // （小附件不走确认框，直接就开始下了）。不判的话这里会把它顶掉，
      // 而它下完之后的 finally 又会把本次的忙碌态清掉——大附件下载中却显示
      // 文件大小、aria-busy 提前消失，读屏不再报忙碌。
      if (downloadingRef.current != null) return
    }
    downloadingRef.current = idx
    setDownloadingIdx(idx)
    try {
      await downloadAttachment(view.id, idx, att.filename)
    } catch (e) {
      toast(apiErrorMessage(e, t('reader.downloadFailed')))
    } finally {
      // 只清自己那一次：万一上面的判断仍有漏网，也不会清掉别人的忙碌态
      if (downloadingRef.current === idx) {
        downloadingRef.current = null
        setDownloadingIdx(null)
      }
    }
  }

  // 源语言显示成自称名（「英语」而不是 en）。清单还没回来时就不显示语言，
  // 退回"由 AI 翻译"那句——宁可少说一句，也不要在提示条上露出一个语言代码。
  const sourceLangName = shownTranslation?.source_lang
    ? (languages?.languages ?? []).find((l) => l.code === shownTranslation.source_lang)?.native ?? ''
    : ''

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
      {/* 翻译状态条：进行中 / 失败 / 正在看译文，三者互斥。
          放在正文**上方**而不是工具栏上：工具栏那颗按钮只表达"看哪一版"，
          而"从哪门语言翻的、是不是只翻了一部分、失败了能不能重试"都是
          关于这份正文的说明，跟着正文走才读得懂。 */}
      {translating ? (
        <div className="translated-bar">
          <span className="tr-note">{t('reader.translating')}</span>
        </div>
      ) : translateError ? (
        <div className="translated-bar is-error">
          <span className="tr-note">{translateError}</span>
          {onRetranslate && (
            <button type="button" className="pill-btn" onClick={onRetranslate}>
              {t('reader.translateRetry')}
            </button>
          )}
        </div>
      ) : shownTranslation ? (
        <div className="translated-bar">
          <span className="tr-note">
            {/* 配了多条 AI 配置、自动切换过时，得让用户看出这份是哪条线路翻的；
                旧译文没有 provider 字段，只显示模型名。 */}
            {sourceLangName
              ? t('reader.translatedFrom', { lang: sourceLangName, model: translatedByLabel(shownTranslation) })
              : t('reader.translatedBy', { model: translatedByLabel(shownTranslation) })}
            {shownTranslation.partial && ' ' + t('reader.translatePartial')}
          </span>
          {onShowOriginal && (
            <button type="button" className="pill-btn" onClick={onShowOriginal}>
              {t('reader.showOriginal')}
            </button>
          )}
          {onRetranslate && (
            <button type="button" className="pill-btn" onClick={onRetranslate}>
              {t('reader.translateRedo')}
            </button>
          )}
        </div>
      ) : null}

      {/* 远程内容拦截提示条 */}
      {blockedRemote && (
        <div className="remote-img-bar">
          <span className="remote-img-note">
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
          {shownAttachments.map(({ att, idx }) => {
            const previewable = isPreviewable(att)
            const typeLabel = attachTypeLabel(att.filename, att.content_type)
            const downloading = downloadingIdx === idx
            return (
              <div
                key={`${att.filename}-${idx}`}
                className="attach-card"
                style={{ cursor: 'pointer' }}
                onClick={() => void handleDownload(att, idx)}
                role="button"
                tabIndex={0}
                aria-busy={downloading}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    // 空格在 role="button" 上要拦掉默认行为，否则按一下
                    // 既触发下载又把页面滚下去一屏
                    e.preventDefault()
                    void handleDownload(att, idx)
                  }
                }}
              >
                {/* 类型角标 */}
                <div className="ac-ic">{typeLabel}</div>
                <div className="ac-text">
                  {/* title：文件名由发件人给，截断之后只能靠它看到全名 */}
                  <div className="ac-name" title={att.filename}>{att.filename}</div>
                  <div className="ac-meta">
                    {downloading ? t('reader.downloading') : formatBytes(att.size)}
                  </div>
                </div>
                {/* 可预览时额外展示预览链接 */}
                {previewable && (
                  <a
                    href={attachmentUrl(view.id, idx, view.attachment_token)}
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

          {/* 折叠开关。群发纪要带一二十个附件时，无条件全量渲染会把正文顶到
              屏幕外——用户得滚过一整屏卡片才看得到邮件本身。 */}
          {visibleAttachments.length > ATTACH_FOLD_AT && (
            <button
              type="button"
              className="attach-more"
              onClick={() => setAttachExpanded((o) => !o)}
              aria-expanded={attachExpanded}
            >
              <Icon name={attachExpanded ? 'chevron-up' : 'more'} size={13} />
              <span>
                {attachExpanded
                  ? t('reader.attachShowLess')
                  : t('reader.attachShowAll', { n: visibleAttachments.length })}
              </span>
            </button>
          )}
        </div>
      )}
    </div>
  )
}

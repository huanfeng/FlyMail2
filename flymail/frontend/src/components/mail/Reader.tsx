import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Download,
  Eye,
  Forward,
  Mail,
  Paperclip,
  Reply,
  Star,
  AlertCircle,
  FileText,
} from 'lucide-react'
import { useMessageDetail, useToggleFlag } from '@/lib/queries'
import type { Address, MessageDetail } from '@/lib/types'
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

/** 把字节数格式化为 KB / MB 字符串 */
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
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

/** 取发件人首字母（用于头像） */
function senderInitial(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

// ── 骨架屏 ────────────────────────────────────────────────

/** 加载中的骨架占位，替代简陋的"加载中…"文案 */
function ReaderSkeleton() {
  return (
    <div className="flex h-full flex-col animate-pulse">
      {/* 头部骨架 */}
      <div
        className="px-6 py-5 flex-shrink-0"
        style={{ borderBottom: '1px solid var(--rule)', background: 'var(--surface)' }}
      >
        {/* 主题占位 */}
        <div
          className="h-6 rounded mb-4"
          style={{ width: '60%', background: 'var(--bg-sunk)' }}
        />
        {/* 发件人行占位 */}
        <div className="flex items-center gap-3 mb-3">
          <div
            className="h-9 w-9 rounded-lg flex-shrink-0"
            style={{ background: 'var(--bg-sunk)' }}
          />
          <div className="flex flex-col gap-1.5">
            <div className="h-3.5 rounded" style={{ width: 120, background: 'var(--bg-sunk)' }} />
            <div className="h-3 rounded" style={{ width: 180, background: 'var(--bg-sunk)' }} />
          </div>
        </div>
        {/* 收件人 / 日期占位 */}
        <div className="h-3 rounded mb-1.5" style={{ width: '45%', background: 'var(--bg-sunk)' }} />
        <div className="h-3 rounded" style={{ width: '25%', background: 'var(--bg-sunk)' }} />
      </div>
      {/* 正文骨架 */}
      <div className="flex-1 px-6 py-5 flex flex-col gap-3">
        {[80, 95, 70, 90, 60].map((w, i) => (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={i}
            className="h-3.5 rounded"
            style={{ width: `${w}%`, background: 'var(--bg-sunk)' }}
          />
        ))}
      </div>
    </div>
  )
}

// ── 子组件：附件卡片 ─────────────────────────────────────

interface AttachmentItemProps {
  messageId: number
  /** 附件在 detail.attachments 数组中的原始索引，对应后端 :idx 参数。 */
  idx: number
  filename: string
  contentType: string
  size: number
  downloadLabel: string
  previewLabel: string
}

function AttachmentItem({
  messageId,
  idx,
  filename,
  contentType,
  size,
  downloadLabel,
  previewLabel,
}: AttachmentItemProps) {
  // 构造临时对象仅用于 isPreviewable 判断
  const previewable = isPreviewable({ filename, content_type: contentType, size, is_inline: false })

  return (
    <div
      className="flex items-center gap-3 rounded-lg px-3 py-2.5"
      style={{
        background: 'var(--surface)',
        border: '1px solid var(--rule)',
        boxShadow: 'var(--shadow-sm)',
      }}
    >
      {/* 文件图标 */}
      <div
        className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md"
        style={{ background: 'var(--bg-alt)' }}
      >
        <FileText size={15} style={{ color: 'var(--ink-3)' }} />
      </div>

      {/* 文件名 + 大小 */}
      <div className="min-w-0 flex-1">
        <p
          className="truncate text-[13px] font-medium leading-snug"
          style={{ color: 'var(--ink)' }}
        >
          {filename}
        </p>
        <p className="text-[11px] leading-snug" style={{ color: 'var(--ink-3)' }}>
          {formatBytes(size)}
        </p>
      </div>

      {/* 预览链接：仅图片/PDF 显示，token 走 query 参数 */}
      {previewable && (
        <a
          href={attachmentUrl(messageId, idx)}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1 rounded-md px-2.5 py-1.5 text-[12px] transition-colors"
          style={{
            color: 'var(--accent-ink)',
            background: 'var(--accent-wash)',
            textDecoration: 'none',
          }}
          aria-label={previewLabel}
        >
          <Eye size={13} />
          <span>{previewLabel}</span>
        </a>
      )}

      {/* 下载按钮：通过 axios Bearer 头下载，不暴露 token 到 URL */}
      <button
        type="button"
        onClick={() => void downloadAttachment(messageId, idx, filename)}
        className="flex items-center gap-1 rounded-md px-2.5 py-1.5 text-[12px] transition-colors"
        style={{
          color: 'var(--ink-2)',
          background: 'var(--bg-alt)',
          border: '1px solid var(--rule)',
        }}
        aria-label={downloadLabel}
        onMouseEnter={(e) => {
          ;(e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
        }}
        onMouseLeave={(e) => {
          ;(e.currentTarget as HTMLElement).style.background = 'var(--bg-alt)'
        }}
      >
        <Download size={13} />
        <span>{downloadLabel}</span>
      </button>
    </div>
  )
}

// ── 操作图标按钮 ─────────────────────────────────────────

interface IconBtnProps {
  onClick: () => void
  title: string
  children: React.ReactNode
  active?: boolean
}

function IconBtn({ onClick, title, children, active }: IconBtnProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      className="flex items-center justify-center rounded-md p-2 transition-colors outline-none focus-visible:ring-2"
      style={{
        color: active ? 'var(--accent-color)' : 'var(--ink-3)',
        background: 'transparent',
      }}
      onMouseEnter={(e) => {
        ;(e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
      }}
      onMouseLeave={(e) => {
        ;(e.currentTarget as HTMLElement).style.background = 'transparent'
      }}
    >
      {children}
    </button>
  )
}

// ── 主组件 ───────────────────────────────────────────────

interface ReaderProps {
  messageId: number | null
  onReply?: (d: MessageDetail) => void
  onForward?: (d: MessageDetail) => void
}

function getRemoteImageDefault(): boolean {
  return localStorage.getItem('flymail_load_remote_images') === 'true'
}

export function Reader({ messageId, onReply, onForward }: ReaderProps) {
  const { t } = useTranslation()
  const [showImages, setShowImages] = useState(() => getRemoteImageDefault())

  // messageId 切换时，将 showImages 重置为 localStorage 中的默认值
  useEffect(() => {
    setShowImages(getRemoteImageDefault())
  }, [messageId])

  const { data: detail, isLoading, isError, error } = useMessageDetail(messageId)
  const toggleFlag = useToggleFlag()

  // 先改写 cid 内联图引用为同源相对路径 /api/...，再做远程图拦截。
  // blockRemoteImages 只匹配 https?:// 外链，不会误伤 /api/... 相对路径。
  // cidHtml：保留 cid 改写、跳过远程图拦截（showImages=true 时使用）。
  // processedHtml：cid 改写 + 远程图拦截（showImages=false 时使用）。
  const htmlBody = detail?.html_body ?? ''
  const msgId = detail?.id ?? 0
  const { processedHtml, blockedCount, cidHtml } = useMemo(() => {
    // attachments 在 memo 内取，避免外部 ?? [] 每次渲染产生新引用
    const atts = detail?.attachments ?? []
    if (!htmlBody) return { processedHtml: '', blockedCount: 0, cidHtml: '' }
    const replaced = rewriteCidLinks(htmlBody, msgId, atts)
    const { html, blocked } = blockRemoteImages(replaced)
    return { processedHtml: html, blockedCount: blocked, cidHtml: replaced }
  }, [htmlBody, msgId, detail?.attachments])

  // ── 占位：未选中邮件 ──
  if (messageId == null) {
    return (
      <div
        className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center"
        style={{ background: 'var(--bg)' }}
      >
        {/* 大图标：淡色装饰 */}
        <div
          className="flex h-16 w-16 items-center justify-center rounded-2xl"
          style={{ background: 'var(--bg-alt)' }}
        >
          <Mail size={28} style={{ color: 'var(--ink-4)' }} />
        </div>
        <div>
          <p className="text-[14px] font-medium" style={{ color: 'var(--ink-2)' }}>
            {t('reader.welcome')}
          </p>
          <p className="mt-1 text-[12px]" style={{ color: 'var(--ink-4)' }}>
            {t('reader.welcomeHint')}
          </p>
        </div>
      </div>
    )
  }

  // ── 加载中：骨架屏 ──
  if (isLoading || (!detail && !isError)) {
    return <ReaderSkeleton />
  }

  // ── 加载失败（抓正文需连 IMAP，失败时显示错误而非永久加载）──
  if (isError) {
    const msg = error instanceof Error ? error.message : String(error ?? '')
    return (
      <div
        className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center"
        style={{ background: 'var(--bg)' }}
      >
        <div
          className="flex h-14 w-14 items-center justify-center rounded-2xl"
          style={{ background: 'var(--bg-alt)' }}
        >
          <AlertCircle size={24} style={{ color: 'var(--ink-3)' }} />
        </div>
        <div>
          <p className="text-[14px] font-medium" style={{ color: 'var(--ink)' }}>
            {t('reader.loadError')}
          </p>
          <p className="mt-1 text-[12px]" style={{ color: 'var(--ink-3)' }}>
            {msg}
          </p>
        </div>
      </div>
    )
  }

  // detail 此时保证非 null（isLoading=false && isError=false && detail 存在）
  if (!detail) return null

  // ── 确定正文 HTML ──
  // showImages=true：用 cidHtml（cid 改写完毕，不拦截远程图）
  // showImages=false：用 processedHtml（cid 改写 + 远程图拦截）
  const bodyHtml = showImages ? cidHtml : processedHtml

  // 非内联附件列表
  const visibleAttachments = detail.attachments?.filter((a) => !a.is_inline) ?? []

  // ── 详情视图 ──
  return (
    <div
      className="flex h-full flex-col overflow-auto"
      style={{ background: 'var(--bg)' }}
    >
      {/* ── 头部 ─────────────────────────────────────────── */}
      <div
        className="flex-shrink-0 px-6 pt-5 pb-4"
        style={{
          borderBottom: '1px solid var(--rule)',
          background: 'var(--surface)',
        }}
      >
        {/* 主题 + 操作工具条（同一行） */}
        <div className="flex items-start justify-between gap-3 mb-4">
          {/* 主题：更大字重，--ink */}
          <h1
            className="text-xl font-semibold leading-snug flex-1 min-w-0"
            style={{ color: 'var(--ink)' }}
          >
            {detail.subject || t('list.noSubject')}
          </h1>

          {/* 操作工具条：回复/转发/星标，图标按钮，尺寸统一 */}
          <div className="flex flex-shrink-0 items-center gap-0.5 -mr-1">
            {onReply && (
              <IconBtn onClick={() => onReply(detail)} title={t('compose.reply')}>
                <Reply size={17} />
              </IconBtn>
            )}
            {onForward && (
              <IconBtn onClick={() => onForward(detail)} title={t('compose.forward')}>
                <Forward size={17} />
              </IconBtn>
            )}
            <IconBtn
              onClick={() => toggleFlag.mutate({ id: messageId, flagged: !detail.flagged })}
              title={detail.flagged ? t('reader.unstar') : t('reader.star')}
              active={detail.flagged}
            >
              <Star
                size={17}
                fill={detail.flagged ? 'var(--accent-color)' : 'none'}
                stroke={detail.flagged ? 'var(--accent-color)' : 'var(--ink-3)'}
              />
            </IconBtn>
          </div>
        </div>

        {/* 发件人行：圆形头像 + 名称 / 邮箱 / 日期 */}
        <div className="flex items-center gap-3 mb-3">
          {/* 首字母圆形头像，背景 --accent-color 白字 */}
          <div
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full text-[13px] font-semibold text-white"
            style={{ background: 'var(--accent-color)' }}
          >
            {senderInitial(detail.from_name, detail.from_addr)}
          </div>

          {/* 发件人信息 */}
          <div className="min-w-0 flex-1">
            <p className="text-[13.5px] font-medium leading-snug truncate" style={{ color: 'var(--ink)' }}>
              {detail.from_name || detail.from_addr}
            </p>
            {detail.from_name && (
              <p className="text-[12px] leading-snug truncate" style={{ color: 'var(--ink-3)' }}>
                {detail.from_addr}
              </p>
            )}
          </div>

          {/* 日期：右对齐，次级文字 */}
          <p className="flex-shrink-0 text-[12px]" style={{ color: 'var(--ink-3)' }}>
            {formatDate(detail.date)}
          </p>
        </div>

        {/* 收件人 / 抄送：小字次级，单行截断 */}
        <div className="flex flex-col gap-0.5">
          <p className="text-[12px] truncate" style={{ color: 'var(--ink-3)' }}>
            <span style={{ color: 'var(--ink-4)' }}>{t('reader.to')}：</span>
            {formatAddresses(detail.to)}
          </p>
          {detail.cc && detail.cc.length > 0 && (
            <p className="text-[12px] truncate" style={{ color: 'var(--ink-3)' }}>
              <span style={{ color: 'var(--ink-4)' }}>CC：</span>
              {formatAddresses(detail.cc)}
            </p>
          )}
        </div>
      </div>

      {/* 图片拦截提示条 */}
      {blockedCount > 0 && !showImages && (
        <div
          className="flex flex-shrink-0 items-center gap-3 px-6 py-2"
          style={{
            background: 'var(--accent-wash)',
            borderBottom: '1px solid var(--rule)',
            fontSize: 13,
            color: 'var(--ink-2)',
          }}
        >
          <span className="flex-1 text-[12.5px]">{t('reader.showImages')}</span>
          <button
            type="button"
            onClick={() => setShowImages(true)}
            className="flex-shrink-0 rounded-md px-3 py-1 text-[12px] transition-colors"
            style={{
              border: '1px solid var(--rule-strong)',
              color: 'var(--accent-ink)',
              background: 'var(--surface)',
            }}
          >
            {t('reader.showImagesBtn')}
          </button>
        </div>
      )}

      {/* ── 正文区域：留白 + 最大宽度居中 ──────────────── */}
      <div
        className="flex-1 overflow-hidden"
        style={{ minHeight: 0 }}
      >
        {detail.html_body ? (
          <iframe
            srcDoc={bodyHtml}
            sandbox=""
            title={detail.subject}
            style={{
              width: '100%',
              height: '100%',
              minHeight: 400,
              border: 'none',
              display: 'block',
            }}
          />
        ) : detail.text_body ? (
          <div
            className="px-6 py-5 text-[14px] leading-relaxed whitespace-pre-wrap max-w-[760px]"
            style={{ color: 'var(--ink)' }}
          >
            {detail.text_body}
          </div>
        ) : (
          <div className="px-6 py-5 text-[14px]" style={{ color: 'var(--ink-3)' }}>
            {t('reader.noBody')}
          </div>
        )}
      </div>

      {/* ── 附件区：卡片化，内联图不展示 ────────────────── */}
      {visibleAttachments.length > 0 && (
        <div
          className="flex-shrink-0 px-6 pt-4 pb-5"
          style={{
            borderTop: '1px solid var(--rule)',
            background: 'var(--bg-alt)',
          }}
        >
          {/* 附件区标题 */}
          <div className="flex items-center gap-1.5 mb-3">
            <Paperclip size={13} style={{ color: 'var(--ink-3)' }} />
            <p
              className="text-[11.5px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--ink-3)' }}
            >
              {t('reader.attachments')} ({visibleAttachments.length})
            </p>
          </div>

          {/* 附件卡片网格 */}
          <div className="flex flex-col gap-2">
            {detail.attachments!.map((att, idx) => {
              // 内联图不在附件列表展示，保留原始 idx 以对应后端 :idx 参数
              if (att.is_inline) return null
              return (
                <AttachmentItem
                  key={`${att.filename}-${idx}`}
                  messageId={detail.id}
                  idx={idx}
                  filename={att.filename}
                  contentType={att.content_type}
                  size={att.size}
                  downloadLabel={t('reader.download')}
                  previewLabel={t('reader.preview')}
                />
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

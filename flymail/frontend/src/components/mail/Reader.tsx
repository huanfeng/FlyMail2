import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Forward, Paperclip, Reply, Star } from 'lucide-react'
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

// ── 子组件：附件列表项 ────────────────────────────────────

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
  // 构造一个临时对象仅用于 isPreviewable 判断，避免重复传附件对象。
  const previewable = isPreviewable({ filename, content_type: contentType, size, is_inline: false })

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        padding: '6px 0',
        borderBottom: '1px solid var(--rule)',
        color: 'var(--ink-2)',
        fontSize: 13,
      }}
    >
      <Paperclip size={14} style={{ flexShrink: 0, color: 'var(--ink-3)' }} />
      <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {filename}
      </span>
      <span style={{ color: 'var(--ink-3)', fontSize: 12, flexShrink: 0 }}>
        {formatBytes(size)}
      </span>
      {/* 下载按钮：通过 axios Bearer 头下载，不暴露 token 到 URL */}
      <button
        onClick={() => void downloadAttachment(messageId, idx, filename)}
        style={{
          background: 'none',
          border: '1px solid var(--rule)',
          borderRadius: 4,
          padding: '2px 8px',
          fontSize: 12,
          cursor: 'pointer',
          color: 'var(--ink)',
          flexShrink: 0,
        }}
      >
        {downloadLabel}
      </button>
      {/* 预览链接：仅图片/PDF 显示，token 走 query 参数 */}
      {previewable && (
        <a
          href={attachmentUrl(messageId, idx)}
          target="_blank"
          rel="noopener noreferrer"
          style={{
            fontSize: 12,
            color: 'var(--accent-color)',
            flexShrink: 0,
            textDecoration: 'none',
          }}
        >
          {previewLabel}
        </a>
      )}
    </div>
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

  // 当 messageId 变化时重置 showImages（依赖 messageId）
  // 用 key 在父层重置更优雅，但这里用 useMemo 惰性处理足矣
  //
  // 先改写 cid: 内联图引用为同源相对路径 /api/...，再做远程图拦截。
  // blockRemoteImages 只匹配 https?:// 外链，不会误伤 /api/... 相对路径，顺序互不干扰。
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
        className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center"
      >
        <p className="text-sm" style={{ color: 'var(--ink-2)' }}>
          {t('reader.welcome')}
        </p>
        <p className="text-xs" style={{ color: 'var(--ink-3)' }}>
          {t('reader.notReady')}
        </p>
      </div>
    )
  }

  // ── 加载失败（抓正文需连 IMAP，失败时显示错误而非永久加载）──
  if (isError) {
    const msg = error instanceof Error ? error.message : String(error ?? '')
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
        <p className="text-sm" style={{ color: 'var(--ink)' }}>{t('reader.loadError')}</p>
        <p className="text-xs" style={{ color: 'var(--ink-3)' }}>{msg}</p>
      </div>
    )
  }

  // ── 加载中 ──
  if (isLoading || !detail) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--ink-3)' }}>
          {t('reader.loading')}
        </p>
      </div>
    )
  }

  // ── 确定正文 HTML ──
  // showImages=true：用 cidHtml（cid 改写完毕，不拦截远程图）
  // showImages=false：用 processedHtml（cid 改写 + 远程图拦截）
  const bodyHtml = showImages ? cidHtml : processedHtml

  // ── 详情视图 ──
  return (
    <div
      style={{
        height: '100%',
        overflow: 'auto',
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--bg)',
      }}
    >
      {/* 头部 */}
      <div
        style={{
          padding: '20px 24px 16px',
          borderBottom: '1px solid var(--rule)',
          background: 'var(--surface)',
          position: 'relative',
        }}
      >
        {/* 操作按钮区（右上角）：回复、转发、星标 */}
        <div
          style={{
            position: 'absolute',
            top: 20,
            right: 20,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          {onReply && (
            <button
              onClick={() => onReply(detail)}
              title={t('compose.reply')}
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: 4,
                lineHeight: 0,
                color: 'var(--ink-3)',
              }}
            >
              <Reply size={18} />
            </button>
          )}
          {onForward && (
            <button
              onClick={() => onForward(detail)}
              title={t('compose.forward')}
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: 4,
                lineHeight: 0,
                color: 'var(--ink-3)',
              }}
            >
              <Forward size={18} />
            </button>
          )}
          <button
            onClick={() =>
              toggleFlag.mutate({ id: messageId, flagged: !detail.flagged })
            }
            title={detail.flagged ? 'Unstar' : 'Star'}
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              padding: 4,
              lineHeight: 0,
            }}
          >
            <Star
              size={18}
              fill={detail.flagged ? 'var(--accent-color)' : 'none'}
              stroke={detail.flagged ? 'var(--accent-color)' : 'var(--ink-3)'}
            />
          </button>
        </div>

        {/* 主题 */}
        <p
          className="text-xl font-medium"
          style={{
            color: 'var(--ink)',
            marginBottom: 10,
            paddingRight: 32,
            lineHeight: 1.4,
          }}
        >
          {detail.subject || t('list.noSubject')}
        </p>

        {/* 发件人 */}
        <p style={{ fontSize: 13, color: 'var(--ink-2)', marginBottom: 4 }}>
          {detail.from_name
            ? `${detail.from_name} <${detail.from_addr}>`
            : detail.from_addr}
        </p>

        {/* 收件人 */}
        <p style={{ fontSize: 13, color: 'var(--ink-2)', marginBottom: 4 }}>
          <span style={{ color: 'var(--ink-3)' }}>{t('reader.to')}：</span>
          {formatAddresses(detail.to)}
        </p>

        {/* 抄送（若有） */}
        {detail.cc && detail.cc.length > 0 && (
          <p style={{ fontSize: 13, color: 'var(--ink-2)', marginBottom: 4 }}>
            <span style={{ color: 'var(--ink-3)' }}>CC：</span>
            {formatAddresses(detail.cc)}
          </p>
        )}

        {/* 日期 */}
        <p style={{ fontSize: 12, color: 'var(--ink-3)' }}>
          {formatDate(detail.date)}
        </p>
      </div>

      {/* 图片拦截提示条 */}
      {blockedCount > 0 && !showImages && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '8px 24px',
            background: 'var(--accent-wash)',
            borderBottom: '1px solid var(--rule)',
            fontSize: 13,
            color: 'var(--ink-2)',
          }}
        >
          <span style={{ flex: 1 }}>{t('reader.showImages')}</span>
          <button
            onClick={() => setShowImages(true)}
            style={{
              background: 'none',
              border: '1px solid var(--rule)',
              borderRadius: 4,
              padding: '2px 10px',
              fontSize: 12,
              cursor: 'pointer',
              color: 'var(--ink)',
            }}
          >
            {t('reader.showImagesBtn')}
          </button>
        </div>
      )}

      {/* 正文区域 */}
      <div style={{ flex: 1, padding: '0', overflow: 'hidden' }}>
        {detail.html_body ? (
          <iframe
            srcDoc={bodyHtml}
            sandbox=""
            title={detail.subject}
            style={{
              width: '100%',
              height: 'calc(100vh - 220px)',
              minHeight: 400,
              border: 'none',
              display: 'block',
            }}
          />
        ) : detail.text_body ? (
          <div
            style={{
              padding: '20px 24px',
              fontSize: 14,
              lineHeight: 1.7,
              color: 'var(--ink)',
              whiteSpace: 'pre-wrap',
            }}
          >
            {detail.text_body}
          </div>
        ) : (
          <div
            style={{
              padding: '20px 24px',
              fontSize: 14,
              color: 'var(--ink-3)',
            }}
          >
            {t('reader.noBody')}
          </div>
        )}
      </div>

      {/* 附件列表：内联图（is_inline）仅用于正文 cid 渲染，不在此展示 */}
      {detail.attachments && detail.attachments.some((a) => !a.is_inline) && (
        <div
          style={{
            borderTop: '1px solid var(--rule)',
            padding: '12px 24px 16px',
            background: 'var(--surface)',
          }}
        >
          <p
            style={{
              fontSize: 12,
              color: 'var(--ink-3)',
              marginBottom: 6,
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}
          >
            {t('reader.attachments')} ({detail.attachments.filter((a) => !a.is_inline).length})
          </p>
          {detail.attachments.map((att, idx) => {
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
      )}
    </div>
  )
}

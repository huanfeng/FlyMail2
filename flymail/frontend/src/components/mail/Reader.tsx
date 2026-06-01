import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Paperclip, Star } from 'lucide-react'
import { useMessageDetail, useToggleFlag } from '@/lib/queries'
import type { Address, Attachment } from '@/lib/types'

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
  attachment: Attachment
  noDownloadLabel: string
}

function AttachmentItem({ attachment, noDownloadLabel }: AttachmentItemProps) {
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
        {attachment.filename}
      </span>
      <span style={{ color: 'var(--ink-3)', fontSize: 12, flexShrink: 0 }}>
        {formatBytes(attachment.size)}
      </span>
      <span style={{ color: 'var(--ink-3)', fontSize: 12, flexShrink: 0 }}>
        {noDownloadLabel}
      </span>
    </div>
  )
}

// ── 主组件 ───────────────────────────────────────────────

interface ReaderProps {
  messageId: number | null
}

export function Reader({ messageId }: ReaderProps) {
  const { t } = useTranslation()
  const [showImages, setShowImages] = useState(false)

  const { data: detail, isLoading } = useMessageDetail(messageId)
  const toggleFlag = useToggleFlag()

  // 当 messageId 变化时重置 showImages（依赖 messageId）
  // 用 key 在父层重置更优雅，但这里用 useMemo 惰性处理足矣
  const { processedHtml, blockedCount } = useMemo(() => {
    if (!detail?.html_body) return { processedHtml: '', blockedCount: 0 }
    const { html, blocked } = blockRemoteImages(detail.html_body)
    return { processedHtml: html, blockedCount: blocked }
  }, [detail?.html_body])

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
  const bodyHtml = showImages ? detail.html_body : processedHtml

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
        {/* 星标按钮 */}
        <button
          onClick={() =>
            toggleFlag.mutate({ id: messageId, flagged: !detail.flagged })
          }
          title={detail.flagged ? 'Unstar' : 'Star'}
          style={{
            position: 'absolute',
            top: 20,
            right: 20,
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

      {/* 附件列表 */}
      {detail.attachments && detail.attachments.length > 0 && (
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
            {t('reader.attachments')} ({detail.attachments.length})
          </p>
          {detail.attachments.map((att, idx) => (
            <AttachmentItem
              key={`${att.filename}-${idx}`}
              attachment={att}
              noDownloadLabel={t('reader.attachmentNoDownload')}
            />
          ))}
        </div>
      )}
    </div>
  )
}

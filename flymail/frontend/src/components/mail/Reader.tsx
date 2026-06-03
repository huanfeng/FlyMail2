import { useEffect, useMemo, useState } from 'react'
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

/** 加载中的骨架占位 */
function ReaderSkeleton() {
  return (
    <div className="flex h-full flex-col animate-pulse" style={{ background: 'var(--bg)' }}>
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
    </div>
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
    return { processedHtml: html, blockedCount: blocked, cidHtml: replaced }
  }, [htmlBody, msgId, detail?.attachments])

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

  // ── 加载中：骨架屏 ────────────────────────────────────
  if (isLoading || (!detail && !isError)) {
    return <ReaderSkeleton />
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
      {/* ── 工具条 ─────────────────────────────────────── */}
      <div className="reader-toolbar">
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
        <div style={{ position: 'relative' }}>
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

        {/* 右侧留白（弹性占位） */}
        <div style={{ flex: 1 }} />
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

              {/* HTML 正文：沙箱 iframe，防止脚本/样式逃逸 */}
              {detail.html_body ? (
                <iframe
                  srcDoc={bodyHtml}
                  sandbox=""
                  title={detail.subject}
                  style={{
                    width: '100%',
                    minHeight: 320,
                    border: 'none',
                    display: 'block',
                    // 自适应高度：需要内容高度，用 onLoad 设置
                  }}
                  onLoad={(e) => {
                    // 让 iframe 自适应内容高度，避免滚动条嵌套
                    const el = e.currentTarget
                    try {
                      const body = el.contentDocument?.body
                      if (body) {
                        el.style.height = `${body.scrollHeight + 32}px`
                      }
                    } catch {
                      // 跨域时忽略
                    }
                  }}
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

          {/* ── 回复框 ──────────────────────────────────── */}
          <div
            className="reply-box"
            onClick={() => onReply?.(detail)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') onReply?.(detail)
            }}
          >
            <div style={{ color: 'var(--ink-3)', fontSize: 13 }}>
              {t('reader.replyTo', { name: senderName })}
            </div>
            <div className="reply-actions">
              <button
                type="button"
                className="pill-btn primary"
                onClick={(e) => {
                  e.stopPropagation()
                  onReply?.(detail)
                }}
              >
                <Icon name="reply" size={12} />
                {' '}{t('reader.reply')}
              </button>
              <button
                type="button"
                className="pill-btn"
                onClick={(e) => {
                  e.stopPropagation()
                  onForward?.(detail)
                }}
              >
                <Icon name="forward" size={12} />
                {' '}{t('reader.forward')}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

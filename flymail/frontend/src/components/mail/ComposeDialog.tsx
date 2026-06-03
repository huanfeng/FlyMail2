import * as React from 'react'
import { useTranslation } from 'react-i18next'
import Editor from 'react-simple-wysiwyg'
import type { ContentEditableEvent } from 'react-simple-wysiwyg'
import { Icon } from '@/components/ui/Icon'
import { AddressInput } from '@/components/mail/AddressInput'
import { useSend, useCreateDraft, useUpdateDraft, useDeleteDraft, useAccounts } from '@/lib/queries'
import { useToast } from '@/components/ui/Toast'
import { formatBytes } from '@/lib/format'

// 附件总大小上限（25 MiB），需与后端 maxAttachmentTotal 保持一致。
const MAX_ATTACH_TOTAL = 25 * 1024 * 1024

// ────────────────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────────────────

export interface ComposeInitial {
  to?: string[]
  cc?: string[]
  subject?: string
  bodyHtml?: string
  inReplyTo?: string
  references?: string
}

export interface ComposeDialogProps {
  open: boolean
  onOpenChange: (o: boolean) => void
  accountId: number | null
  initial?: ComposeInitial
  draftId?: number | null // 非空 = 编辑已有草稿
}

interface FormState {
  fromId: number | null
  toStr: string
  ccStr: string
  bccStr: string
  subject: string
  bodyHtml: string
}

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

function emptyForm(fromId: number | null): FormState {
  return { fromId, toStr: '', ccStr: '', bccStr: '', subject: '', bodyHtml: '' }
}

/** 按逗号或分号拆分地址，去空格和空项 */
function parseAddrs(s: string): string[] {
  return s
    .split(/[,;]/)
    .map((a) => a.trim())
    .filter(Boolean)
}

/**
 * 根据账户 id 生成点颜色（使用 CSS 变量 accent 派生色）。
 * 多账户时用 index 区分；此处简单用固定的 accent 颜色令外观一致。
 */
function acctDotColor(index: number): string {
  // 利用 CSS accent 色系，index 0 使用主色，其余旋转色调
  const hues = ['var(--accent)', '#4ade80', '#f59e0b', '#a78bfa', '#f87171']
  return hues[index % hues.length]
}

// ────────────────────────────────────────────────────────────────────────────────
// ComposeDialog — MailMaster 浮窗风格，右下固定，支持最小化
// ────────────────────────────────────────────────────────────────────────────────

export function ComposeDialog({
  open,
  onOpenChange,
  accountId,
  initial,
  draftId,
}: ComposeDialogProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { data: accounts = [] } = useAccounts()

  // ── UI state ─────────────────────────────────────────────────────────────────
  const [minimized, setMinimized] = React.useState(false)
  const [showCc, setShowCc] = React.useState(false)
  const [showBcc, setShowBcc] = React.useState(false)
  const [validationError, setValidationError] = React.useState<string | null>(null)
  const [infoMessage, setInfoMessage] = React.useState<string | null>(null)

  // ── Form state ───────────────────────────────────────────────────────────────
  const [form, setForm] = React.useState<FormState>(() => emptyForm(accountId))

  // ── 附件 ─────────────────────────────────────────────────────────────────────
  const [attachments, setAttachments] = React.useState<File[]>([])
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const attachTotal = attachments.reduce((sum, f) => sum + f.size, 0)

  function onPickFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const picked = Array.from(e.target.files ?? [])
    e.target.value = '' // 允许再次选择同一文件
    if (picked.length === 0) return
    const next = [...attachments, ...picked]
    const total = next.reduce((sum, f) => sum + f.size, 0)
    if (total > MAX_ATTACH_TOTAL) {
      setValidationError(t('compose.attachTooLarge', { size: formatBytes(MAX_ATTACH_TOTAL) }))
      return
    }
    setValidationError(null)
    setAttachments(next)
  }

  function removeAttachment(index: number) {
    setAttachments((prev) => prev.filter((_, i) => i !== index))
  }

  // 便捷 setter
  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  // ── 浮窗自由拖动（拖标题栏）─────────────────────────────────────────────────
  // pos 为 null 时用 CSS 默认右下角；拖动后切换为 left/top 绝对定位并夹紧在视口内。
  const winRef = React.useRef<HTMLDivElement>(null)
  const dragRef = React.useRef<{ dx: number; dy: number } | null>(null)
  const [pos, setPos] = React.useState<{ x: number; y: number } | null>(null)

  function onHeadPointerDown(e: React.PointerEvent) {
    // 点到标题栏按钮（最小化/关闭）时不触发拖动
    if ((e.target as HTMLElement).closest('button')) return
    const el = winRef.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    dragRef.current = { dx: e.clientX - rect.left, dy: e.clientY - rect.top }
    setPos({ x: rect.left, y: rect.top }) // 从当前位置接管，避免跳变
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  function onHeadPointerMove(e: React.PointerEvent) {
    if (!dragRef.current) return
    const el = winRef.current
    const w = el?.offsetWidth ?? 0
    const h = el?.offsetHeight ?? 0
    const x = Math.max(0, Math.min(window.innerWidth - w, e.clientX - dragRef.current.dx))
    const y = Math.max(0, Math.min(window.innerHeight - h, e.clientY - dragRef.current.dy))
    setPos({ x, y })
  }
  function onHeadPointerUp(e: React.PointerEvent) {
    dragRef.current = null
    try { e.currentTarget.releasePointerCapture(e.pointerId) } catch { /* ignore */ }
  }

  // 打开时用 initial 预填，关闭时重置 UI 状态
  React.useEffect(() => {
    if (open) {
      setValidationError(null)
      setInfoMessage(null)
      setMinimized(false)
      setPos(null) // 每次打开回到默认右下角
      setAttachments([]) // 附件不随草稿持久化，每次打开清空

      // 有抄送预填时自动展开抄送行
      const hasCc = (initial?.cc ?? []).length > 0
      setShowCc(hasCc)
      setShowBcc(false)

      const defaultFrom = accountId ?? (accounts[0]?.id ?? null)

      if (initial) {
        setForm({
          fromId: defaultFrom,
          toStr: (initial.to ?? []).join(', '),
          ccStr: (initial.cc ?? []).join(', '),
          bccStr: '',
          subject: initial.subject ?? '',
          bodyHtml: initial.bodyHtml ?? '',
        })
      } else {
        setForm(emptyForm(defaultFrom))
      }
    }
  }, [open, initial, draftId, accountId, accounts])

  // ── Mutations ────────────────────────────────────────────────────────────────
  const sendMutation = useSend()
  const createDraft = useCreateDraft()
  const updateDraft = useUpdateDraft()
  const deleteDraft = useDeleteDraft()

  // ── Derived ─────────────────────────────────────────────────────────────────
  const isSending = sendMutation.isPending
  const isSavingDraft = createDraft.isPending || updateDraft.isPending
  const isBusy = isSending || isSavingDraft
  const effectiveAccountId = form.fromId ?? accountId
  const noAccount = effectiveAccountId === null

  // 当前选中账户信息（用于 from 行显示）
  const fromAccount = accounts.find((a) => a.id === effectiveAccountId) ?? accounts[0] ?? null
  const fromAccountIndex = fromAccount ? accounts.indexOf(fromAccount) : 0

  // 多账户时允许切换，单账户只展示
  const multiAccount = accounts.length > 1

  // ── 标题：回复/转发/写邮件 ─────────────────────────────────────────────────
  function resolveTitle(): string {
    if (!initial) return t('compose.title')
    if (initial.inReplyTo) return t('compose.reply')
    if (initial.subject?.startsWith('Fwd:') || initial.subject?.startsWith('转发：')) {
      return t('compose.forward')
    }
    return t('compose.title')
  }

  const title = resolveTitle()

  // ── 发送 ─────────────────────────────────────────────────────────────────────
  function handleSend() {
    setValidationError(null)
    setInfoMessage(null)

    const toAddrs = parseAddrs(form.toStr)
    if (toAddrs.length === 0) {
      setValidationError(t('compose.toRequired'))
      return
    }
    if (noAccount) return

    const ccAddrs = parseAddrs(form.ccStr)
    const bccAddrs = parseAddrs(form.bccStr)

    sendMutation.mutate(
      {
        req: {
          account_id: effectiveAccountId as number,
          to: toAddrs,
          cc: ccAddrs.length > 0 ? ccAddrs : undefined,
          bcc: bccAddrs.length > 0 ? bccAddrs : undefined,
          subject: form.subject,
          body_html: form.bodyHtml,
          in_reply_to: initial?.inReplyTo,
          references: initial?.references,
        },
        files: attachments,
      },
      {
        onSuccess: () => {
          // 发送成功提示
          toast(t('compose.sent'))
          // 发送成功后，若正在编辑草稿则将其删除
          if (draftId != null && effectiveAccountId != null) {
            deleteDraft.mutate(
              { id: draftId, accountId: effectiveAccountId as number },
              { onSettled: () => onOpenChange(false) },
            )
          } else {
            onOpenChange(false)
          }
        },
      },
    )
  }

  // ── 存草稿 ───────────────────────────────────────────────────────────────────
  function handleSaveDraft() {
    setValidationError(null)
    setInfoMessage(null)
    if (noAccount) return

    const req = {
      account_id: effectiveAccountId as number,
      to: parseAddrs(form.toStr),
      cc: parseAddrs(form.ccStr),
      bcc: parseAddrs(form.bccStr),
      subject: form.subject,
      body_html: form.bodyHtml,
      in_reply_to: initial?.inReplyTo ?? '',
      references: initial?.references ?? '',
    }

    if (draftId != null) {
      updateDraft.mutate(
        { id: draftId, req },
        {
          onSuccess: () => {
            // 存草稿成功 toast（替代原内联提示，关闭后仍可见）
            toast(t('compose.draftSaved'))
            onOpenChange(false)
          },
        },
      )
    } else {
      createDraft.mutate(req, {
        onSuccess: () => {
          toast(t('compose.draftSaved'))
          onOpenChange(false)
        },
      })
    }
  }

  // ── onChange 富文本编辑器 ────────────────────────────────────────────────────
  function handleBodyChange(e: ContentEditableEvent) {
    set('bodyHtml', e.target.value)
  }

  // ── 切换最小化 ──────────────────────────────────────────────────────────────
  function handleToggleMinimize(e?: React.MouseEvent) {
    e?.stopPropagation()
    setMinimized((prev) => !prev)
  }

  // 未打开时不渲染任何内容
  if (!open) return null

  // ────────────────────────────────────────────────────────────────────────────
  // 最小化条 .compose-bar
  // ────────────────────────────────────────────────────────────────────────────
  if (minimized) {
    return (
      <div className="compose-bar" onClick={handleToggleMinimize}>
        <Icon name="compose" size={12} />
        <span className="cb-title">{form.subject || title}</span>
        {/* spacer */}
        <div style={{ flex: 1, minWidth: 0 }} />
        {/* 展开按钮 */}
        <button
          className="icon-btn"
          title={t('compose.minimize')}
          onClick={handleToggleMinimize}
        >
          {/* 向上箭头（展开） */}
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
            <path d="M4 10l4-4 4 4" />
          </svg>
        </button>
        {/* 关闭按钮 */}
        <button
          className="icon-btn"
          title={t('compose.cancel')}
          onClick={(e) => { e.stopPropagation(); onOpenChange(false) }}
        >
          <Icon name="close" size={12} />
        </button>
      </div>
    )
  }

  // ────────────────────────────────────────────────────────────────────────────
  // 浮窗主体 .compose-window
  // ────────────────────────────────────────────────────────────────────────────
  // 拖动后用 left/top 绝对定位；移动端(≤768)保持 CSS 全屏，不应用 pos
  const winStyle: React.CSSProperties | undefined =
    pos && window.innerWidth > 768
      ? { left: pos.x, top: pos.y, right: 'auto', bottom: 'auto' }
      : undefined

  return (
    <div
      ref={winRef}
      className="compose-window"
      style={winStyle}
      onMouseDown={(e) => e.stopPropagation()}
    >

      {/* ── 标题栏 .compose-head（可拖动）─────────────────────────────────────── */}
      <div
        className="compose-head"
        style={{ cursor: 'move', touchAction: 'none' }}
        onPointerDown={onHeadPointerDown}
        onPointerMove={onHeadPointerMove}
        onPointerUp={onHeadPointerUp}
        onPointerCancel={onHeadPointerUp}
      >
        <Icon name="compose" size={12} />
        <h3>{title}</h3>
        {/* spacer */}
        <div style={{ flex: 1 }} />
        {/* 最小化 */}
        <button
          className="icon-btn"
          title={t('compose.minimize')}
          onClick={handleToggleMinimize}
        >
          <Icon name="minus" size={12} />
        </button>
        {/* 关闭 */}
        <button
          className="icon-btn"
          title={t('compose.cancel')}
          onClick={() => onOpenChange(false)}
        >
          <Icon name="close" size={14} />
        </button>
      </div>

      {/* ── 表单主体 .compose-body ───────────────────────────────────────────── */}
      <div className="compose-body">

        {/* From 行 */}
        <div className="compose-row">
          <label>{t('compose.from')}</label>
          {multiAccount ? (
            // 多账户：可点击选择，用原生 select 套 from-account 样式
            <select
              value={form.fromId ?? ''}
              onChange={(e) => set('fromId', Number(e.target.value))}
              disabled={isBusy}
              style={{
                border: 0, outline: 0,
                background: 'var(--bg-alt)',
                padding: '4px 8px',
                borderRadius: 6,
                fontSize: 13,
                color: 'var(--ink)',
                fontFamily: 'var(--font-body)',
              }}
            >
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} — {a.email}
                </option>
              ))}
            </select>
          ) : (
            // 单账户：只读展示
            <span className="from-account">
              <span
                className="acct-dot"
                style={{ background: acctDotColor(fromAccountIndex) }}
              />
              {fromAccount?.email ?? ''}
            </span>
          )}
        </div>

        {/* To 行 */}
        <div className="compose-row">
          <label>{t('compose.to')}</label>
          <div style={{ display: 'flex', alignItems: 'center', width: '100%' }}>
            <AddressInput
              value={form.toStr}
              onChange={(v) => set('toStr', v)}
              placeholder="name@example.com"
              disabled={isBusy}
              autoFocus={!initial}
            />
            {/* 未展开抄送时显示 Cc 切换按钮 */}
            {!showCc && (
              <button
                className="pill-btn"
                style={{ marginLeft: 8, flexShrink: 0 }}
                onClick={() => setShowCc(true)}
                type="button"
              >
                {t('compose.showCc')}
              </button>
            )}
            {/* 未展开密送时显示 Bcc 切换按钮 */}
            {!showBcc && (
              <button
                className="pill-btn"
                style={{ marginLeft: 4, flexShrink: 0 }}
                onClick={() => setShowBcc(true)}
                type="button"
              >
                {t('compose.showBcc')}
              </button>
            )}
          </div>
        </div>

        {/* Cc 行（可折叠） */}
        {showCc && (
          <div className="compose-row">
            <label>{t('compose.cc')}</label>
            <AddressInput
              value={form.ccStr}
              onChange={(v) => set('ccStr', v)}
              placeholder="cc@example.com"
              disabled={isBusy}
            />
          </div>
        )}

        {/* Bcc 行（可折叠） */}
        {showBcc && (
          <div className="compose-row">
            <label>{t('compose.bcc')}</label>
            <AddressInput
              value={form.bccStr}
              onChange={(v) => set('bccStr', v)}
              placeholder="bcc@example.com"
              disabled={isBusy}
            />
          </div>
        )}

        {/* 主题行 */}
        <div className="compose-row">
          <label>{t('compose.subject')}</label>
          <input
            value={form.subject}
            onChange={(e) => set('subject', e.target.value)}
            placeholder={t('compose.subject')}
            disabled={isBusy}
            autoFocus={!!initial}
          />
        </div>

        {/* 正文：保留 react-simple-wysiwyg 富文本编辑，外观贴近 compose-textarea */}
        {/* 保留富文本的原因：回复/转发预填是 HTML 引用块，纯 textarea 会显示原始标签 */}
        <div style={{ position: 'relative' }}>
          <Editor
            value={form.bodyHtml}
            onChange={handleBodyChange}
            disabled={isBusy}
            containerProps={{
              // 用内联样式覆盖 rsw 默认边框，使其外观贴近 compose-textarea
              style: {
                border: 'none',
                background: 'transparent',
                minHeight: 240,
                fontSize: 14,
                lineHeight: '1.6',
                color: 'var(--ink)',
                fontFamily: 'var(--font-body)',
              },
            }}
          />
        </div>

        {/* 附件列表 */}
        {attachments.length > 0 && (
          <div className="compose-attachments">
            {attachments.map((f, i) => (
              <div className="attach-chip" key={`${f.name}-${i}`}>
                <Icon name="attach" size={11} />
                <span className="ac-name" title={f.name}>{f.name}</span>
                <span className="ac-size">{formatBytes(f.size)}</span>
                <button
                  className="ac-remove"
                  type="button"
                  title={t('compose.attachRemove')}
                  onClick={() => removeAttachment(i)}
                  disabled={isBusy}
                >
                  <Icon name="close" size={11} />
                </button>
              </div>
            ))}
            <span className="ac-total">{formatBytes(attachTotal)}</span>
          </div>
        )}

        {/* 校验错误提示 */}
        {validationError && (
          <div
            style={{
              padding: '6px 0',
              fontSize: 13,
              color: 'var(--destructive)',
            }}
          >
            {validationError}
          </div>
        )}

        {/* 草稿保存成功提示 */}
        {infoMessage && (
          <div
            style={{
              padding: '6px 0',
              fontSize: 13,
              color: 'var(--ink-2)',
            }}
          >
            {infoMessage}
          </div>
        )}
      </div>

      {/* ── 底部操作栏 .compose-foot ─────────────────────────────────────────── */}
      <div className="compose-foot">
        {/* 发送按钮（primary pill） */}
        <button
          className="pill-btn primary"
          onClick={handleSend}
          disabled={isBusy || noAccount}
          type="button"
        >
          {isSending ? t('compose.sending') : t('compose.send')}
        </button>

        {/* 存草稿按钮 */}
        <button
          className="pill-btn"
          onClick={handleSaveDraft}
          disabled={isBusy || noAccount}
          type="button"
        >
          {isSavingDraft ? t('compose.savingDraft') : t('compose.saveDraft')}
        </button>

        {/* 附件按钮 */}
        <button
          className="pill-btn"
          onClick={() => fileInputRef.current?.click()}
          disabled={isBusy}
          title={t('compose.attach')}
          type="button"
        >
          <Icon name="attach" size={12} />
          {attachments.length > 0 && (
            <span style={{ marginLeft: 4 }}>{attachments.length}</span>
          )}
        </button>
        {/* 隐藏的文件选择 input */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          hidden
          onChange={onPickFiles}
        />

        {/* spacer 推开右侧 */}
        <div style={{ flex: 1 }} />

        {/* 右侧：显示发件账户邮箱 */}
        {fromAccount && (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--ink-3)' }}>
            {fromAccount.email}
          </span>
        )}

        {/* 丢弃（关闭浮窗） */}
        <button
          className="pill-btn"
          onClick={() => onOpenChange(false)}
          disabled={isBusy}
          type="button"
        >
          <Icon name="trash" size={12} />
        </button>
      </div>
    </div>
  )
}

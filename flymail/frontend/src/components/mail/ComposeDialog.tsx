import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { AddressInput } from '@/components/mail/AddressInput'
import { RichEditor } from '@/components/mail/composer/RichEditor'
import type { RichEditorHandle } from '@/components/mail/composer/RichEditor'
import {
  useSend,
  useCreateDraft,
  useUpdateDraft,
  useDeleteDraft,
  useAccounts,
  useAliasesOfAccounts,
  useSignature,
} from '@/lib/queries'
import { useToast } from '@/components/ui/Toast'
import { COMPOSE_CLOSE_EVENT } from '@/hooks/useKeyboardShortcuts'
import { useInlineImages } from '@/hooks/useInlineImages'
import { formatBytes } from '@/lib/format'
import { buildFromOptions, pickFromOption } from '@/lib/from-options'
import { prepareInlineForDraft, prepareInlineForSend } from '@/lib/inline-images'
import { signatureForScenario } from '@/lib/signature'
import type { ComposeScenario } from '@/lib/signature'

// 附件总大小上限（25 MiB），需与后端 maxAttachmentTotal 保持一致。
// 内联图也走同一份配额：对收件方而言它们同样是 MIME part，服务器不区分。
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
  /** 撰写场景，决定插哪一份签名（新建 / 回复）。缺省按新建处理。 */
  scenario?: ComposeScenario
  /** 草稿里存的发件别名；空串或缺省表示用账户主地址 */
  fromAlias?: string
}

export interface ComposeDialogProps {
  open: boolean
  onOpenChange: (o: boolean) => void
  accountId: number | null
  initial?: ComposeInitial
  draftId?: number | null // 非空 = 编辑已有草稿
}

interface FormState {
  /**
   * 用户手动选中的发件项 key；null 表示"没手动选过"，此时按账户默认别名推导。
   * 存"覆盖值"而不是"当前值"，是为了别名列表异步到达时不需要在 effect 里补一次 setState。
   */
  fromOverride: string | null
  toStr: string
  ccStr: string
  bccStr: string
  subject: string
}

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

function emptyForm(): FormState {
  return { fromOverride: null, toStr: '', ccStr: '', bccStr: '', subject: '' }
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
  const [form, setForm] = React.useState<FormState>(emptyForm)

  // 一次撰写会话的标识：变化时 RichEditor 重灌正文。
  const [session, setSession] = React.useState(0)

  // ── 正文编辑器 ───────────────────────────────────────────────────────────────
  // 正文不进 React state：每敲一个键就把整篇（可能带十封引用的）文档序列化一遍，
  // 在长回复里是实打实的卡顿。真正需要正文的只有发送和存草稿两处，届时按需取。
  const editorRef = React.useRef<RichEditorHandle>(null)
  const inline = useInlineImages()

  // ── 附件 ─────────────────────────────────────────────────────────────────────
  const [attachments, setAttachments] = React.useState<File[]>([])
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const attachTotal = attachments.reduce((sum, f) => sum + f.size, 0)

  function onPickFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const picked = Array.from(e.target.files ?? [])
    e.target.value = '' // 允许再次选择同一文件
    if (picked.length === 0) return
    const next = [...attachments, ...picked]
    const total = next.reduce((sum, f) => sum + f.size, 0) + inline.totalBytes()
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
  // 关闭确认：写了一半的邮件是这个应用里唯一真正会丢失的东西——
  // 删邮件有撤销、服务器上还有副本，而没存的草稿一旦关掉就什么都不剩。
  // 别处的确认框都被撤销取代了，这里反而必须留一道。
  const [closeGuard, setCloseGuard] = React.useState(false)

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

  // ── 发件人别名 ───────────────────────────────────────────────────────────────
  // 只在浮窗打开时拉：没在写信的时候，为每个账户都发一次别名请求没有意义。
  const aliasAccountIds = React.useMemo(
    () => (open ? accounts.map((a) => a.id) : []),
    [open, accounts],
  )
  const { byAccount: aliasesByAccount } = useAliasesOfAccounts(aliasAccountIds)

  const fromOptions = React.useMemo(
    () => buildFromOptions(accounts, aliasesByAccount),
    [accounts, aliasesByAccount],
  )

  /**
   * 当前发件项。
   *
   * 手动选过就用手选的；没选过则由 pickFromOption 推导（草稿里的别名 → 账户默认别名 →
   * 主地址）。做成派生值而不是 state，别名列表异步到达时才不需要再补一次 setState。
   */
  const fromOption = React.useMemo(() => {
    if (form.fromOverride) {
      const hit = fromOptions.find((o) => o.key === form.fromOverride)
      if (hit) return hit
    }
    return pickFromOption(fromOptions, accountId, initial?.fromAlias, aliasesByAccount)
  }, [form.fromOverride, fromOptions, accountId, initial?.fromAlias, aliasesByAccount])

  const effectiveAccountId = fromOption?.accountId ?? accountId
  const noAccount = effectiveAccountId == null

  // ── 签名 ─────────────────────────────────────────────────────────────────────
  const scenario: ComposeScenario = initial?.scenario ?? (initial?.inReplyTo ? 'reply' : 'new')
  const sigQuery = useSignature(effectiveAccountId)
  const signatureHtml = signatureForScenario(sigQuery.data, scenario)
  // 等结果落定再动文档：placeholder 阶段的空签名一插，真签名到货时就成了"重复插入"
  const signatureReady = effectiveAccountId == null || sigQuery.isSuccess || sigQuery.isError

  // 打开时用 initial 预填，关闭时释放内联图。
  // ⚠ 所有初始化 setState 必须留在这**一个** effect 里：拆成多个不会更清楚，
  //   只会让"打开一次浮窗"变成好几轮渲染，而且更难保证它们的先后顺序。
  React.useEffect(() => {
    if (!open) return

    setValidationError(null)
    setInfoMessage(null)
    setMinimized(false)
    setPos(null) // 每次打开回到默认右下角
    setAttachments([]) // 附件不随草稿持久化，每次打开清空
    setShowCc((initial?.cc ?? []).length > 0) // 有抄送预填时自动展开抄送行
    setShowBcc(false)
    setForm({
      fromOverride: null,
      toStr: (initial?.to ?? []).join(', '),
      ccStr: (initial?.cc ?? []).join(', '),
      bccStr: '',
      subject: initial?.subject ?? '',
    })
    setSession((s) => s + 1)

    // 关闭（或换一封信）时释放上一会话的 blob URL——早了图裂，不放就是内存泄漏
    return () => { inline.reset() }
  }, [open, initial, draftId, accountId, inline])

  /**
   * 签名的插入与替换。
   *
   * 三条规则：
   *   1. 打开草稿时不插——草稿正文里已经带着上次存的签名，再插一次就是两份；
   *   2. 新会话插一次；
   *   3. 会话中途换发件人时整块替换（RichEditor.applySignature 走 ProseMirror 事务，
   *      只动 signature 节点，用户写的正文一个字不碰）。
   * 这里只调编辑器的命令、不碰 React state，所以不会多出一次渲染。
   */
  const sigAppliedRef = React.useRef<{ session: number; account: number | null } | null>(null)
  React.useEffect(() => {
    if (!open || !signatureReady) return
    const prev = sigAppliedRef.current
    const isNewSession = prev === null || prev.session !== session
    if (isNewSession) {
      sigAppliedRef.current = { session, account: effectiveAccountId }
      if (draftId == null && signatureHtml) editorRef.current?.applySignature(signatureHtml)
      return
    }
    if (prev.account !== effectiveAccountId) {
      sigAppliedRef.current = { session, account: effectiveAccountId }
      editorRef.current?.applySignature(signatureHtml)
    }
  }, [open, session, signatureReady, signatureHtml, effectiveAccountId, draftId])

  // ── Mutations ────────────────────────────────────────────────────────────────
  const sendMutation = useSend()
  const createDraft = useCreateDraft()
  const updateDraft = useUpdateDraft()
  const deleteDraft = useDeleteDraft()

  // ── Derived ─────────────────────────────────────────────────────────────────
  const isSending = sendMutation.isPending
  const isSavingDraft = createDraft.isPending || updateDraft.isPending
  const isBusy = isSending || isSavingDraft

  // 单账户单地址时只展示不可选
  const fromAccountIndex = Math.max(0, accounts.findIndex((a) => a.id === effectiveAccountId))
  const multiFrom = fromOptions.length > 1

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

  // 指向最新的发送逻辑（含最新的表单与忙碌状态），供下面的全局组合键监听调用。
  const sendRef = React.useRef<() => void>(() => {})
  const closeRef = React.useRef<() => void>(() => {})

  // ⌘/Ctrl + Enter 发送。
  //
  // 绑在 window 而不是对话框上：焦点多半在富文本编辑器的 contenteditable 里，
  // 而全局单键快捷键在 Compose 打开时是整体屏蔽的（见 useKeyboardShortcuts），
  // 组合键不受那条屏蔽影响，正好留给这里。
  React.useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault()
        // 读 ref 而不是闭包里的 handleSend：它每渲染都是新函数，
        // 进依赖数组会让监听器随每次按键反复摘挂。
        sendRef.current()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  // Esc 关闭撰写器：全局快捷键广播请求，由这里决定是直接关还是先问一句。
  // 同 sendRef，走 ref 避免监听器随每次渲染摘挂。
  React.useEffect(() => {
    if (!open) return
    function onRequest() { closeRef.current() }
    window.addEventListener(COMPOSE_CLOSE_EVENT, onRequest)
    return () => window.removeEventListener(COMPOSE_CLOSE_EVENT, onRequest)
  }, [open])

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

    // 正文里的 blob:/data: 图收敛成 cid: 引用，同时得到与之严格同序的文件列表
    const raw = editorRef.current?.getHTML() ?? ''
    const prepared = prepareInlineForSend(raw, (src) => inline.lookup(src))

    const inlineBytes = prepared.files.reduce((sum, f) => sum + f.size, 0)
    if (attachTotal + inlineBytes > MAX_ATTACH_TOTAL) {
      setValidationError(t('compose.attachTooLarge', { size: formatBytes(MAX_ATTACH_TOTAL) }))
      return
    }

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
          body_html: prepared.html,
          in_reply_to: initial?.inReplyTo,
          references: initial?.references,
          // 主地址不带该字段，保持与 M13 之前完全一致的请求体
          from_alias: fromOption?.alias || undefined,
          inline_cids: prepared.cids.length > 0 ? prepared.cids : undefined,
        },
        files: attachments,
        inline: prepared.files,
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

  // 忙碌或没有可用账户时按下组合键不应重复提交——按钮那条路径由 disabled 挡住，
  // 键盘这条得自己挡。
  // 写在 effect 里：渲染期间赋值 ref 会破坏渲染的纯粹性。
  React.useEffect(() => {
    sendRef.current = () => {
      if (isBusy || noAccount) return
      handleSend()
    }
    closeRef.current = requestClose
  })

  /** 正文是否有实际内容（签名不算——它是自动插入的，不是用户写的）。 */
  function hasBodyText(): boolean {
    const html = editorRef.current?.getHTML() ?? ''
    const withoutSignature = html.replace(/<div[^>]*data-signature[\s\S]*?<\/div>/gi, '')
    return withoutSignature.replace(/<[^>]*>/g, '').replace(/&nbsp;/gi, ' ').trim().length > 0
  }

  /** 有没有值得挽救的东西 */
  function isDirty(): boolean {
    return (
      form.toStr.trim().length > 0 ||
      form.ccStr.trim().length > 0 ||
      form.bccStr.trim().length > 0 ||
      form.subject.trim().length > 0 ||
      attachments.length > 0 ||
      hasBodyText()
    )
  }

  /**
   * 请求关闭撰写器。空白撰写器直接关，写过东西的先问一句。
   * 所有关闭入口（× 按钮、丢弃按钮、Esc）都必须走这里。
   */
  function requestClose() {
    if (isBusy) return
    if (isDirty()) setCloseGuard(true)
    else onOpenChange(false)
  }

  // ── 存草稿 ───────────────────────────────────────────────────────────────────
  async function handleSaveDraft() {
    setValidationError(null)
    setInfoMessage(null)
    if (noAccount) return

    // 草稿要自包含：blob URL 换页面就失效，内联图必须内嵌成 data: URI 存进正文
    const raw = editorRef.current?.getHTML() ?? ''
    const { html, truncated } = await prepareInlineForDraft(raw, (src) => inline.toDataUri(src))
    if (truncated) toast(t('compose.draftTooLarge'))

    const req = {
      account_id: effectiveAccountId as number,
      to: parseAddrs(form.toStr),
      cc: parseAddrs(form.ccStr),
      bcc: parseAddrs(form.bccStr),
      subject: form.subject,
      body_html: html,
      in_reply_to: initial?.inReplyTo ?? '',
      references: initial?.references ?? '',
      from_alias: fromOption?.alias ?? '',
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
          onClick={(e) => { e.stopPropagation(); requestClose() }}
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

      {/* 关闭确认。三选一而不是二选一：直接问「确定丢弃吗」会逼用户在
          「丢掉」和「继续写」之间选，而他真正想要的多半是第三个——先存着。 */}
      {closeGuard && (
        <div className="compose-guard" role="dialog" aria-modal="true" aria-label={t('compose.closeGuardTitle')}>
          <div className="compose-guard-card">
            <div className="compose-guard-title">{t('compose.closeGuardTitle')}</div>
            <div className="compose-guard-text">{t('compose.closeGuardText')}</div>
            <div className="compose-guard-actions">
              <button type="button" className="pill-btn" onClick={() => setCloseGuard(false)}>
                {t('compose.closeGuardCancel')}
              </button>
              <button
                type="button"
                className="pill-btn danger"
                onClick={() => { setCloseGuard(false); onOpenChange(false) }}
              >
                {t('compose.closeGuardDiscard')}
              </button>
              <button
                type="button"
                className="pill-btn primary"
                disabled={noAccount || isBusy}
                onClick={() => { setCloseGuard(false); void handleSaveDraft() }}
              >
                {t('compose.closeGuardSave')}
              </button>
            </div>
          </div>
        </div>
      )}

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
          onClick={requestClose}
        >
          <Icon name="close" size={14} />
        </button>
      </div>

      {/* ── 表单主体 .compose-body ───────────────────────────────────────────── */}
      <div className="compose-body">

        {/* From 行：账户 × 别名的扁平列表 */}
        <div className="compose-row">
          <label>{t('compose.from')}</label>
          {multiFrom ? (
            <select
              value={fromOption?.key ?? ''}
              onChange={(e) => set('fromOverride', e.target.value)}
              disabled={isBusy}
              style={{
                border: 0, outline: 0,
                background: 'var(--bg-alt)',
                padding: '4px 8px',
                borderRadius: 6,
                fontSize: 13,
                color: 'var(--ink)',
                fontFamily: 'var(--font-body)',
                maxWidth: '100%',
              }}
            >
              {fromOptions.map((o) => (
                <option key={o.key} value={o.key}>{o.label}</option>
              ))}
            </select>
          ) : (
            // 单账户单地址：只读展示
            <span className="from-account">
              <span
                className="acct-dot"
                style={{ background: acctDotColor(fromAccountIndex) }}
              />
              {fromOption?.email ?? ''}
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

        {/* 正文：Tiptap 富文本（工具栏 + 引用折叠 + 内联图） */}
        <RichEditor
          ref={editorRef}
          initialHtml={initial?.bodyHtml ?? ''}
          resetKey={String(session)}
          editable={!isBusy}
          registerInlineImage={(file) => inline.register(file)}
        />

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
          onClick={() => { void handleSaveDraft() }}
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

        {/* 右侧：显示发件地址 */}
        {fromOption && (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--ink-3)' }}>
            {fromOption.email}
          </span>
        )}

        {/* 丢弃（关闭浮窗） */}
        <button
          className="pill-btn"
          onClick={requestClose}
          disabled={isBusy}
          type="button"
        >
          <Icon name="trash" size={12} />
        </button>
      </div>
    </div>
  )
}

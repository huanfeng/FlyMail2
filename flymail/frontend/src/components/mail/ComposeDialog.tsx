import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import Editor from 'react-simple-wysiwyg'
import type { ContentEditableEvent } from 'react-simple-wysiwyg'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSend, useCreateDraft, useUpdateDraft, useDeleteDraft } from '@/lib/queries'

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
  toStr: string
  ccStr: string
  bccStr: string
  subject: string
  bodyHtml: string
}

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

function emptyForm(): FormState {
  return { toStr: '', ccStr: '', bccStr: '', subject: '', bodyHtml: '' }
}

/** 按逗号或分号拆分地址，去空格和空项 */
function parseAddrs(s: string): string[] {
  return s
    .split(/[,;]/)
    .map((a) => a.trim())
    .filter(Boolean)
}

// ────────────────────────────────────────────────────────────────────────────────
// Sub-components
// ────────────────────────────────────────────────────────────────────────────────

interface FieldProps {
  label: string
  children: React.ReactNode
}

function Field({ label, children }: FieldProps) {
  return (
    <div className="flex flex-col gap-1">
      <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>{label}</Label>
      {children}
    </div>
  )
}

// ────────────────────────────────────────────────────────────────────────────────
// ComposeDialog
// ────────────────────────────────────────────────────────────────────────────────

export function ComposeDialog({
  open,
  onOpenChange,
  accountId,
  initial,
  draftId,
}: ComposeDialogProps) {
  const { t } = useTranslation()

  // ── Form state ──────────────────────────────────────────────────────────────
  const [form, setForm] = React.useState<FormState>(emptyForm)
  const [validationError, setValidationError] = React.useState<string | null>(null)
  const [infoMessage, setInfoMessage] = React.useState<string | null>(null)

  // 便捷 setter
  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  // 打开时用 initial 预填，关闭时清空
  React.useEffect(() => {
    if (open) {
      setValidationError(null)
      setInfoMessage(null)
      if (initial) {
        setForm({
          toStr: (initial.to ?? []).join(', '),
          ccStr: (initial.cc ?? []).join(', '),
          bccStr: '',
          subject: initial.subject ?? '',
          bodyHtml: initial.bodyHtml ?? '',
        })
      } else {
        setForm(emptyForm())
      }
    }
  }, [open, initial, draftId])

  // ── Mutations ────────────────────────────────────────────────────────────────
  const sendMutation = useSend()
  const createDraft = useCreateDraft()
  const updateDraft = useUpdateDraft()
  const deleteDraft = useDeleteDraft()

  // ── Derived ─────────────────────────────────────────────────────────────────
  const isSending = sendMutation.isPending
  const isSavingDraft = createDraft.isPending || updateDraft.isPending
  const isBusy = isSending || isSavingDraft
  const noAccount = accountId === null

  // ── 标题：回复/转发/写邮件 ─────────────────────────────────────────────────
  function resolveTitle(): string {
    if (!initial) return t('compose.title')
    if (initial.inReplyTo) return t('compose.reply')
    if (initial.subject?.startsWith('Fwd:') || initial.subject?.startsWith('转发：')) {
      return t('compose.forward')
    }
    return t('compose.title')
  }

  // ── 发送 ────────────────────────────────────────────────────────────────────
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
        account_id: accountId as number,
        to: toAddrs,
        cc: ccAddrs.length > 0 ? ccAddrs : undefined,
        bcc: bccAddrs.length > 0 ? bccAddrs : undefined,
        subject: form.subject,
        body_html: form.bodyHtml,
        in_reply_to: initial?.inReplyTo,
        references: initial?.references,
      },
      {
        onSuccess: () => {
          // 发送成功后，若正在编辑草稿则将其删除
          if (draftId != null && accountId != null) {
            deleteDraft.mutate(
              { id: draftId, accountId: accountId as number },
              { onSettled: () => onOpenChange(false) },
            )
          } else {
            onOpenChange(false)
          }
        },
      },
    )
  }

  // ── 存草稿 ──────────────────────────────────────────────────────────────────
  function handleSaveDraft() {
    setValidationError(null)
    setInfoMessage(null)
    if (noAccount) return

    const req = {
      account_id: accountId as number,
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
            setInfoMessage(t('compose.draftSaved'))
            onOpenChange(false)
          },
        },
      )
    } else {
      createDraft.mutate(req, {
        onSuccess: () => {
          setInfoMessage(t('compose.draftSaved'))
          onOpenChange(false)
        },
      })
    }
  }

  // ── onChange for Editor ──────────────────────────────────────────────────────
  function handleBodyChange(e: ContentEditableEvent) {
    set('bodyHtml', e.target.value)
  }

  // ────────────────────────────────────────────────────────────────────────────
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        {/* 遮罩 */}
        <Dialog.Overlay
          className="fixed inset-0 z-40"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />

        {/* 对话框 */}
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 w-[560px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-y-auto rounded-xl shadow-xl flex flex-col gap-0 outline-none"
          style={{ background: 'var(--surface)', color: 'var(--ink)' }}
          aria-describedby={undefined}
        >
          {/* 标题栏 */}
          <div
            className="flex items-center justify-between px-6 py-4 shrink-0"
            style={{ borderBottom: '1px solid var(--rule)' }}
          >
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {resolveTitle()}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('compose.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          {/* 表单主体 */}
          <div className="flex flex-col gap-3 px-6 py-5">
            {/* 收件人 */}
            <Field label={t('compose.to')}>
              <Input
                value={form.toStr}
                onChange={(e) => set('toStr', e.target.value)}
                placeholder={t('compose.addrHint')}
                disabled={isBusy}
              />
            </Field>

            {/* 抄送 */}
            <Field label={t('compose.cc')}>
              <Input
                value={form.ccStr}
                onChange={(e) => set('ccStr', e.target.value)}
                placeholder={t('compose.addrHint')}
                disabled={isBusy}
              />
            </Field>

            {/* 密送 */}
            <Field label={t('compose.bcc')}>
              <Input
                value={form.bccStr}
                onChange={(e) => set('bccStr', e.target.value)}
                placeholder={t('compose.addrHint')}
                disabled={isBusy}
              />
            </Field>

            {/* 主题 */}
            <Field label={t('compose.subject')}>
              <Input
                value={form.subject}
                onChange={(e) => set('subject', e.target.value)}
                placeholder={t('compose.subject')}
                disabled={isBusy}
              />
            </Field>

            {/* 正文富文本编辑器 */}
            <Field label={t('compose.body')}>
              <div
                style={{
                  border: '1px solid var(--rule)',
                  borderRadius: '6px',
                  overflow: 'hidden',
                  minHeight: '180px',
                  background: 'var(--bg)',
                }}
              >
                <Editor
                  value={form.bodyHtml}
                  onChange={handleBodyChange}
                  disabled={isBusy}
                  containerProps={{
                    style: { minHeight: '180px' },
                  }}
                />
              </div>
            </Field>

            {/* 校验错误 */}
            {validationError && (
              <div
                className="rounded-md px-3 py-2 text-sm"
                style={{
                  background: 'oklch(0.577 0.245 27.325 / 0.1)',
                  color: 'var(--destructive)',
                }}
              >
                {validationError}
              </div>
            )}

            {/* 成功提示 */}
            {infoMessage && (
              <div
                className="rounded-md px-3 py-2 text-sm"
                style={{ background: 'var(--accent-wash, oklch(0.9 0.1 145 / 0.15))', color: 'var(--ink-2)' }}
              >
                {infoMessage}
              </div>
            )}
          </div>

          {/* 底部操作栏 */}
          <div
            className="flex items-center justify-between gap-3 px-6 py-4 shrink-0"
            style={{ borderTop: '1px solid var(--rule)' }}
          >
            {/* 存草稿 */}
            <Button
              variant="outline"
              size="sm"
              onClick={handleSaveDraft}
              disabled={isBusy || noAccount}
            >
              {isSavingDraft ? t('compose.sending') : t('compose.saveDraft')}
            </Button>

            {/* 右侧：取消 + 发送 */}
            <div className="flex items-center gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost" size="sm" disabled={isBusy}>
                  {t('compose.cancel')}
                </Button>
              </Dialog.Close>
              <Button
                size="sm"
                onClick={handleSend}
                disabled={isBusy || noAccount}
              >
                {isSending ? t('compose.sending') : t('compose.send')}
              </Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

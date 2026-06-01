import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { presetForEmail } from '@/lib/providers'
import { useCreateAccount, useUpdateAccount, useTestConnection } from '@/lib/queries'
import type { Account, AccountInput, ConnectionTestResult } from '@/lib/types'

// ────────────────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────────────────

export interface AccountDialogProps {
  open: boolean
  account: Account | null // null = 添加模式，非空 = 编辑模式
  onOpenChange: (open: boolean) => void
}

type SecurityOption = 'ssl' | 'starttls' | 'none'

interface FormState {
  name: string
  email: string
  username: string
  password: string
  imapHost: string
  imapPort: number
  imapSecurity: SecurityOption
  smtpHost: string
  smtpPort: number
  smtpSecurity: SecurityOption
}

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

function defaultForm(): FormState {
  return {
    name: '',
    email: '',
    username: '',
    password: '',
    imapHost: '',
    imapPort: 993,
    imapSecurity: 'ssl',
    smtpHost: '',
    smtpPort: 465,
    smtpSecurity: 'ssl',
  }
}

function formFromAccount(account: Account): FormState {
  return {
    name: account.name,
    email: account.email,
    username: account.username ?? '',
    password: '', // 编辑时密码留空，占位提示"留空则不修改"
    imapHost: account.imap_host,
    imapPort: account.imap_port,
    imapSecurity: account.imap_security as SecurityOption,
    smtpHost: account.smtp_host,
    smtpPort: account.smtp_port,
    smtpSecurity: account.smtp_security as SecurityOption,
  }
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
    <div className="flex flex-col gap-1.5">
      <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>{label}</Label>
      {children}
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps) {
  return (
    <div className="flex flex-col gap-3">
      <div
        className="text-xs font-semibold uppercase tracking-wide"
        style={{ color: 'var(--ink-3)', borderBottom: '1px solid var(--rule)', paddingBottom: '4px' }}
      >
        {title}
      </div>
      {children}
    </div>
  )
}

interface SecuritySelectProps {
  value: SecurityOption
  onChange: (v: SecurityOption) => void
  id?: string
}

function SecuritySelect({ value, onChange, id }: SecuritySelectProps) {
  return (
    <select
      id={id}
      value={value}
      onChange={(e) => onChange(e.target.value as SecurityOption)}
      className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
      style={{ color: 'var(--ink)' }}
    >
      <option value="ssl">SSL / TLS</option>
      <option value="starttls">STARTTLS</option>
      <option value="none">None</option>
    </select>
  )
}

// ────────────────────────────────────────────────────────────────────────────────
// TestResultPanel
// ────────────────────────────────────────────────────────────────────────────────

interface TestResultPanelProps {
  result: ConnectionTestResult | null
  isPending: boolean
  t: (key: string) => string
}

function TestResultPanel({ result, isPending, t }: TestResultPanelProps) {
  if (isPending) {
    return (
      <div
        className="rounded-md px-3 py-2 text-sm"
        style={{ background: 'var(--accent-wash)', color: 'var(--accent-ink)' }}
      >
        {t('account.testing')}
      </div>
    )
  }
  if (!result) return null

  return (
    <div
      className="rounded-md px-3 py-2 text-sm flex flex-col gap-1"
      style={{ background: 'var(--bg-alt)', color: 'var(--ink-2)' }}
    >
      <div className="flex items-start gap-2">
        <span>{result.imap ? '✓' : '✗'}</span>
        <span>
          {result.imap ? t('account.imapOk') : t('account.imapFail')}
          {!result.imap && result.imap_error ? ` — ${result.imap_error}` : ''}
        </span>
      </div>
      <div className="flex items-start gap-2">
        <span>{result.smtp ? '✓' : '✗'}</span>
        <span>
          {result.smtp ? t('account.smtpOk') : t('account.smtpFail')}
          {!result.smtp && result.smtp_error ? ` — ${result.smtp_error}` : ''}
        </span>
      </div>
    </div>
  )
}

// ────────────────────────────────────────────────────────────────────────────────
// AccountDialog
// ────────────────────────────────────────────────────────────────────────────────

export function AccountDialog({ open, account, onOpenChange }: AccountDialogProps) {
  const { t } = useTranslation()
  const isEdit = account !== null

  // ── Form state ──────────────────────────────────────────────────────────────
  const [form, setForm] = React.useState<FormState>(defaultForm)

  // 打开时根据模式初始化表单
  React.useEffect(() => {
    if (open) {
      setForm(account ? formFromAccount(account) : defaultForm())
    }
  }, [account, open])

  // 便捷 setter
  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  // ── Mutations ────────────────────────────────────────────────────────────────
  const createAccount = useCreateAccount()
  const updateAccount = useUpdateAccount()
  const testConnection = useTestConnection()

  // ── Build input ──────────────────────────────────────────────────────────────
  function buildInput(): AccountInput {
    const input: AccountInput = {
      name: form.name.trim(),
      email: form.email.trim(),
      imap_host: form.imapHost.trim(),
      imap_port: Number(form.imapPort) || 0,
      imap_security: form.imapSecurity,
      smtp_host: form.smtpHost.trim(),
      smtp_port: Number(form.smtpPort) || 0,
      smtp_security: form.smtpSecurity,
    }
    if (form.username.trim()) {
      input.username = form.username.trim()
    }
    // 编辑模式下密码为空时不传（后端 omitempty 保持原值）
    if (form.password) {
      input.password = form.password
    }
    return input
  }

  // ── Email blur：自动预设 ────────────────────────────────────────────────────
  function handleEmailBlur() {
    const preset = presetForEmail(form.email)
    if (!preset) return
    setForm((prev) => ({
      ...prev,
      imapHost: preset.imap.host,
      imapPort: preset.imap.port,
      imapSecurity: preset.imap.security,
      smtpHost: preset.smtp.host,
      smtpPort: preset.smtp.port,
      smtpSecurity: preset.smtp.security,
    }))
  }

  // ── 测试连接 ─────────────────────────────────────────────────────────────────
  function handleTest() {
    testConnection.mutate(buildInput())
  }

  // ── 保存 ────────────────────────────────────────────────────────────────────
  const [validationError, setValidationError] = React.useState<string | null>(null)

  function handleSave() {
    setValidationError(null)

    if (!form.name.trim() || !form.email.trim()) {
      setValidationError(t('account.nameRequired'))
      return
    }
    if (!isEdit && !form.password) {
      setValidationError(t('account.passwordRequired'))
      return
    }

    const input = buildInput()

    if (isEdit && account) {
      updateAccount.mutate(
        { id: account.id, input },
        { onSuccess: () => onOpenChange(false) },
      )
    } else {
      createAccount.mutate(input, {
        onSuccess: () => onOpenChange(false),
      })
    }
  }

  // ── Derived state ────────────────────────────────────────────────────────────
  const isSaving = createAccount.isPending || updateAccount.isPending
  const testResult = testConnection.data ?? null
  const testPending = testConnection.isPending

  // ────────────────────────────────────────────────────────────────────────────
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        {/* 遮罩 */}
        <Dialog.Overlay
          className="fixed inset-0 z-40"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />

        {/* 对话框内容 */}
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 w-[520px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-y-auto rounded-xl shadow-xl flex flex-col gap-0 outline-none"
          style={{ background: 'var(--surface)', color: 'var(--ink)' }}
          aria-describedby={undefined}
        >
          {/* 标题栏 */}
          <div
            className="flex items-center justify-between px-6 py-4"
            style={{ borderBottom: '1px solid var(--rule)' }}
          >
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {isEdit ? t('account.edit') : t('account.add')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('account.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          {/* 表单主体 */}
          <div className="flex flex-col gap-6 px-6 py-5">

            {/* ── 基本信息 ── */}
            <Section title={t('account.name') + ' / ' + t('account.email')}>
              <Field label={t('account.name')}>
                <Input
                  value={form.name}
                  onChange={(e) => set('name', e.target.value)}
                  placeholder={t('account.name')}
                />
              </Field>
              <Field label={t('account.email')}>
                <Input
                  type="email"
                  value={form.email}
                  onChange={(e) => set('email', e.target.value)}
                  onBlur={handleEmailBlur}
                  placeholder="user@example.com"
                />
              </Field>
              <Field label={t('account.username')}>
                <Input
                  value={form.username}
                  onChange={(e) => set('username', e.target.value)}
                  placeholder={t('account.username')}
                />
              </Field>
              <Field label={t('account.password')}>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => set('password', e.target.value)}
                  placeholder={isEdit ? t('account.passwordKeep') : t('account.password')}
                />
              </Field>
            </Section>

            {/* ── IMAP ── */}
            <Section title={t('account.imapSection')}>
              <Field label={t('account.host')}>
                <Input
                  value={form.imapHost}
                  onChange={(e) => set('imapHost', e.target.value)}
                  placeholder="imap.example.com"
                />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label={t('account.port')}>
                  <Input
                    type="number"
                    value={form.imapPort}
                    onChange={(e) => set('imapPort', Number(e.target.value))}
                    placeholder="993"
                  />
                </Field>
                <Field label={t('account.security')}>
                  <SecuritySelect
                    value={form.imapSecurity}
                    onChange={(v) => set('imapSecurity', v)}
                  />
                </Field>
              </div>
            </Section>

            {/* ── SMTP ── */}
            <Section title={t('account.smtpSection')}>
              <Field label={t('account.host')}>
                <Input
                  value={form.smtpHost}
                  onChange={(e) => set('smtpHost', e.target.value)}
                  placeholder="smtp.example.com"
                />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label={t('account.port')}>
                  <Input
                    type="number"
                    value={form.smtpPort}
                    onChange={(e) => set('smtpPort', Number(e.target.value))}
                    placeholder="465"
                  />
                </Field>
                <Field label={t('account.security')}>
                  <SecuritySelect
                    value={form.smtpSecurity}
                    onChange={(v) => set('smtpSecurity', v)}
                  />
                </Field>
              </div>
            </Section>

            {/* ── 测试连接结果 ── */}
            <TestResultPanel result={testResult} isPending={testPending} t={t} />

            {/* ── 校验错误 ── */}
            {validationError && (
              <div
                className="rounded-md px-3 py-2 text-sm"
                style={{ background: 'oklch(0.577 0.245 27.325 / 0.1)', color: 'var(--destructive)' }}
              >
                {validationError}
              </div>
            )}
          </div>

          {/* 底部操作栏 */}
          <div
            className="flex items-center justify-between gap-3 px-6 py-4"
            style={{ borderTop: '1px solid var(--rule)' }}
          >
            {/* 左侧：测试连接 */}
            <Button
              variant="outline"
              size="sm"
              onClick={handleTest}
              disabled={testPending || isSaving}
            >
              {t('account.test')}
            </Button>

            {/* 右侧：取消 + 保存 */}
            <div className="flex items-center gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost" size="sm" disabled={isSaving}>
                  {t('account.cancel')}
                </Button>
              </Dialog.Close>
              <Button size="sm" onClick={handleSave} disabled={isSaving || testPending}>
                {t('account.save')}
              </Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

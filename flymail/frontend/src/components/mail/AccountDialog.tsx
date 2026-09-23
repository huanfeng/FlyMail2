import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Icon } from '@/components/ui/Icon'
import { isDirty, useDismissGuard } from '@/lib/dismiss-guard'
import { presetForEmail } from '@/lib/providers'
import { useCreateAccount, useUpdateAccount, useTestConnection, useOAuthProviders } from '@/lib/queries'
import { OAuthPanel } from '@/components/mail/OAuthPanel'
import { ACCOUNT_STATUS_NEEDS_REAUTH } from '@/lib/types'
import type { Account, AccountInput, OAuthProviderInfo } from '@/lib/types'

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
// AccountDialog
// ────────────────────────────────────────────────────────────────────────────────

export function AccountDialog({ open, account, onOpenChange }: AccountDialogProps) {
  const { t } = useTranslation()
  const isEdit = account !== null

  // ── Form state ──────────────────────────────────────────────────────────────
  const [form, setForm] = React.useState<FormState>(defaultForm)
  // 服务器设置默认折叠（常见邮箱由 preset 自动识别，无需手动配置），降低表单复杂度
  const [advancedOpen, setAdvancedOpen] = React.useState(false)
  // 邮箱失焦后是否命中了服务商预设（命中时折叠区外显示提示，让用户放心）
  const [autoFilled, setAutoFilled] = React.useState(false)
  // 非空时表单被 OAuth 授权面板取代（新建或重新授权）
  const [oauthWith, setOAuthWith] = React.useState<OAuthProviderInfo | null>(null)

  // OAuth 账户没有密码，服务器地址也由提供方预设决定，编辑时这些字段一律不展示。
  const isOAuthAccount = account?.auth_type === 'oauth'
  const needsReauth = account?.status === ACCOUNT_STATUS_NEEDS_REAUTH
  const { data: oauthProviders } = useOAuthProviders()
  /**
   * ⚠ 未配置凭据的提供方**不过滤掉**，而是置灰并说明去哪配。
   *
   * 这里原先是 `.filter((p) => p.configured)`，于是没配 client_id 时整个入口
   * 凭空消失——用户看到的是「这个功能没做」，而真相是「还差一步配置」。
   * 后端也是照这个契约写的（见 account.OAuthProviders 的头注释）。
   */
  const availableProviders = oauthProviders ?? []

  /*
   * 表单的初始化与**保留**。
   *
   * ── 原先的问题 ───────────────────────────────────────────────────────────
   *
   * 这里过去是 `if (open) setForm(...)`——**每次打开都重置**。而 radix Dialog
   * 默认点遮罩就关闭，于是「填到一半手滑点了对话框外面」= 全部白填。
   * 添加账户要填邮箱、密码、两组服务器地址端口，重填一遍代价相当高，
   * 而触发它只需要一次误点。
   *
   * ── 现在的判据 ───────────────────────────────────────────────────────────
   *
   * 只在**目标变了**的时候才初始化：换了另一个账户、或在"新建/编辑"之间切换。
   * 同一个目标重新打开时，上一次填的内容原样还在（组件常驻挂载，state 本来
   * 就活着，是这个 effect 一直在抹掉它）。
   *
   * 保存成功后要显式清掉草稿（见 handleSave），否则下次点"添加账户"会看到
   * 刚存过的那份数据。
   */
  const targetKey = account ? `edit:${account.id}` : 'new'
  const initializedFor = React.useRef<string | null>(null)
  const [restoredDraft, setRestoredDraft] = React.useState(false)

  React.useEffect(() => {
    if (!open) return
    if (initializedFor.current === targetKey) {
      // 同一个目标再次打开：保留用户填到一半的内容。这里只记"是重开"，
      // 提示条渲染时还要看 dirty——空表单重开没有草稿，不该弹提示。
      setRestoredDraft(true)
      return
    }
    initializedFor.current = targetKey
    setForm(account ? formFromAccount(account) : defaultForm())
    setAdvancedOpen(false)
    setAutoFilled(false)
    setOAuthWith(null)
    setRestoredDraft(false)
  }, [account, open, targetKey])

  /**
   * 有没有值得保护的未保存内容。
   *
   * 基线就是 resetForm 会恢复到的那个状态——两处共用同一个概念，
   * 免得出现「点清空回到 A，但 dirty 判定拿 B 当基线」这种自相矛盾。
   */
  const baseline = React.useMemo(
    () => (account ? formFromAccount(account) : defaultForm()),
    [account],
  )
  // OAuth 授权面板占据整个对话框时不拦：那时表单不可见，用户点外面
  // 想的是"退出这个授权流程"，拦住只会让人莫名其妙。
  const dirty = !oauthWith && isDirty(form, baseline)
  const { contentRef, dismissProps } = useDismissGuard(() => dirty)

  // 便捷 setter
  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  // ── Mutations ────────────────────────────────────────────────────────────────
  const createAccount = useCreateAccount()
  const updateAccount = useUpdateAccount()
  const testConnection = useTestConnection()
  // 声明放在这里而不是下面：resetForm 要用它。函数声明虽然会提升、
  // 实际调用也在渲染之后，但让读的人去推断"这时候它初始化了没有"是没必要的负担。
  const resetTest = testConnection.reset

  /** 清空重置：回到该目标的初始状态（新建=空表单，编辑=账户当前值）。 */
  function resetForm() {
    setForm(account ? formFromAccount(account) : defaultForm())
    setAdvancedOpen(false)
    setAutoFilled(false)
    setValidationError(null)
    setRestoredDraft(false)
    resetTest()
  }


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
  // 命中服务商预设 → 静默填充服务器配置并给出提示（保持折叠）；
  // 未命中且服务器还没填 → 展开高级区提示手动配置。名称为空时顺带用邮箱前缀补上。
  function handleEmailBlur() {
    const email = form.email.trim()
    const preset = presetForEmail(email)
    setForm((prev) => {
      const next = { ...prev }
      if (!prev.name.trim() && email.includes('@')) {
        next.name = email.split('@')[0]
      }
      if (preset) {
        next.imapHost = preset.imap.host
        next.imapPort = preset.imap.port
        next.imapSecurity = preset.imap.security
        next.smtpHost = preset.smtp.host
        next.smtpPort = preset.smtp.port
        next.smtpSecurity = preset.smtp.security
      }
      return next
    })
    if (preset) {
      setAutoFilled(true)
    } else if (email && !form.imapHost.trim()) {
      setAdvancedOpen(true)
    }
  }

  // ── 测试连接 ─────────────────────────────────────────────────────────────────
  // 测试用「当前表单里」的凭据连服务器，前置校验必填项，缺服务器时展开高级区。
  function handleTest() {
    setValidationError(null)
    if (!form.email.trim() || !form.password) {
      setValidationError(t('account.testNeedsInput'))
      return
    }
    if (!form.imapHost.trim() || !form.smtpHost.trim()) {
      setValidationError(t('account.serverRequired'))
      setAdvancedOpen(true)
      return
    }
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
    // 服务器地址缺失（preset 未命中且用户没手动填）→ 展开高级区引导补全
    if (!form.imapHost.trim() || !form.smtpHost.trim()) {
      setValidationError(t('account.serverRequired'))
      setAdvancedOpen(true)
      return
    }

    const input = buildInput()

    // 保存成功后丢弃草稿：留着的话，下次点"添加账户"会看到刚存过的那份内容。
    const done = () => {
      initializedFor.current = null
      setRestoredDraft(false)
      onOpenChange(false)
    }
    if (isEdit && account) {
      updateAccount.mutate({ id: account.id, input }, { onSuccess: done })
    } else {
      createAccount.mutate(input, { onSuccess: done })
    }
  }

  // 重新打开时清掉上一次的测试结果（避免旧状态残留在底栏）
  React.useEffect(() => {
    if (open) resetTest()
  }, [open, resetTest])

  // ── Derived state ────────────────────────────────────────────────────────────
  const isSaving = createAccount.isPending || updateAccount.isPending
  const testResult = testConnection.data ?? null
  const testPending = testConnection.isPending

  // 测试状态汇总为一行文本，固定显示在底栏（高度不变，对话框不会因测试跳动）。
  // 失败详情放 title 悬停提示。
  const testStatus = testPending
    ? t('account.testing')
    : testConnection.isError
      ? t('account.testError')
      : testResult
        ? `IMAP ${testResult.imap ? '✓' : '✗'} · SMTP ${testResult.smtp ? '✓' : '✗'}`
        : ''
  const testDetail = testResult
    ? [testResult.imap_error, testResult.smtp_error].filter(Boolean).join(' / ')
    : ''
  const testAllOk = testResult !== null && testResult.imap && testResult.smtp
  const testHasFail = testConnection.isError || (testResult !== null && (!testResult.imap || !testResult.smtp))

  // ────────────────────────────────────────────────────────────────────────────
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        {/* 遮罩 */}
        <Dialog.Overlay
          className="fixed inset-0 z-[70]"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />

        {/* 对话框内容：外层 overflow-hidden 保住四角圆角（滚动条如出现在内层，
            不会盖住右侧圆角），标题栏/底栏固定，仅中间表单区滚动 */}
        <Dialog.Content
          ref={contentRef}
          {...dismissProps}
          className="fixed left-1/2 top-1/2 z-[80] -translate-x-1/2 -translate-y-1/2 w-[520px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-hidden rounded-xl shadow-xl flex flex-col gap-0 outline-none"
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

          {/* 草稿恢复提示。
              内容跨"关闭再打开"保留之后，用户重新打开会看到一堆已填字段——
              不说一句的话，很容易被理解成"系统记住了我的账号"。这一行把
              「这是你上次填到一半的」讲明白，并给出就地丢弃的入口。
              ⚠ 要同时满足 dirty：只是打开看了一眼、一个字没填就关掉，
              再打开时没有任何"草稿"可言，弹这一行只会让人莫名其妙。 */}
          {restoredDraft && dirty && (
            <div className="acct-draft-note" role="status">
              <span>{t('account.draftRestored')}</span>
              <button type="button" className="acct-draft-discard" onClick={resetForm}>
                {t('account.reset')}
              </button>
            </div>
          )}

          {/* 表单主体（唯一滚动区） */}
          <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-6 py-5">

            {/* ── OAuth 授权面板：进行中时完全取代表单，避免用户同时面对两条互斥的建号路径 ── */}
            {oauthWith ? (
              <OAuthPanel
                provider={oauthWith}
                accountId={account?.id}
                hintEmail={form.email.trim() || account?.email}
                name={form.name.trim() || undefined}
                onDone={() => onOpenChange(false)}
                onCancel={() => setOAuthWith(null)}
              />
            ) : (
            <>

            {/* ── OAuth 入口（仅新建）──
                排在手填表单前面是因为两家的口令认证都在收缩：Outlook 个人账户
                已于 2024-09-16 彻底关闭基本认证（没有应用专用密码这条退路），
                Gmail 还留着应用专用密码但已放话要淘汰。
                所以「没配凭据」对两家的意义不同，退路提示要看 password_auth。 */}
            {!isEdit && availableProviders.length > 0 && (
              <div className="flex flex-col gap-2">
                {availableProviders.map((p) => (
                  <React.Fragment key={p.id}>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={!p.configured}
                      title={p.configured ? undefined : t('account.oauthNotConfiguredHint')}
                      onClick={() => setOAuthWith(p)}
                    >
                      {t('account.oauthSignIn', { provider: p.name })}
                    </Button>
                    {!p.configured && (
                      <span className="text-xs" style={{ color: 'var(--ink-3)' }}>
                        {t('account.oauthNotConfigured')}
                        {p.password_auth && ` ${t('account.oauthPasswordFallback')}`}
                      </span>
                    )}
                  </React.Fragment>
                ))}
                <div className="flex items-center gap-3 py-1">
                  <span className="h-px flex-1" style={{ background: 'var(--rule)' }} />
                  <span className="text-xs" style={{ color: 'var(--ink-3)' }}>
                    {t('account.oauthOr')}
                  </span>
                  <span className="h-px flex-1" style={{ background: 'var(--rule)' }} />
                </div>
              </div>
            )}

            {/* ── OAuth 账户状态条（仅编辑）：授权失效时给出可操作的恢复入口 ── */}
            {isEdit && isOAuthAccount && (
              <div
                className="flex flex-col gap-2 rounded-md px-3 py-3 text-sm"
                style={
                  needsReauth
                    ? { background: 'oklch(0.577 0.245 27.325 / 0.1)', color: 'var(--destructive)' }
                    : { background: 'var(--accent-wash)', color: 'var(--accent-ink)' }
                }
              >
                <span>
                  {needsReauth
                    ? t('account.oauthNeedsReauth')
                    : t('account.oauthManaged', { provider: account?.oauth_provider ?? '' })}
                </span>
                <div>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      const p = (oauthProviders ?? []).find((x) => x.id === account?.oauth_provider)
                      if (p) setOAuthWith(p)
                    }}
                    disabled={!(oauthProviders ?? []).some((x) => x.id === account?.oauth_provider && x.configured)}
                  >
                    {t('account.oauthReauthorize')}
                  </Button>
                </div>
              </div>
            )}

            {/* ── 基本信息（常显）：邮箱驱动自动识别，名称留空自动取邮箱前缀 ── */}
            <Field label={t('account.email')}>
              <Input
                type="email"
                value={form.email}
                onChange={(e) => set('email', e.target.value)}
                onBlur={handleEmailBlur}
                placeholder="user@example.com"
              />
            </Field>
            {!isOAuthAccount && (
              <Field label={t('account.password')}>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => set('password', e.target.value)}
                  placeholder={isEdit ? t('account.passwordKeep') : t('account.password')}
                />
              </Field>
            )}
            <Field label={t('account.name')}>
              <Input
                value={form.name}
                onChange={(e) => set('name', e.target.value)}
                placeholder={t('account.name')}
              />
            </Field>

            {/* preset 命中提示：告知服务器已自动配置，无需展开高级区 */}
            {autoFilled && !advancedOpen && (
              <div
                className="rounded-md px-3 py-2 text-sm"
                style={{ background: 'var(--accent-wash)', color: 'var(--accent-ink)' }}
              >
                {t('account.advancedAuto')} — {form.imapHost} · {form.smtpHost}
              </div>
            )}

            {/* ── 服务器设置（默认折叠的高级区）── */}
            <button
              type="button"
              className="flex items-center gap-2 text-sm outline-none"
              style={{ color: 'var(--ink-2)', padding: '2px 0' }}
              onClick={() => setAdvancedOpen((v) => !v)}
              aria-expanded={advancedOpen}
            >
              <span
                style={{
                  display: 'inline-flex',
                  transform: advancedOpen ? 'rotate(90deg)' : 'none',
                  transition: 'transform 0.15s',
                  color: 'var(--ink-3)',
                }}
              >
                <Icon name="chevron-right" size={14} />
              </span>
              <span className="font-medium">{t('account.advanced')}</span>
              {!advancedOpen && (
                <span style={{ color: 'var(--ink-3)', fontSize: '0.75rem' }}>
                  {form.imapHost.trim() || t('account.notConfigured')} · {form.smtpHost.trim() || t('account.notConfigured')}
                </span>
              )}
            </button>

            {advancedOpen && (
              <div className="flex flex-col gap-6 rounded-lg p-4" style={{ background: 'var(--bg-alt)' }}>
                <Field label={t('account.username')}>
                  <Input
                    value={form.username}
                    onChange={(e) => set('username', e.target.value)}
                    placeholder={t('account.username')}
                  />
                </Field>

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
              </div>
            )}

            {/* ── 校验错误 ── */}
            {validationError && (
              <div
                className="rounded-md px-3 py-2 text-sm"
                style={{ background: 'oklch(0.577 0.245 27.325 / 0.1)', color: 'var(--destructive)' }}
              >
                {validationError}
              </div>
            )}
            </>
            )}
          </div>

          {/* 底部操作栏：测试状态内联显示在固定高度的底栏里，对话框尺寸不随测试变化。
              授权面板自带取消按钮，且此时既无密码可测也无内容可存，故整条隐藏。 */}
          {!oauthWith && (
          <div
            className="flex items-center gap-3 px-6 py-4"
            style={{ borderTop: '1px solid var(--rule)' }}
          >
            {/* 左侧：测试连接 + 单行状态（截断，详情悬停）。
                OAuth 账户没有密码可供测试，凭据有效性由授权流程本身保证，故不展示。 */}
            {!isOAuthAccount && (
              <Button
                variant="outline"
                size="sm"
                onClick={handleTest}
                disabled={testPending || isSaving}
              >
                {t('account.test')}
              </Button>
            )}
            <span
              className="flex-1 min-w-0 truncate text-sm"
              title={testDetail || undefined}
              style={{
                color: testHasFail
                  ? 'var(--destructive)'
                  : testAllOk
                    ? '#2c7a4f'
                    : 'var(--ink-3)',
              }}
            >
              {testStatus}
            </span>

            {/* 右侧：清空 + 取消 + 保存 */}
            <div className="flex items-center gap-2 shrink-0">
              {/* 表单内容现在会跨"关闭再打开"保留（见 initializedFor 处的说明），
                  所以必须给一个显式的丢弃入口——否则用户没有办法回到空表单。 */}
              <Button
                variant="ghost"
                size="sm"
                onClick={resetForm}
                disabled={isSaving || testPending}
                title={t('account.resetHint')}
              >
                {t('account.reset')}
              </Button>
              <Dialog.Close asChild>
                <Button variant="outline" size="sm" disabled={isSaving}>
                  {t('account.cancel')}
                </Button>
              </Dialog.Close>
              <Button size="sm" onClick={handleSave} disabled={isSaving || testPending}>
                {t('account.save')}
              </Button>
            </div>
          </div>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

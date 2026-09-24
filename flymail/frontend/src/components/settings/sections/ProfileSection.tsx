// 设置 → 个人资料与安全：展示名、联系邮箱，以及修改管理员密码。
//
// 「安全」页此前只有改密码一项，并进来：两者都是「这个管理员账户本身」的设置。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/components/ui/Toast'
import { useChangePassword, useMe, useUpdateProfile } from '@/lib/queries'
import { Row } from '../controls'

export function ProfileSection() {
  return (
    <>
      <ProfileBlock />
      <PasswordBlock />
    </>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：资料分区（管理员展示名 / 邮箱）
// ════════════════════════════════════════════════════════════

/** 从名称取首字母（最多 2 个），用于头像占位 */
function nameInitials(name: string): string {
  const s = name.trim()
  if (!s) return '?'
  return s
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0] ?? '')
    .join('')
    .toUpperCase()
}

function ProfileBlock() {
  const { t, i18n } = useTranslation()
  const { toast } = useToast()
  const { data: me } = useMe()
  const updateProfile = useUpdateProfile()

  const [displayName, setDisplayName] = React.useState('')
  const [email, setEmail] = React.useState('')

  // 资料加载后填充表单
  React.useEffect(() => {
    if (me) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDisplayName(me.display_name)
      setEmail(me.email)
    }
  }, [me])

  /** 本地化日期；无值回落到「从未」 */
  function fmtDate(s?: string): string {
    if (!s) return t('settings.account.never')
    try {
      return new Date(s).toLocaleString(i18n.language)
    } catch {
      return s
    }
  }

  function handleSave() {
    updateProfile.mutate(
      { display_name: displayName, email },
      { onSuccess: () => toast(t('settings.profile.saved')) },
    )
  }

  const avatarName = (displayName || me?.username || '').trim()

  return (
    <div className="settings-block">
      <h3>{t('settings.profile.title')}</h3>
      <p className="help">{t('settings.profile.help')}</p>

      {/* 头像 + 用户名概览。尺寸与形状全部交给 .settings-identity：
          此前尺寸写在内联样式里、形状（圆角/居中/白字）指望 .account-card 下的
          规则，而这里根本不在 .account-card 内——于是只剩一个直角色块。 */}
      <div className="settings-identity">
        <div className="ac-avatar" aria-hidden="true">
          {nameInitials(avatarName)}
        </div>
        <div className="ac-text">
          <div className="ac-name">{me?.username ?? '—'}</div>
          <div className="ac-mail">{me?.email || t('settings.profile.noEmail')}</div>
        </div>
      </div>

      <div style={{ maxWidth: 380, marginTop: 8 }}>
        {/* 用户名（只读，登录账号不可改）*/}
        <Row label={t('settings.profile.username')} help={t('settings.profile.usernameHint')}>
          <input type="text" value={me?.username ?? ''} readOnly disabled className="inline-input" />
        </Row>

        {/* 展示名 */}
        <Row label={t('settings.profile.displayName')}>
          <input
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder={me?.username ?? ''}
            className="inline-input"
          />
        </Row>

        {/* 联系邮箱 */}
        <Row label={t('settings.profile.email')}>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            className="inline-input"
          />
        </Row>

        {/* 元信息：创建时间 / 最后登录 */}
        <div style={{ fontSize: 12, color: 'var(--ink-3)', fontFamily: 'var(--font-mono)', marginTop: 4, lineHeight: 1.7 }}>
          <div>{t('settings.profile.created')}: {fmtDate(me?.created_at)}</div>
          <div>{t('settings.profile.lastLogin')}: {fmtDate(me?.last_login_at)}</div>
        </div>

        <div style={{ marginTop: 16 }}>
          <button
            type="button"
            className="pill-btn"
            onClick={handleSave}
            disabled={updateProfile.isPending}
          >
            {t('settings.profile.save')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：安全分区（改密码）
// ════════════════════════════════════════════════════════════

function PasswordBlock() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const changePassword = useChangePassword()

  const [oldPwd, setOldPwd] = React.useState('')
  const [newPwd, setNewPwd] = React.useState('')
  const [confirmPwd, setConfirmPwd] = React.useState('')
  const [status, setStatus] = React.useState<{ type: 'success' | 'error'; text: string } | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setStatus(null)

    if (!oldPwd.trim() || !newPwd.trim() || !confirmPwd.trim()) {
      setStatus({ type: 'error', text: t('settings.security.required') })
      return
    }
    if (newPwd !== confirmPwd) {
      setStatus({ type: 'error', text: t('settings.security.mismatch') })
      return
    }

    changePassword.mutate(
      { oldPassword: oldPwd, newPassword: newPwd },
      {
        onSuccess: () => {
          setStatus({ type: 'success', text: t('settings.security.success') })
          toast(t('settings.security.success'))
          setOldPwd('')
          setNewPwd('')
          setConfirmPwd('')
        },
        onError: () => {
          setStatus({ type: 'error', text: t('settings.security.wrongOld') })
        },
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.security.title')}</h3>
      <form onSubmit={handleSubmit} style={{ maxWidth: 380, marginTop: 8 }}>
        {/* 当前密码 */}
        <Row label={t('settings.security.oldPwd')}>
          <input
            type="password"
            value={oldPwd}
            onChange={(e) => setOldPwd(e.target.value)}
            autoComplete="current-password"
            className="inline-input"
          />
        </Row>

        {/* 新密码 */}
        <Row label={t('settings.security.newPwd')}>
          <input
            type="password"
            value={newPwd}
            onChange={(e) => setNewPwd(e.target.value)}
            autoComplete="new-password"
            className="inline-input"
          />
        </Row>

        {/* 确认新密码 */}
        <Row label={t('settings.security.confirmPwd')}>
          <input
            type="password"
            value={confirmPwd}
            onChange={(e) => setConfirmPwd(e.target.value)}
            autoComplete="new-password"
            className="inline-input"
          />
        </Row>

        {/* 状态消息 */}
        {status && (
          <div
            style={{
              fontSize: 13,
              padding: '8px 12px',
              borderRadius: 6,
              marginTop: 4,
              background: status.type === 'success' ? 'var(--accent-wash)' : 'oklch(0.577 0.245 27.325 / 0.1)',
              color: status.type === 'success' ? 'var(--accent)' : 'var(--destructive)',
            }}
          >
            {status.text}
          </div>
        )}

        <div style={{ marginTop: 16 }}>
          <button
            type="submit"
            className="pill-btn"
            disabled={changePassword.isPending}
          >
            {t('settings.security.submit')}
          </button>
        </div>
      </form>
    </div>
  )
}

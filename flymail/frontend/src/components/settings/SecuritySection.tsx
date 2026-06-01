import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useChangePassword } from '@/lib/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

// ────────────────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────────────────

type MessageType = 'success' | 'error'

interface StatusMessage {
  type: MessageType
  text: string
}

// ────────────────────────────────────────────────────────────────────────────────
// SecuritySection
// ────────────────────────────────────────────────────────────────────────────────

export function SecuritySection() {
  const { t } = useTranslation()
  const changePassword = useChangePassword()

  const [oldPwd, setOldPwd] = React.useState('')
  const [newPwd, setNewPwd] = React.useState('')
  const [confirmPwd, setConfirmPwd] = React.useState('')
  const [status, setStatus] = React.useState<StatusMessage | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setStatus(null)

    // 非空校验
    if (!oldPwd.trim() || !newPwd.trim() || !confirmPwd.trim()) {
      setStatus({ type: 'error', text: t('settings.security.required') })
      return
    }

    // 两次密码一致性校验
    if (newPwd !== confirmPwd) {
      setStatus({ type: 'error', text: t('settings.security.mismatch') })
      return
    }

    changePassword.mutate(
      { oldPassword: oldPwd, newPassword: newPwd },
      {
        onSuccess: () => {
          setStatus({ type: 'success', text: t('settings.security.success') })
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
    <div className="flex flex-col gap-6 p-6">
      <div
        className="text-sm font-semibold"
        style={{ color: 'var(--ink-2)' }}
      >
        {t('settings.security.title')}
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4 max-w-sm">
        {/* 当前密码 */}
        <div className="flex flex-col gap-1.5">
          <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
            {t('settings.security.oldPwd')}
          </Label>
          <Input
            type="password"
            value={oldPwd}
            onChange={(e) => setOldPwd(e.target.value)}
            autoComplete="current-password"
          />
        </div>

        {/* 新密码 */}
        <div className="flex flex-col gap-1.5">
          <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
            {t('settings.security.newPwd')}
          </Label>
          <Input
            type="password"
            value={newPwd}
            onChange={(e) => setNewPwd(e.target.value)}
            autoComplete="new-password"
          />
        </div>

        {/* 确认新密码 */}
        <div className="flex flex-col gap-1.5">
          <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
            {t('settings.security.confirmPwd')}
          </Label>
          <Input
            type="password"
            value={confirmPwd}
            onChange={(e) => setConfirmPwd(e.target.value)}
            autoComplete="new-password"
          />
        </div>

        {/* 状态消息 */}
        {status && (
          <div
            className="rounded-md px-3 py-2 text-sm"
            style={{
              background:
                status.type === 'success'
                  ? 'var(--accent-wash)'
                  : 'oklch(0.577 0.245 27.325 / 0.1)',
              color:
                status.type === 'success'
                  ? 'var(--accent-color)'
                  : 'var(--destructive)',
            }}
          >
            {status.text}
          </div>
        )}

        <Button
          type="submit"
          size="sm"
          className="self-start"
          disabled={changePassword.isPending}
        >
          {t('settings.security.submit')}
        </Button>
      </form>
    </div>
  )
}

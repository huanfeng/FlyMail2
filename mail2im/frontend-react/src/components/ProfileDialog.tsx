import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Separator } from '@/components/ui/separator'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ProfileDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const updateProfile = useAuthStore((s) => s.updateProfile)

  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (open && user) {
      setUsername(user.username || '')
      setEmail(user.email || '')
      setCurrentPassword('')
      setNewPassword('')
    }
  }, [open, user])

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim()) return

    setLoading(true)
    try {
      await updateProfile({
        username: username.trim(),
        email: email.trim() || undefined,
        current_password: currentPassword || undefined,
        new_password: newPassword || undefined,
      })
      toast.success(t('profile.save_success'))
      onOpenChange(false)
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      const msg = error.response?.data?.error
      toast.error(msg || t('profile.save_error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('profile.title')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSave} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="profile-username">{t('auth.username')}</Label>
            <Input
              id="profile-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t('auth.username_placeholder')}
              autoComplete="username"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="profile-email">{t('profile.email')}</Label>
            <Input
              id="profile-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t('profile.email_placeholder')}
              autoComplete="email"
            />
          </div>

          <Separator />

          <p className="text-sm text-muted-foreground">{t('profile.change_password_hint')}</p>

          <div className="space-y-2">
            <Label htmlFor="profile-current-password">{t('profile.current_password')}</Label>
            <Input
              id="profile-current-password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder={t('profile.current_password_placeholder')}
              autoComplete="current-password"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="profile-new-password">{t('profile.new_password')}</Label>
            <Input
              id="profile-new-password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder={t('profile.new_password_placeholder')}
              autoComplete="new-password"
            />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={loading}>
              {loading ? '...' : t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

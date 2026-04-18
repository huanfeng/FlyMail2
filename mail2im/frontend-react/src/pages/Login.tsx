import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Mail, Eye, EyeOff } from 'lucide-react'
import '@/services/api' // ensure interceptors are registered

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const setup = useAuthStore((s) => s.setup)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [isSetup, setIsSetup] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim() || !password.trim()) return

    setLoading(true)
    try {
      if (isSetup) {
        await setup(username, password)
      } else {
        await login(username, password)
      }
      navigate('/')
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string }; status?: number } }
      const msg = error.response?.data?.error

      if (!isSetup && error.response?.status === 400 && msg === 'user_exists') {
        // First visit but user already exists - not setup mode
      } else if (!isSetup && error.response?.status === 400 && msg !== 'user_exists') {
        // No user exists yet, switch to setup mode
        setIsSetup(true)
        toast.info(t('auth.setup_hint'))
      } else {
        toast.error(msg || t('auth.login_failed'))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center space-y-2">
          <div className="mx-auto h-12 w-12 rounded-xl bg-primary/10 flex items-center justify-center">
            <Mail className="h-6 w-6 text-primary" />
          </div>
          <CardTitle className="text-2xl font-semibold">Mail2IM</CardTitle>
          <p className="text-sm text-muted-foreground">
            {isSetup ? t('auth.setup_title') : t('auth.login_title')}
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">{t('auth.username')}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t('auth.username_placeholder')}
                autoComplete="username"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t('auth.password')}</Label>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t('auth.password_placeholder')}
                  autoComplete={isSetup ? 'new-password' : 'current-password'}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  tabIndex={-1}
                >
                  {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? '...' : isSetup ? t('auth.setup_button') : t('auth.login_button')}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

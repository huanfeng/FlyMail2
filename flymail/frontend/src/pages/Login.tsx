import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { login } from '@/lib/api'
import { parseRetryAfter, retryAfterText } from '@/lib/rate-limit'
import { savedLogin } from '@/lib/saved-login'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Mail, Eye, EyeOff } from 'lucide-react'

export function LoginPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()

  // 「记住密码」：勾选登录后保存凭据，下次打开自动填充；取消勾选登录即清除。
  const [saved] = useState(() => savedLogin.load())
  const [username, setUsername] = useState(saved?.username ?? 'admin')
  const [password, setPassword] = useState(saved?.password ?? '')
  const [remember, setRemember] = useState(saved !== null)
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  // 后端限流（15 分钟内失败 10 次）解禁的时刻；null = 未被限流。
  // 存的是绝对时刻而不是剩余秒数：这样倒计时只依赖时钟，
  // 标签页被挂起再回来也不会停在一个早就过期的数字上。
  const [blockedUntil, setBlockedUntil] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  // 限流刚刚解除：倒计时文字直接消失、按钮默默变可点，
  // 用户说不准现在到底能不能登，所以明确说一句。
  const [retryReady, setRetryReady] = useState(false)

  useEffect(() => {
    if (blockedUntil == null) return
    const timer = setInterval(() => {
      const nowMs = Date.now()
      setNow(nowMs)
      if (nowMs >= blockedUntil) {
        setBlockedUntil(null)
        setRetryReady(true)
      }
    }, 1000)
    return () => clearInterval(timer)
  }, [blockedUntil])

  const remainingSec =
    blockedUntil != null ? Math.max(0, Math.ceil((blockedUntil - now) / 1000)) : 0
  const rateLimited = remainingSec > 0
  const retry = retryAfterText(remainingSec)
  const rateLimitMsg = t(
    retry.unit === 'minutes' ? 'login.errRateLimitedMinutes' : 'login.errRateLimitedSeconds',
    { n: retry.value },
  )

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    // 限流期内连请求都不发：多打一次只会把后端的失败计数推得更远
    if (rateLimited) return
    if (!username.trim() || !password.trim()) return

    setLoading(true)
    setError('')
    setRetryReady(false)
    try {
      await login(username, password)
      if (remember) {
        savedLogin.save({ username, password })
      } else {
        savedLogin.clear()
      }
      navigate('/')
    } catch (err: unknown) {
      const status = (err as { response?: { status?: number } }).response?.status
      if (status === 429) {
        // 429 不会被 api.ts 的 401 刷新逻辑截走（那条分支只认 401），这里能原样拿到
        const sec = parseRetryAfter(err)
        if (sec > 0) {
          setBlockedUntil(Date.now() + sec * 1000)
          setNow(Date.now())
          setError('')
          setRetryReady(false)
        } else {
          // 后端没给可用的秒数：不编一个假的倒计时，退回通用文案
          setError(t('login.errRateLimitedUnknown'))
        }
      } else if (status === 401) {
        setError(t('login.errInvalid'))
      } else {
        setError(t('login.errGeneric'))
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
          <CardTitle className="text-2xl font-semibold">{t('app.name')}</CardTitle>
          <p className="text-sm text-muted-foreground">{t('login.subtitle')}</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">{t('login.username')}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t('login.username')}
                autoComplete="username"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t('login.password')}</Label>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t('login.password')}
                  autoComplete="current-password"
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
            <label className="flex items-center gap-2 text-sm text-muted-foreground cursor-pointer select-none">
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
                className="h-4 w-4 rounded border-input accent-primary"
              />
              {t('login.rememberPassword')}
            </label>
            {rateLimited ? (
              <p className="text-sm text-destructive">{rateLimitMsg}</p>
            ) : retryReady ? (
              <p className="text-sm text-muted-foreground">{t('login.rateLimitOver')}</p>
            ) : (
              error && <p className="text-sm text-destructive">{error}</p>
            )}
            <Button type="submit" className="w-full" disabled={loading || rateLimited}>
              {loading ? t('login.submitting') : t('login.submit')}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

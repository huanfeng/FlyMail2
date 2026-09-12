import { useEffect, useId, useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { login } from '@/lib/api'
import { parseRetryAfter, retryAfterText } from '@/lib/rate-limit'
import { savedLogin } from '@/lib/saved-login'
import { Icon } from '@/components/ui/Icon'

export function LoginPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const uid = useId()

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

  // 三种消息互斥，且都进同一个常驻的 live region——常驻是前几轮的教训：
  // 与内容同时插入 DOM 的 live region，读屏不播报。
  const message = rateLimited
    ? { text: rateLimitMsg, danger: true }
    : retryReady
      ? { text: t('login.rateLimitOver'), danger: false }
      : error
        ? { text: error, danger: true }
        : null

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-brand">
          <div className="login-logo" aria-hidden="true">
            <Icon name="mail" size={22} stroke={1.4} />
          </div>
          <h1 className="login-title">{t('app.name')}</h1>
          <p className="login-sub">{t('login.subtitle')}</p>
        </div>

        <div className="login-field">
          <label htmlFor={`${uid}-user`}>{t('login.username')}</label>
          <input
            id={`${uid}-user`}
            className="login-input"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder={t('login.username')}
            autoComplete="username"
            autoFocus
          />
        </div>

        <div className="login-field">
          <label htmlFor={`${uid}-pass`}>{t('login.password')}</label>
          <div className="login-input-wrap">
            <input
              id={`${uid}-pass`}
              className="login-input"
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('login.password')}
              autoComplete="current-password"
            />
            {/* 可聚焦（原先是 tabIndex={-1} 且没有可访问名，读屏只报「按钮」）。
                aria-pressed 表达的是「明文显示开着没开着」，比切换 label 更稳妥。 */}
            <button
              type="button"
              className="login-eye"
              onClick={() => setShowPassword(!showPassword)}
              aria-pressed={showPassword}
              aria-label={t('login.togglePassword')}
              title={t('login.togglePassword')}
            >
              <Icon name={showPassword ? 'eye-off' : 'eye'} size={16} />
            </button>
          </div>
        </div>

        <label className="login-remember">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
          />
          {t('login.rememberPassword')}
        </label>

        {/* 常驻占位：有消息才有文字，但区域一直在 DOM 里 */}
        <p
          className={'login-msg' + (message?.danger ? ' danger' : '')}
          role="status"
          aria-live="polite"
        >
          {message?.text ?? ''}
        </p>

        <button type="submit" className="login-submit" disabled={loading || rateLimited}>
          {loading ? t('login.submitting') : t('login.submit')}
        </button>
      </form>
    </div>
  )
}

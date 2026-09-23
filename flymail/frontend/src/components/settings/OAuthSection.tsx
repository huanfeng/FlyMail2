// 设置 → 授权登录（第三方 OAuth 应用凭据）。
//
// ── 为什么这项要有界面 ──────────────────────────────────────────────────────
//
// 凭据本来只认环境变量/配置文件。但配一个 OAuth 应用是**要反复试**的事：
// 回调地址填错、测试用户没加进白名单、secret 复制时漏一位——每试一次就要改
// compose 再重启一次容器。所以改成存数据库、改完即刻生效。
//
// client_secret 存的是密文，且**永不回显**：后端的 GET /settings 只回报「配没配」
// （见 setting.All）。因此这里能做的只有「覆盖」与「清除」，没有「查看」。
//
// ── 为什么自成一个分区、且不用 Row ────────────────────────────────────────
//
// 它最早挂在「账户」分区末尾、套着 Row 那套 `1fr auto` 两列栅格，结果是错位的：
// 左列装说明、右列装控件并垂直居中，而这里的说明长到能把左列撑高好几行；
// 「回调地址」那种整段提示更是直接塞进 auto 那列，把栅格撑破。
//
// 长说明不是可以删掉的累赘——去哪个后台、点哪个菜单、哪一步不做就会 403，
// 少一句用户就卡住。所以换个放法：整块纵向单列，分步骤的外部操作说明收进
// 默认折叠的 <details>，真正要看它的只有第一次配置那一遍。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiErrorMessage } from '@/lib/api'
import { useOAuthProviders, useSettings, useUpdateSettings } from '@/lib/queries'

/**
 * 配置指引：每步配一条控制台直达链接。
 *
 * 链接不是锦上添花——Google 控制台的菜单名近两年动过一轮（「OAuth 同意屏幕」
 * 并进了「Google 身份验证平台」），照着旧名字在控制台里翻是翻不到的，
 * 连「新建项目」都藏在顶栏项目选择器的弹窗里而不是任何菜单项下。
 * 所以按页给直达地址，绕开找菜单这一步。
 *
 * 顺序与 locales 里的 settings.oauth.guideStep1..N 一一对应。
 */
const GUIDE_STEPS: readonly string[] = [
  'https://console.cloud.google.com/projectcreate',
  'https://console.cloud.google.com/apis/library/gmail.googleapis.com',
  'https://console.cloud.google.com/auth/overview',
  'https://console.cloud.google.com/auth/scopes',
  'https://console.cloud.google.com/auth/audience',
  'https://console.cloud.google.com/auth/clients',
]

export function OAuthSection() {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const { data: providers } = useOAuthProviders()
  const update = useUpdateSettings()

  const google = (providers ?? []).find((p) => p.id === 'google')
  const storedID = settings?.oauth_google_client_id ?? ''
  const secretSaved = settings?.oauth_google_client_secret_set ?? false

  const [clientID, setClientID] = React.useState(storedID)
  // 空串在这里的意思是「不改动已保存的那份」，不是「清成空」——清除走单独的按钮。
  // 两者混为一谈的话，用户只改 client_id 点保存就会把 secret 连带抹掉。
  const [secret, setSecret] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)
  const [copied, setCopied] = React.useState(false)

  // 服务端数据回来后同步一次；用 ref 记住已同步过的值，避免冲掉用户正在输入的内容
  const syncedRef = React.useRef<string | null>(null)
  React.useEffect(() => {
    if (settings == null) return
    if (syncedRef.current === storedID) return
    syncedRef.current = storedID
    setClientID(storedID)
  }, [settings, storedID])

  function apply(payload: Record<string, string>, onOk?: () => void) {
    setError(null)
    update.mutate(payload, {
      onSuccess: () => {
        onOk?.()
        setSaved(true)
        setTimeout(() => setSaved(false), 2500)
      },
      onError: (err) => setError(apiErrorMessage(err, t('settings.oauth.errSave'))),
    })
  }

  function save() {
    const payload: Record<string, string> = { oauth_google_client_id: clientID.trim() }
    // 只在用户确实输入了新值时才提交 secret，否则保持库里那份不动
    if (secret !== '') payload.oauth_google_client_secret = secret
    apply(payload, () => setSecret(''))
  }

  function clearSecret() {
    apply({ oauth_google_client_secret: '' }, () => setSecret(''))
  }

  async function copyRedirect() {
    if (!google?.redirect_uri) return
    try {
      await navigator.clipboard.writeText(google.redirect_uri)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // 剪贴板在非安全上下文（局域网 http）里不可用，而 FlyMail 常这么部署。
      // 地址本来就显示在旁边，复制不了就让用户手选——不必弹错误吓人。
    }
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.oauth.title')}</h3>
      <p className="help">{t('settings.oauth.intro')}</p>

      {/* 去 Google 后台怎么操作：默认收起，只有第一次配置才需要展开 */}
      <details className="settings-guide">
        <summary>{t('settings.oauth.guideTitle')}</summary>
        <ol>
          {GUIDE_STEPS.map((href, i) => (
            <li key={href}>
              {t(`settings.oauth.guideStep${i + 1}`)}
              {/* 链接文字直接用地址本身：每条各不相同（读屏时不会读出一串「打开」），
                  而且点不开时（登录态不对、或把步骤抄去别的机器）还能照着敲。
                  rel 必须带 noopener，否则新开的页面能通过 window.opener 反向操纵本页。 */}
              <a className="sg-link" href={href} target="_blank" rel="noopener noreferrer">
                {href.replace('https://', '')}
              </a>
            </li>
          ))}
        </ol>
        <p className="sg-note">{t('settings.oauth.guideNoteProject')}</p>
        <p className="sg-note">{t('settings.oauth.guideNote')}</p>
      </details>

      {/* 回调地址排在两个输入框之前：它是要先拿去 Google 后台登记的那一步，
          顺序跟着实际操作走，而不是跟着「哪个字段更重要」走。 */}
      <div className="settings-field">
        <span className="sf-label">{t('settings.oauth.redirectUri')}</span>
        {google?.redirect_uri ? (
          <>
            <div className="settings-copybar">
              <code>{google.redirect_uri}</code>
              <Button size="sm" variant="outline" onClick={() => void copyRedirect()}>
                {copied ? t('settings.oauth.copied') : t('settings.oauth.copy')}
              </Button>
            </div>
            <p className="sf-help">{t('settings.oauth.redirectUriHelp')}</p>
          </>
        ) : (
          // 没配对外访问地址时走 loopback，而 loopback 要求浏览器与后端同机——
          // Docker/远程部署下必然走不通，这里直接说清楚该去哪补。
          <p className="sf-help">{t('settings.oauth.redirectUriMissing')}</p>
        )}
      </div>

      <div className="settings-field">
        <label className="sf-label" htmlFor="oauth-google-client-id">
          {t('settings.oauth.clientId')}
        </label>
        <Input
          id="oauth-google-client-id"
          placeholder="123456789-xxxx.apps.googleusercontent.com"
          value={clientID}
          onChange={(e) => { setClientID(e.target.value); setError(null) }}
        />
        <p className="sf-help">{t('settings.oauth.clientIdHelp')}</p>
      </div>

      <div className="settings-field">
        <label className="sf-label" htmlFor="oauth-google-client-secret">
          {t('settings.oauth.clientSecret')}
        </label>
        <div className="settings-copybar">
          <Input
            id="oauth-google-client-secret"
            type="password"
            autoComplete="new-password"
            style={{ flex: '1 1 280px', minWidth: 0 }}
            placeholder={secretSaved ? t('settings.oauth.secretSaved') : t('settings.oauth.secretEmpty')}
            value={secret}
            onChange={(e) => { setSecret(e.target.value); setError(null) }}
          />
          {secretSaved && (
            <Button size="sm" variant="ghost" onClick={clearSecret} disabled={update.isPending}>
              {t('settings.oauth.clearSecret')}
            </Button>
          )}
        </div>
        <p className="sf-help">{t('settings.oauth.clientSecretHelp')}</p>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 10, paddingTop: 14 }}>
        <Button size="sm" onClick={save} disabled={update.isPending}>
          {t('settings.oauth.save')}
        </Button>
        {saved && <span style={{ fontSize: 13, color: 'var(--accent)' }}>{t('settings.oauth.saved')}</span>}
      </div>

      {error != null && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: 8 }}>{error}</div>
      )}
    </div>
  )
}

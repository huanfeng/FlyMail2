// 设置 → 通知 → 对外访问地址。
//
// ── 为什么这项必须由用户填 ───────────────────────────────────────────────────
//
// 通知里那条「打开邮件」的链接要写成绝对地址，而**服务端没有可靠办法知道自己
// 对外是什么地址**：它看到的 Host 头可能是反向代理的内网名或容器名，监听地址
// 可能是 0.0.0.0，端口还可能被代理改写。猜错的后果是通知里带一条死链——
// 比不带链接更糟，因为用户会去点。
//
// 所以留空是合法状态：通知照发，只是不带链接。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings, useUpdateSettings } from '@/lib/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { checkBaseUrl } from '@/lib/base-url'
import { apiErrorMessage } from '@/lib/api'

interface Props {
  /** 复用设置页里的 Row，避免这里再手搓一套外观 */
  Row: React.ComponentType<{ label: string; help?: string; children: React.ReactNode }>
}

export function BaseUrlSection({ Row }: Props) {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const update = useUpdateSettings()

  const stored = settings?.app_base_url ?? ''
  const [value, setValue] = React.useState(stored)
  const [error, setError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)
  // 服务端数据回来之后同步一次；用 ref 记住已同步过的值，避免把用户正在输入的内容冲掉
  const syncedRef = React.useRef<string | null>(null)

  React.useEffect(() => {
    if (settings == null) return
    if (syncedRef.current === stored) return
    syncedRef.current = stored
    setValue(stored)
  }, [settings, stored])

  function save() {
    const verdict = checkBaseUrl(value)
    if (verdict !== 'ok') {
      setError(t(`settings.baseUrl.err_${verdict}`))
      return
    }
    setError(null)
    update.mutate(
      { app_base_url: value.trim() },
      {
        onSuccess: () => {
          setSaved(true)
          setTimeout(() => setSaved(false), 2500)
        },
        // 后端的拒绝理由是写给人看的中文（「缺少主机名」之类），而且这是个 400——
        // 重试一万次都一样。只说「保存失败，请稍后重试」的话用户会一直卡在那儿。
        onError: (err) => setError(apiErrorMessage(err, t('settings.baseUrl.err_save'))),
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.baseUrl.title')}</h3>
      <Row label={t('settings.baseUrl.label')} help={t('settings.baseUrl.hint')}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Input
            type="url"
            inputMode="url"
            placeholder="https://mail.example.com"
            style={{ width: 260 }}
            value={value}
            onChange={(e) => { setValue(e.target.value); setError(null) }}
            aria-label={t('settings.baseUrl.label')}
          />
          <Button size="sm" onClick={save} disabled={update.isPending}>
            {t('settings.baseUrl.save')}
          </Button>
        </div>
      </Row>
      {error != null && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {error}
        </div>
      )}
      {saved && (
        <span style={{ fontSize: 13, color: 'var(--accent)' }}>{t('settings.baseUrl.saved')}</span>
      )}
    </div>
  )
}

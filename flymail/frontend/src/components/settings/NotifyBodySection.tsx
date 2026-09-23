// 设置 → 通知 → 推送正文长度。
//
// ── 这个数字管什么、不管什么 ─────────────────────────────────────────────────
//
// 它是**排版偏好**：一条推送里最多放多少字。它不是安全上限——无论填多大，
// 后端都还会按序列化后的实际字节数再裁一道（飞书对卡片请求体有 30KB 硬限制，
// 超了整条静默发不出去，用户只会发现「有几封邮件没推送」）。
//
// 所以这里的上界 20000 不是技术限制，是「再多也没意义」：两万字的邮件推进
// 聊天群，谁也不会在那里读完。
//
// 另外它只在渠道配了「完整正文」那一档时才起作用；配「摘要」或「基本信息」的
// 渠道各有自己的内容闸门，跟这个数字无关。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings, useUpdateSettings } from '@/lib/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiErrorMessage } from '@/lib/api'

/** 与后端 setting.handler 的校验保持一致 */
const MIN = 0
const MAX = 20000
const DEFAULT = 8000

interface Props {
  Row: React.ComponentType<{ label: string; help?: string; children: React.ReactNode }>
}

export function NotifyBodySection({ Row }: Props) {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const update = useUpdateSettings()

  const stored = settings?.notify_body_runes ?? DEFAULT
  const [value, setValue] = React.useState<string>(String(stored))
  const [error, setError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)

  // 服务端数据回来后同步一次；用 ref 记住已同步过的值，免得冲掉用户正在输入的内容
  const syncedRef = React.useRef<number | null>(null)
  React.useEffect(() => {
    if (settings == null) return
    if (syncedRef.current === stored) return
    syncedRef.current = stored
    setValue(String(stored))
  }, [settings, stored])

  function save() {
    // 用 Number 而不是 parseInt：parseInt('80abc') 会得到 80，
    // 于是用户敲错的内容被悄悄接受成一个他没打算填的数。
    const n = Number(value.trim())
    if (!Number.isInteger(n) || n < MIN || n > MAX) {
      setError(t('settings.notifyBody.err_range', { min: MIN, max: MAX }))
      return
    }
    setError(null)
    update.mutate(
      { notify_body_runes: String(n) },
      {
        onSuccess: () => {
          setSaved(true)
          setTimeout(() => setSaved(false), 2500)
        },
        // 后端的拒绝理由是写给人看的中文，而且是 400——重试多少次都一样。
        onError: (err) => setError(apiErrorMessage(err, t('settings.notifyBody.err_save'))),
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.notifyBody.title')}</h3>
      <Row label={t('settings.notifyBody.label')} help={t('settings.notifyBody.hint')}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Input
            type="number"
            inputMode="numeric"
            min={MIN}
            max={MAX}
            style={{ width: 120 }}
            value={value}
            onChange={(e) => { setValue(e.target.value); setError(null) }}
            aria-label={t('settings.notifyBody.label')}
          />
          <Button size="sm" onClick={save} disabled={update.isPending}>
            {t('settings.notifyBody.save')}
          </Button>
        </div>
      </Row>
      {error != null && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {error}
        </div>
      )}
      {saved && (
        <span style={{ fontSize: 13, color: 'var(--accent)' }}>{t('settings.notifyBody.saved')}</span>
      )}
    </div>
  )
}

// 设置 → 通知 → 这台设备上的提醒。
//
// 与下面的「外发渠道」（Telegram / 飞书 / webhook）分开：那些是服务端推给别处的，
// 这几项只对**当前这个浏览器**成立——通知权限是浏览器授予当前源的，
// 在公司电脑上开了声音不等于手机上也想要。所以存 localStorage 而不是后端。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  getNotifyPrefs,
  setNotifyPrefs,
  subscribeNotifyPrefs,
  type NotifyPrefs,
} from '@/lib/notify-prefs'
import {
  notifyPermission,
  playChime,
  requestNotifyPermission,
  type NotifyPermission,
} from '@/lib/browser-notify'

interface Props {
  /** 复用设置页里的 Row / Toggle，避免这里再手搓一套外观 */
  Row: React.ComponentType<{ label: string; help?: string; children: React.ReactNode }>
  Toggle: React.ComponentType<{ on: boolean; onChange: (v: boolean) => void; ariaLabel?: string }>
}

export function BrowserNotifySection({ Row, Toggle }: Props) {
  const { t } = useTranslation()
  const prefs = React.useSyncExternalStore(subscribeNotifyPrefs, getNotifyPrefs, getNotifyPrefs)
  const [perm, setPerm] = React.useState<NotifyPermission>(() => notifyPermission())

  function set(patch: Partial<NotifyPrefs>) {
    setNotifyPrefs(patch)
  }

  /**
   * 打开桌面通知。
   *
   * 权限只能由用户的点击来求：页面一加载就弹权限框，多数浏览器直接拒绝或折叠，
   * 而且一旦被拒就再也问不了（denied 是粘住的）。所以这个开关本身就是那次点击。
   */
  async function handleDesktop(next: boolean) {
    if (!next) {
      set({ desktop: false })
      return
    }
    let p = notifyPermission()
    if (p === 'default') {
      p = await requestNotifyPermission()
      setPerm(p)
    }
    // 没拿到权限就不要把开关打开——显示成「已开启」而实际弹不出来，
    // 比明确告诉用户被浏览器拦了更糟
    set({ desktop: p === 'granted' })
  }

  const blocked = perm === 'denied'
  const unsupported = perm === 'unsupported'

  return (
    <div className="settings-block">
      <h3>{t('settings.notify.deviceTitle')}</h3>
      <p className="help">{t('settings.notify.deviceHint')}</p>

      <Row label={t('settings.notify.desktop')} help={t('settings.notify.desktopHint')}>
        <Toggle
          on={prefs.desktop && perm === 'granted'}
          onChange={(v) => void handleDesktop(v)}
          ariaLabel={t('settings.notify.desktop')}
        />
      </Row>

      {/* 权限被拒或不支持时必须说出来：否则用户只会看到一个「点了没反应」的开关 */}
      {(blocked || unsupported) && (
        <p className="help" style={{ color: 'var(--danger-ink)' }}>
          {blocked ? t('settings.notify.permissionDenied') : t('settings.notify.unsupported')}
        </p>
      )}

      <Row label={t('settings.notify.sound')} help={t('settings.notify.soundHint')}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {prefs.sound && (
            // 试听按钮不是装饰：提示音只在标签页不可见时才响，
            // 没有它用户在设置页里根本没机会知道自己开的是什么声音。
            <button type="button" className="pill-btn" onClick={() => playChime()}>
              {t('settings.notify.soundTest')}
            </button>
          )}
          <Toggle
            on={prefs.sound}
            onChange={(v) => {
              set({ sound: v })
              // 开的那一下就放一次：这次点击同时满足了浏览器的自动播放策略
              if (v) playChime()
            }}
            ariaLabel={t('settings.notify.sound')}
          />
        </div>
      </Row>

      <Row label={t('settings.notify.titleBadge')} help={t('settings.notify.titleBadgeHint')}>
        <Toggle
          on={prefs.titleBadge}
          onChange={(v) => set({ titleBadge: v })}
          ariaLabel={t('settings.notify.titleBadge')}
        />
      </Row>
    </div>
  )
}

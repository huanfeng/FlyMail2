// 外观页顶部的迷你三栏预览。
//
// 用真实的类名与令牌渲染（.mail-item / .avatar-sq / .folder-row…），只在外层 scale 缩小：
// 色板与排版只有 index.css 一份，预览不另画一套，改了样式预览自动跟上。
// data-theme / data-mode / data-contrast 挂在预览根上，令牌在子树内重新生效（同 ThemeCard 的做法）。
import { useTranslation } from 'react-i18next'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import type { Appearance } from '@/lib/appearance-prefs'

export function AppearancePreview({ tone, mode, listStyle, layoutMode, appearance }: {
  tone: ToneId; mode: ThemeMode; listStyle: ListStyle; layoutMode: LayoutMode; appearance: Appearance
}) {
  const { t } = useTranslation()
  const rows = [
    { from: t('settings.appearance.pvFrom1'), subject: t('settings.appearance.pvSubject1'), time: '09:41', unread: true, hue: 3 },
    { from: t('settings.appearance.pvFrom2'), subject: t('settings.appearance.pvSubject2'), time: t('settings.appearance.pvYesterday'), unread: false, hue: 6 },
    { from: t('settings.appearance.pvFrom3'), subject: t('settings.appearance.pvSubject3'), time: '10/12', unread: false, hue: 1 },
  ]
  return (
    <div
      className="appearance-preview"
      data-theme={tone}
      data-mode={mode}
      data-contrast={appearance.textContrast === 'high' ? 'high' : undefined}
      data-layout={layoutMode}
      aria-hidden="true"
    >
      <div className="ap-canvas" style={{ zoom: appearance.uiScale / 100 }}>
        <div className="ap-side">
          {[t('folder.inbox'), t('folder.sent'), t('folder.archive')].map((n, i) => (
            <div key={n} className={'folder-row' + (i === 0 ? ' active' : '')}>{n}</div>
          ))}
        </div>
        <div className="ap-list">
          {rows.map((r) => (
            <div key={r.subject} className={'mail-item' + (listStyle === 'compact' ? ' mail-item-row' : '') + (r.unread ? ' unread' : '')}>
              <span className="mi-unread-dot" />
              <span className="mi-avatar-wrap"><div className="avatar-sq" data-hue={r.hue}>{r.from.slice(0, 1)}</div></span>
              <div style={{ minWidth: 0 }}>
                <div className="mi-top"><span className="mi-sender">{r.from}</span><span className="mi-time">{r.time}</span></div>
                <div className="mi-subject">{r.subject}</div>
              </div>
            </div>
          ))}
        </div>
        <div className="ap-reader">
          <div className="ap-title">{rows[0].subject}</div>
          <div className="ap-line" /><div className="ap-line" style={{ width: '85%' }} /><div className="ap-line" style={{ width: '60%' }} />
        </div>
      </div>
    </div>
  )
}

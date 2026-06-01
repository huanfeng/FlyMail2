import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { getTheme, applyTheme } from '@/lib/theme'
import type { Theme } from '@/lib/theme'
import { Label } from '@/components/ui/label'

// ────────────────────────────────────────────────────────────────────────────────
// GeneralSection
// ────────────────────────────────────────────────────────────────────────────────

export function GeneralSection() {
  const { t, i18n } = useTranslation()
  const [currentTheme, setCurrentTheme] = React.useState<Theme>(getTheme)

  function handleLanguageChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const lng = e.target.value
    void i18n.changeLanguage(lng)
    localStorage.setItem('flymail_lang', lng)
  }

  function handleThemeChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const theme = e.target.value as Theme
    applyTheme(theme)
    setCurrentTheme(theme)
  }

  return (
    <div className="flex flex-col gap-6 p-6">
      {/* 语言 */}
      <div className="flex flex-col gap-1.5">
        <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
          {t('settings.general.language')}
        </Label>
        <select
          value={i18n.language.startsWith('zh') ? 'zh' : 'en'}
          onChange={handleLanguageChange}
          className="h-9 w-full max-w-xs rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
          style={{ color: 'var(--ink)' }}
        >
          <option value="zh">{t('settings.general.zh')}</option>
          <option value="en">{t('settings.general.en')}</option>
        </select>
      </div>

      {/* 主题 */}
      <div className="flex flex-col gap-1.5">
        <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
          {t('settings.general.theme')}
        </Label>
        <select
          value={currentTheme}
          onChange={handleThemeChange}
          className="h-9 w-full max-w-xs rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
          style={{ color: 'var(--ink)' }}
        >
          <option value="light">{t('settings.general.light')}</option>
          <option value="dark">{t('settings.general.dark')}</option>
        </select>
      </div>
    </div>
  )
}

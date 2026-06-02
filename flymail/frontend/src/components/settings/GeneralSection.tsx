import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { getTheme, applyTheme, TONES } from '@/lib/theme'
import type { ThemeMode, ToneId } from '@/lib/theme'
import { Label } from '@/components/ui/label'

// ────────────────────────────────────────────────────────────────────────────────
// GeneralSection — 通用设置：语言 / 亮暗模式 / 色调
// ────────────────────────────────────────────────────────────────────────────────

export function GeneralSection() {
  const { t, i18n } = useTranslation()

  // 从持久化偏好初始化本地状态
  const initial = getTheme()
  const [currentMode, setCurrentMode] = React.useState<ThemeMode>(initial.mode)
  const [currentTone, setCurrentTone] = React.useState<ToneId>(initial.tone)

  /** 切换语言 */
  function handleLanguageChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const lng = e.target.value
    void i18n.changeLanguage(lng)
    localStorage.setItem('flymail_lang', lng)
  }

  /** 切换亮/暗模式 */
  function handleModeChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const mode = e.target.value as ThemeMode
    applyTheme({ mode, tone: currentTone })
    setCurrentMode(mode)
  }

  /** 切换色调 */
  function handleToneChange(tone: ToneId) {
    applyTheme({ mode: currentMode, tone })
    setCurrentTone(tone)
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

      {/* 亮/暗模式 */}
      <div className="flex flex-col gap-1.5">
        <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
          {t('settings.general.theme')}
        </Label>
        <select
          value={currentMode}
          onChange={handleModeChange}
          className="h-9 w-full max-w-xs rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
          style={{ color: 'var(--ink)' }}
        >
          <option value="light">{t('settings.general.light')}</option>
          <option value="dark">{t('settings.general.dark')}</option>
        </select>
      </div>

      {/* 主题色调 */}
      <div className="flex flex-col gap-2">
        <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
          {t('settings.general.toneTitle')}
        </Label>
        <div className="flex flex-wrap gap-2">
          {TONES.map((tone) => {
            const isSelected = tone.id === currentTone
            return (
              <button
                key={tone.id}
                type="button"
                title={t(tone.nameKey)}
                onClick={() => handleToneChange(tone.id)}
                style={{
                  width: 28,
                  height: 28,
                  borderRadius: 6,
                  background: tone.swatch,
                  border: isSelected
                    ? '2.5px solid var(--accent-color)'
                    : '2.5px solid transparent',
                  outline: isSelected ? '2px solid var(--accent-wash)' : 'none',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  padding: 0,
                  position: 'relative',
                  transition: 'border-color 0.15s, outline 0.15s',
                }}
                aria-pressed={isSelected}
                aria-label={t(tone.nameKey)}
              >
                {/* 选中对勾 */}
                {isSelected && (
                  <svg
                    width="12"
                    height="12"
                    viewBox="0 0 12 12"
                    fill="none"
                    style={{ position: 'absolute' }}
                  >
                    <path
                      d="M2 6l3 3 5-5"
                      stroke="#fff"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                )}
              </button>
            )
          })}
        </div>
        {/* 当前色调名称提示 */}
        <span style={{ fontSize: '0.75rem', color: 'var(--ink-3)' }}>
          {t(`settings.general.tone.${currentTone}`)}
        </span>
      </div>
    </div>
  )
}

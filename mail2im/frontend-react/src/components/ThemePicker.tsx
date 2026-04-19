import { useState, useEffect } from 'react'
import { Sun, Moon } from 'lucide-react'

// ─── Theme definitions ────────────────────────────────────────────────────────

interface ThemeDef {
  id: string
  label: string
  light: { accent: string; bgAlt: string }
  dark: { accent: string; bgAlt: string }
}

const THEMES: ThemeDef[] = [
  { id: 'warm',     label: 'Warm',     light: { accent: '#b5886b', bgAlt: '#f5f3ee' }, dark: { accent: '#d2a482', bgAlt: '#26231d' } },
  { id: 'sky',      label: 'Sky',      light: { accent: '#4a86c2', bgAlt: '#f1f5f9' }, dark: { accent: '#7aaad8', bgAlt: '#1c2029' } },
  { id: 'lavender', label: 'Lavender', light: { accent: '#8d6cc4', bgAlt: '#f3f1f7' }, dark: { accent: '#b593e0', bgAlt: '#221f2d' } },
  { id: 'coral',    label: 'Coral',    light: { accent: '#d27a63', bgAlt: '#f8f0ec' }, dark: { accent: '#e89a84', bgAlt: '#261f1c' } },
  { id: 'slate',    label: 'Slate',    light: { accent: '#5b6470', bgAlt: '#eef0f3' }, dark: { accent: '#b6bdc8', bgAlt: '#1a1d22' } },
  { id: 'mint',     label: 'Mint',     light: { accent: '#3d9970', bgAlt: '#e7f3eb' }, dark: { accent: '#6dc99c', bgAlt: '#18211c' } },
  { id: 'butter',   label: 'Butter',   light: { accent: '#d4a72c', bgAlt: '#faf3df' }, dark: { accent: '#e8c560', bgAlt: '#232017' } },
  { id: 'rose',     label: 'Rose',     light: { accent: '#d6628a', bgAlt: '#f9e9ee' }, dark: { accent: '#e88aac', bgAlt: '#251b1f' } },
  { id: 'aqua',     label: 'Aqua',     light: { accent: '#2ba9b5', bgAlt: '#e3f2f6' }, dark: { accent: '#6cd1d8', bgAlt: '#142225' } },
]

const STORAGE_THEME = 'mail2im_ui_theme'
const STORAGE_DARK  = 'mail2im_ui_theme_dark'

// ─── Public helpers ───────────────────────────────────────────────────────────

export function applyTheme(themeId: string, dark: boolean) {
  document.documentElement.setAttribute('data-theme', themeId)
  document.documentElement.setAttribute('data-mode', dark ? 'dark' : 'light')
}

export function initThemeFromStorage() {
  const themeId = localStorage.getItem(STORAGE_THEME) ?? 'sky'
  const dark    = localStorage.getItem(STORAGE_DARK)  === 'true'
  applyTheme(themeId, dark)
}

// ─── Component ────────────────────────────────────────────────────────────────

interface ThemePickerProps {
  /** Called after theme/mode changes, so parent can update its own state */
  onClose?: () => void
}

export function ThemePicker({ onClose }: ThemePickerProps) {
  const [activeTheme, setActiveTheme] = useState<string>(
    () => localStorage.getItem(STORAGE_THEME) ?? 'sky'
  )
  const [isDark, setIsDark] = useState<boolean>(
    () => localStorage.getItem(STORAGE_DARK) === 'true'
  )

  useEffect(() => {
    applyTheme(activeTheme, isDark)
    localStorage.setItem(STORAGE_THEME, activeTheme)
    localStorage.setItem(STORAGE_DARK, String(isDark))
  }, [activeTheme, isDark])

  return (
    <div style={{ padding: '0 0 12px' }}>
      {/* Section label */}
      <div style={{
        padding: '12px 14px 6px',
        fontFamily: 'var(--font-mono, ui-monospace)',
        fontSize: '10.5px',
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        color: 'var(--ink-3, #8a857a)',
      }}>
        主题色
      </div>

      {/* 3×3 swatch grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(3, 1fr)',
        gap: '8px',
        padding: '0 14px',
      }}>
        {THEMES.map((theme) => {
          const colors = isDark ? theme.dark : theme.light
          const isActive = theme.id === activeTheme
          return (
            <button
              key={theme.id}
              title={theme.label}
              onClick={() => setActiveTheme(theme.id)}
              style={{
                aspectRatio: '1.6',
                borderRadius: '8px',
                position: 'relative',
                cursor: 'pointer',
                border: isActive
                  ? '2px solid var(--mm-accent, #4a86c2)'
                  : '1px solid var(--rule, rgba(0,0,0,0.08))',
                overflow: 'hidden',
                background: 'none',
                padding: 0,
                transition: 'all 0.12s',
                boxShadow: isActive ? '0 0 0 2px var(--accent-wash, #e6f0f9)' : 'none',
                transform: isActive ? 'none' : undefined,
              }}
              onMouseEnter={(e) => {
                if (!isActive) {
                  (e.currentTarget as HTMLButtonElement).style.transform = 'translateY(-1px)'
                  ;(e.currentTarget as HTMLButtonElement).style.borderColor = 'var(--rule-strong, rgba(0,0,0,0.14))'
                }
              }}
              onMouseLeave={(e) => {
                if (!isActive) {
                  (e.currentTarget as HTMLButtonElement).style.transform = 'none'
                  ;(e.currentTarget as HTMLButtonElement).style.borderColor = 'var(--rule, rgba(0,0,0,0.08))'
                }
              }}
            >
              {/* Top band = accent */}
              <div style={{
                position: 'absolute', inset: '0 0 40% 0',
                background: colors.accent,
              }} />
              {/* Bottom band = bg-alt */}
              <div style={{
                position: 'absolute', inset: '60% 0 0 0',
                background: colors.bgAlt,
              }} />
              {/* Label */}
              <div style={{
                position: 'absolute', bottom: 3, left: 6, right: 6,
                fontSize: '10px',
                fontFamily: 'var(--font-mono, ui-monospace)',
                color: isDark ? 'rgba(255,255,255,0.85)' : 'rgba(0,0,0,0.75)',
                letterSpacing: '0.02em',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                textAlign: 'left',
              }}>
                {theme.label}
              </div>
            </button>
          )
        })}
      </div>

      {/* Light / Dark mode toggle */}
      <div style={{ padding: '12px 14px 6px', fontFamily: 'var(--font-mono, ui-monospace)', fontSize: '10.5px', letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--ink-3, #8a857a)' }}>
        模式
      </div>
      <div style={{
        margin: '0 14px',
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: '4px',
        padding: '3px',
        borderRadius: '8px',
        background: 'var(--bg-alt, #f1f5f9)',
        border: '1px solid var(--rule, rgba(0,0,0,0.08))',
      }}>
        <button
          onClick={() => setIsDark(false)}
          style={{
            padding: '6px',
            borderRadius: '6px',
            fontSize: '12.5px',
            color: !isDark ? 'var(--ink, #1f2937)' : 'var(--ink-2, #4b5563)',
            background: !isDark ? 'var(--surface, #fff)' : 'transparent',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '6px',
            border: 'none',
            cursor: 'pointer',
            boxShadow: !isDark ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
            transition: 'all 0.12s',
          }}
        >
          <Sun size={13} />
          浅色
        </button>
        <button
          onClick={() => setIsDark(true)}
          style={{
            padding: '6px',
            borderRadius: '6px',
            fontSize: '12.5px',
            color: isDark ? 'var(--ink, #e6ebf3)' : 'var(--ink-2, #4b5563)',
            background: isDark ? 'var(--surface, #1c2029)' : 'transparent',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '6px',
            border: 'none',
            cursor: 'pointer',
            boxShadow: isDark ? '0 1px 2px rgba(0,0,0,0.1)' : 'none',
            transition: 'all 0.12s',
          }}
        >
          <Moon size={13} />
          深色
        </button>
      </div>

      {onClose && (
        <div style={{ margin: '12px 14px 0' }}>
          <button
            onClick={onClose}
            style={{
              width: '100%',
              padding: '8px 12px',
              borderRadius: '8px',
              border: '1px solid var(--rule, rgba(0,0,0,0.08))',
              background: 'var(--bg-alt, #f1f5f9)',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              fontSize: '13px',
              color: 'var(--ink, #1f2937)',
              cursor: 'pointer',
              transition: 'background 0.1s',
            }}
          >
            完成
          </button>
        </div>
      )}
    </div>
  )
}

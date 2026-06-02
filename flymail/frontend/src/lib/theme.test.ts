import { describe, it, expect, beforeEach } from 'vitest'
import { getTheme, applyTheme, initTheme, TONES } from '@/lib/theme'
import type { ThemePref } from '@/lib/theme'

// ────────────────────────────────────────────────────────────────────────────────
// theme.ts 单元测试
// 环境：jsdom（vitest.config.ts 已配置 environment: 'jsdom'）
// ────────────────────────────────────────────────────────────────────────────────

beforeEach(() => {
  // 每个测试前清空 localStorage 并重置 documentElement
  localStorage.clear()
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-mode')
  document.documentElement.classList.remove('dark')
})

describe('getTheme()', () => {
  it('localStorage 清空后返回默认值 { mode: light, tone: slate }', () => {
    const pref = getTheme()
    expect(pref).toEqual<ThemePref>({ mode: 'light', tone: 'slate' })
  })

  it('非法 tone 值回落到 slate', () => {
    localStorage.setItem('flymail_theme_tone', 'nonexistent')
    localStorage.setItem('flymail_theme_mode', 'light')
    const pref = getTheme()
    expect(pref.tone).toBe('slate')
  })

  it('非法 mode 值回落到 light', () => {
    localStorage.setItem('flymail_theme_mode', 'auto')
    const pref = getTheme()
    expect(pref.mode).toBe('light')
  })

  it('合法值正常读取', () => {
    localStorage.setItem('flymail_theme_mode', 'dark')
    localStorage.setItem('flymail_theme_tone', 'sky')
    expect(getTheme()).toEqual<ThemePref>({ mode: 'dark', tone: 'sky' })
  })
})

describe('applyTheme()', () => {
  it('同时设置 data-theme、data-mode 属性和 dark class', () => {
    applyTheme({ mode: 'dark', tone: 'sky' })
    // MailMaster CSS 使用 data-theme + data-mode 双属性选择器
    expect(document.documentElement.dataset.theme).toBe('sky')
    expect(document.documentElement.dataset.mode).toBe('dark')
    // shadcn 组件使用 .dark class（@custom-variant dark）
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('亮色模式：data-mode="light" 且移除 dark class', () => {
    // 先设置暗色状态
    document.documentElement.classList.add('dark')
    document.documentElement.dataset.mode = 'dark'
    applyTheme({ mode: 'light', tone: 'warm' })
    expect(document.documentElement.dataset.theme).toBe('warm')
    expect(document.documentElement.dataset.mode).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('持久化：applyTheme 后 getTheme() 返回相同偏好', () => {
    const pref: ThemePref = { mode: 'dark', tone: 'lavender' }
    applyTheme(pref)
    expect(getTheme()).toEqual(pref)
  })

  it('持久化所有 9 个色调', () => {
    for (const tone of TONES) {
      applyTheme({ mode: 'light', tone: tone.id })
      expect(getTheme().tone).toBe(tone.id)
    }
  })
})

describe('initTheme()', () => {
  it('读取持久化偏好并同时设置 data-theme、data-mode 和 dark class', () => {
    localStorage.setItem('flymail_theme_mode', 'dark')
    localStorage.setItem('flymail_theme_tone', 'mint')
    initTheme()
    expect(document.documentElement.dataset.theme).toBe('mint')
    expect(document.documentElement.dataset.mode).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('无持久化时应用默认值（light + slate）', () => {
    initTheme()
    expect(document.documentElement.dataset.theme).toBe('slate')
    expect(document.documentElement.dataset.mode).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})

describe('TONES', () => {
  it('包含 9 个色调', () => {
    expect(TONES).toHaveLength(9)
  })

  it('每个色调有合法的 id / nameKey / swatch', () => {
    for (const tone of TONES) {
      expect(tone.id).toBeTruthy()
      expect(tone.nameKey).toMatch(/^settings\.general\.tone\./)
      expect(tone.swatch).toMatch(/^#[0-9a-f]{6}$/i)
    }
  })
})

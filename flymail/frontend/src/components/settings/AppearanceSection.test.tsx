import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { AppearanceSection } from './sections/AppearanceSection'
import { getAppearance, setAppearance } from '@/lib/appearance-prefs'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh', changeLanguage: () => Promise.resolve() } }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('外观页', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    localStorage.clear(); setAppearance({})
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })

  afterEach(async () => { await act(async () => root.unmount()); container.remove() })

  async function mount() {
    await act(async () => root.render(
      <AppearanceSection listStyle="card" onChangeListStyle={() => {}}
        onChangeAlwaysShowSelect={() => {}} alwaysShowSelect={false} layoutMode="three" onChangeLayoutMode={() => {}} />,
    ))
  }

  it('顶部有预览，且挂着当前色调与亮暗', async () => {
    await mount()
    const p = container.querySelector('.appearance-preview')!
    expect(p).toBeTruthy()
    expect(p.getAttribute('data-theme')).toBeTruthy()
    expect(p.getAttribute('data-mode')).toMatch(/light|dark/)
  })

  it('切换文字对比：写入偏好，预览同步 data-contrast', async () => {
    await mount()
    const btn = [...container.querySelectorAll('button')].find((b) => b.textContent === 'settings.appearance.contrastHigh')!
    await act(async () => btn.click())
    expect(getAppearance().textContrast).toBe('high')
    expect(container.querySelector('.appearance-preview')!.getAttribute('data-contrast')).toBe('high')
  })

  it('界面字号四档', async () => {
    await mount()
    const group = container.querySelector('[data-pref="uiScale"]')!
    expect([...group.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['90%', '100%', '110%', '120%'])
  })

  it('「恢复默认」只重置本组', async () => {
    setAppearance({ textContrast: 'high', bodyFontSize: 16 })
    await mount()
    const reset = container.querySelector<HTMLButtonElement>('[data-reset="text"]')!
    await act(async () => reset.click())
    expect(getAppearance().textContrast).toBe('standard')
    expect(getAppearance().bodyFontSize).toBe(16)
  })
})

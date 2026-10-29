import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ReadingSection } from './sections/ReadingSection'
import { getAppearance, setAppearance } from '@/lib/appearance-prefs'

vi.mock('@/components/settings/TrustedSendersSection', () => ({
  TrustedSendersSection: () => null,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh', changeLanguage: () => Promise.resolve() } }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('阅读页「邮件正文」分组', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    localStorage.clear(); setAppearance({})
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })

  afterEach(async () => { await act(async () => root.unmount()); container.remove() })

  async function mount() {
    await act(async () => root.render(
      <ReadingSection conversationView={false} onChangeConversationView={() => {}} />,
    ))
  }

  it('正文分组四项控件齐全，位于会话视图之后、阅读隐私之前', async () => {
    await mount()
    const titles = [...container.querySelectorAll('.settings-block h3')].map((h) => h.textContent)
    expect(titles.indexOf('settings.reading.bodyTitle')).toBe(titles.indexOf('settings.reading.viewTitle') + 1)
    for (const k of ['bodyFontSize', 'bodyLineHeight', 'readingWidth', 'bodyZoom']) {
      expect(container.querySelector(`[data-pref="${k}"]`), k).toBeTruthy()
    }
  })

  it('拨动字号：写入偏好，预览 iframe 的文档随之更新', async () => {
    await mount()
    const btn = container.querySelector<HTMLButtonElement>('[data-pref="bodyFontSize"] button:last-child')!
    await act(async () => btn.click())
    expect(getAppearance().bodyFontSize).toBe(18)
    expect(container.querySelector('.body-preview iframe')!.getAttribute('srcdoc')).toContain('font-size: 18px')
  })

  it('预览 iframe 带与阅读区相同的 sandbox', async () => {
    await mount()
    expect(container.querySelector('.body-preview iframe')!.getAttribute('sandbox')).toContain('allow-scripts')
  })
})

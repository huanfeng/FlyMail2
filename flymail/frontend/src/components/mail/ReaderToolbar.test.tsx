import { describe, it, expect, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ReaderToolbar } from '@/components/mail/ReaderToolbar'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 窄阅读容器下工具条切图标模式（CSS 隐藏 .tb-btn > span），
 * 读屏与悬浮提示只剩 aria-label 可依赖——每颗按钮都必须有。
 */
describe('ReaderToolbar 可访问名', () => {
  it('每颗按钮在文字隐藏后仍有 aria-label', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root: Root = createRoot(host)
    act(() => {
      root.render(
        <ReaderToolbar
          onPrev={() => {}}
          onNext={() => {}}
          onReply={() => {}}
          onForward={() => {}}
          onTranslate={() => {}}
          onArchive={() => {}}
          onDelete={() => {}}
          moreItems={[]}
        />,
      )
    })
    const btns = Array.from(host.querySelectorAll<HTMLButtonElement>('.tb-btn'))
    expect(btns.length).toBeGreaterThanOrEqual(7)
    for (const b of btns) {
      expect(b.getAttribute('aria-label'), b.outerHTML).toBeTruthy()
    }
    act(() => root.unmount())
    host.remove()
  })
})

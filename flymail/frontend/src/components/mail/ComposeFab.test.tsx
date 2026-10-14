import { describe, it, expect, vi } from 'vitest'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { ComposeFab } from '@/components/mail/ComposeFab'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('ComposeFab', () => {
  it('是带可访问名的按钮，点击触发写邮件', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const onCompose = vi.fn()
    act(() => root.render(<ComposeFab onCompose={onCompose} />))
    const btn = host.querySelector<HTMLButtonElement>('button.compose-fab')!
    expect(btn).not.toBeNull()
    expect(btn.getAttribute('aria-label')).toBe('sidebar.compose')
    act(() => btn.click())
    expect(onCompose).toHaveBeenCalledTimes(1)
    act(() => root.unmount())
    host.remove()
  })
})

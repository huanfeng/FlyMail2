import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HealthBanner } from '@/components/mail/HealthBanner'
import type { Account } from '@/lib/types'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => k }) }))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function acct(id: number, name: string, health?: Account['health']): Account {
  return {
    id, name, email: `${name}@example.com`, auth_type: 'password',
    imap_host: 'i', imap_port: 993, imap_security: 'ssl',
    smtp_host: 's', smtp_port: 465, smtp_security: 'ssl',
    status: health?.needs_reauth ? 'needs_reauth' : 'ok', enabled: true,
    health,
  }
}

describe('HealthBanner', () => {
  let qc: QueryClient
  let container: HTMLDivElement
  let root: Root

  async function renderWith(accounts: Account[]) {
    qc.setQueryData(['accounts'], accounts)
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <HealthBanner />
        </QueryClientProvider>,
      )
    })
  }

  /** 换一份数据重渲染（模拟 ['accounts'] 缓存被 SSE invalidate 后的刷新） */
  async function rerenderWith(accounts: Account[]) {
    qc.setQueryData(['accounts'], accounts)
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <HealthBanner />
        </QueryClientProvider>,
      )
    })
  }

  function banner(): HTMLElement | null {
    return container.querySelector('.health-banner')
  }

  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
  }

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    sessionStorage.clear()
  })

  afterEach(async () => {
    await act(async () => { root.unmount() })
    container.remove()
  })

  it('无故障账户不渲染横幅', async () => {
    await renderWith([acct(1, 'a')])
    expect(banner()).toBe(null)
  })

  it('一个故障账户：横幅出现并点名', async () => {
    await renderWith([
      acct(1, 'a'),
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    expect(banner()).not.toBe(null)
    expect(banner()!.textContent).toContain('b')
    expect(banner()!.textContent).toContain('account.healthBanner.title')
  })

  it('三个故障账户：点名前 2 个，其余并作「等 N 个」', async () => {
    await renderWith([
      acct(1, 'x', { needs_reauth: false, breaker_open: true, failures: 5 }),
      acct(2, 'y', { needs_reauth: false, breaker_open: true, failures: 2 }),
      acct(3, 'z', { needs_reauth: false, breaker_open: true, failures: 3 }),
    ])
    const text = banner()!.textContent ?? ''
    expect(text).toContain('x')
    expect(text).toContain('y')
    expect(text).not.toContain('z')
    expect(text).toContain('account.healthBanner.more')
  })

  it('点「知道了」后横幅消失且记忆落在 sessionStorage', async () => {
    await renderWith([
      acct(1, 'a'),
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    const btn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'account.healthBanner.dismiss',
    )
    expect(btn).toBeTruthy()
    await act(async () => { btn!.click() })
    await flush()
    expect(banner()).toBe(null)
    expect(JSON.parse(sessionStorage.getItem('flymail-health-dismissed') ?? '[]')).toContain(2)
  })

  it('关闭后账户恢复再故障：横幅重现（恢复时已清记忆）', async () => {
    await renderWith([
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    const btn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'account.healthBanner.dismiss',
    )
    await act(async () => { btn!.click() })
    await flush()
    expect(banner()).toBe(null)

    // 恢复（faultyIds 不再含 2）→ 记忆被清
    await rerenderWith([acct(2, 'b')])
    await flush()
    const stored = JSON.parse(sessionStorage.getItem('flymail-health-dismissed') ?? '[]')
    expect(stored).not.toContain(2)

    // 再故障 → 横幅重现
    await rerenderWith([
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    await flush()
    expect(banner()).not.toBe(null)
  })
})

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { SyncProgress } from '@/components/mail/AccountSidebar'
import type { SyncStatus } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 同步进度行的无障碍结构。
 *
 * 这里钉的几乎全是**「什么不该在 live region 里」**。原因是首次导入几千封是
 * 分钟级操作、轮询间隔 1 秒：整块若是 live region，读屏用户就会连续几分钟
 * 每秒听一句「正在同步邮件 37 / 2000」，而新邮件提醒、操作结果、Shell 那个
 * announce 全被挤掉——比没有进度表达更糟。
 *
 * 这种缺陷肉眼完全看不出来（视觉上一模一样），所以只能靠断言结构来守。
 */
describe('SyncProgress 的无障碍结构', () => {
  let container: HTMLDivElement
  let root: Root

  async function render(status: SyncStatus | null) {
    await act(async () => root.render(<SyncProgress status={status} />))
  }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('外层不是 live region——计数每秒都在变', async () => {
    await render({ phase: 'messages', total: 2000, processed: 37 })
    const box = container.querySelector('.sync-progress')!
    expect(box.getAttribute('aria-live'), '整块成了 live region，计数会被每秒播报一次').toBeNull()
    expect(box.getAttribute('role')).not.toBe('status')
  })

  it('可见的计数文本对读屏隐藏（progressbar 已经完整表达了它）', async () => {
    await render({ phase: 'messages', total: 2000, processed: 37 })
    const text = container.querySelector('.sync-progress-text')!
    expect(text.textContent).toContain('37 / 2000')
    expect(text.getAttribute('aria-hidden')).toBe('true')
  })

  it('只有阶段名进 live region，一次同步最多播报三次', async () => {
    await render({ phase: 'messages', total: 2000, processed: 37 })
    const live = container.querySelector('[aria-live="polite"]')!
    expect(live, '阶段变化完全不播报，读屏用户无从知道同步进行到哪一步').not.toBeNull()
    expect(live.textContent, 'live region 里混进了每秒变化的计数').toBe('sync.messages')

    // 计数变了，live region 的内容必须不变——否则等于每秒播报
    await render({ phase: 'messages', total: 2000, processed: 38 })
    expect(container.querySelector('[aria-live="polite"]')!.textContent).toBe('sync.messages')
  })

  it('进度条带 valuetext，读屏按自己的节奏查询时听到的是「37 / 2000」', async () => {
    await render({ phase: 'messages', total: 2000, processed: 37 })
    const bar = container.querySelector('[role="progressbar"]')!
    expect(bar.getAttribute('aria-valuenow')).toBe('37')
    expect(bar.getAttribute('aria-valuemax')).toBe('2000')
    expect(bar.getAttribute('aria-valuetext')).toBe('37 / 2000')
  })

  it('还不知道总数时走不确定态，不给 valuenow', async () => {
    // queued / folders 阶段后端还没数出来。给一个 valuenow=0 会让读屏念「0%」，
    // 那是个错的事实——不是「进度为零」，是「还不知道」。
    await render({ phase: 'queued' })
    const bar = container.querySelector('[role="progressbar"]')!
    expect(bar.classList.contains('indeterminate')).toBe(true)
    expect(bar.getAttribute('aria-valuenow')).toBeNull()
    expect(bar.getAttribute('aria-label')).toBe('sync.queued')
  })
})

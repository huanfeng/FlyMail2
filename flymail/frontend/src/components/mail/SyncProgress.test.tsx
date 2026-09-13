import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { BodyPrefetchNote, SyncProgress } from '@/components/mail/AccountSidebar'
import type { SyncStatus } from '@/lib/types'

// 插值也要带出来：BodyPrefetchNote 的封数是通过 t 的参数传的，
// 只返回 key 的话「显示了多少封」这件事根本断言不到。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join('/')}` : k,
  }),
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

  it('外层不是 live region——进度在长同步里会变很多次', async () => {
    await render({ phase: 'messages', folders_total: 12, folders_done: 3 })
    const box = container.querySelector('.sync-progress')!
    expect(box.getAttribute('aria-live'), '整块成了 live region，计数会被每秒播报一次').toBeNull()
    expect(box.getAttribute('role')).not.toBe('status')
  })

  it('可见的进度文本对读屏隐藏（progressbar 已经完整表达了它）', async () => {
    await render({ phase: 'messages', folders_total: 12, folders_done: 3 })
    const text = container.querySelector('.sync-progress-text')!
    expect(text.textContent).toContain('sync.folderProgress')
    expect(text.getAttribute('aria-hidden')).toBe('true')
  })

  it('只有阶段名进 live region，一次同步最多播报三次', async () => {
    await render({ phase: 'messages', folders_total: 12, folders_done: 3 })
    const live = container.querySelector('[aria-live="polite"]')!
    expect(live, '阶段变化完全不播报，读屏用户无从知道同步进行到哪一步').not.toBeNull()
    expect(live.textContent, 'live region 里混进了每秒变化的计数').toBe('sync.messages')

    // 进度变了，live region 的内容必须不变——否则等于每推进一个文件夹播报一次
    await render({ phase: 'messages', folders_total: 12, folders_done: 4 })
    expect(container.querySelector('[aria-live="polite"]')!.textContent).toBe('sync.messages')
  })

  it('进度条按文件夹计数，valuetext 带上当前文件夹', async () => {
    await render({
      phase: 'messages',
      folders_total: 12,
      folders_done: 3,
      current_folder: '我的项目',
      current_folder_type: 'custom',
    })
    const bar = container.querySelector('[role="progressbar"]')!
    expect(bar.getAttribute('aria-valuenow')).toBe('3')
    expect(bar.getAttribute('aria-valuemax')).toBe('12')
    expect(bar.getAttribute('aria-valuetext')).toContain('我的项目')
  })

  it('系统文件夹显示本地化名，不是服务器原名', async () => {
    // 后端给的是 IMAP 那边的名字（INBOX / Sent Items / 已发送，各家拼法不一），
    // 而侧栏的文件夹列表对系统文件夹显示的是 t('folder.inbox')。
    // 这里若直接显示 current_folder，同一个文件夹在两处叫两个名字。
    await render({
      phase: 'messages',
      folders_total: 12,
      folders_done: 3,
      current_folder: 'INBOX',
      current_folder_type: 'inbox',
    })
    const text = container.querySelector('.sync-progress-text')!
    expect(text.textContent, '进度行显示了服务器原名').not.toContain('INBOX')
    expect(text.textContent).toContain('folder.inbox')
  })

  it('没有文件夹进度时走不确定态', async () => {
    // 早先这里读的是 status.total / processed，而后端只在同步结束时写它们，
    // messages 阶段恒为 0——确定态那一支在真实路径上一次都没渲染过，
    // 而代码和测试都看起来像是做了进度。那两个字段现已从协议里整个删掉，
    // 免得下一个人又拿一个恒为 0 的字段去画进度。
    await render({ phase: 'messages' })
    const bar = container.querySelector('[role="progressbar"]')!
    expect(bar.classList.contains('indeterminate')).toBe(true)
    expect(bar.getAttribute('aria-valuenow')).toBeNull()
    expect(container.querySelector('.sync-bar-fill')).toBeNull()
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

/**
 * 正文回补的弱表达。
 *
 * 它**不是**同步阶段，这一点是设计的核心：此刻邮件列表已经完整、缺的只是正文。
 * 做成阶段有两个坏处——前端把五个 invalidateQueries 挂在 done 上，推迟 done
 * 就等于「用户点了同步，新邮件几十秒后才出现在列表里」；而用「同步中」的转圈
 * 表达它，又会让用户以为邮件还没收全。
 */
describe('BodyPrefetchNote', () => {
  let container: HTMLDivElement
  let root: Root

  async function render(status: SyncStatus | null) {
    await act(async () => root.render(<BodyPrefetchNote status={status} />))
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

  it('没有待补正文时什么都不渲染', async () => {
    await render({ phase: 'done' })
    expect(container.textContent).toBe('')
    await render({ phase: 'done', bodies_total: 0, bodies_done: 0 })
    expect(container.textContent).toBe('')
  })

  it('有待补正文时显示封数级进度', async () => {
    // 分母是后端一轮捞到的待补条数，分子是已落库封数——
    // 比同步那边的文件夹粒度精确得多。
    await render({ phase: 'done', bodies_total: 200, bodies_done: 40 })
    expect(container.textContent).toContain('40')
    expect(container.textContent).toContain('200')
    expect(container.textContent).toContain('sync.bodies')
  })

  it('不转圈、不画进度条——表达强度要比"同步中"低一档', async () => {
    await render({ phase: 'done', bodies_total: 200, bodies_done: 40 })
    expect(container.querySelector('.spin-anim'), '正文回补不该转圈').toBeNull()
    expect(container.querySelector('[role="progressbar"]')).toBeNull()
    expect(container.querySelector('.sync-progress')).toBeNull()
  })
})

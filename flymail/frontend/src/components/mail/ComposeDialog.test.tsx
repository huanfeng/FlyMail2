import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { ComposeDialog } from '@/components/mail/ComposeDialog'
import { uploadRatio } from '@/lib/queries'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

// Tiptap 在 jsdom 里跑得起来但很慢，而这里验的是消息条的**位置**，与编辑器无关
vi.mock('@/components/mail/composer/RichEditor', () => ({
  RichEditor: () => null,
}))

// ⚠ 形状必须和真的 useToast() 对得上：它给的是 { toast, dismiss }，不是 { show }。
// 写错了不会当场报错——只有真正走到 toast() 那一支的用例才会炸，
// 而那种用例今天一条都没有，于是这个假 mock 可以一直绿着躺在这里。
vi.mock('@/components/ui/Toast', () => ({
  useToast: () => ({ toast: vi.fn(), dismiss: vi.fn() }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 撰写器的校验消息必须和发送按钮待在同一个**固定**区域。
 *
 * 原先它渲染在 `.compose-body` 的最末尾，而那是 `overflow-y: auto` 的滚动区；
 * 发送按钮在固定的 `.compose-foot` 里。写完长正文点发送，错误出现在滚动区中
 * 看不见的地方——按钮表现为「点了没反应」（ui-audit 第 13 条）。
 *
 * 所以这里断言的不是「有没有错误文本」，而是**它挂在哪**。
 */
describe('ComposeDialog 的校验消息位置', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  async function mount() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ComposeDialog open accountId={1} onOpenChange={vi.fn()} />
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  beforeEach(() => {
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, [])
    mock.onPost(/.*/).reply(200, {})
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('消息条常驻在 DOM 里，且不在滚动区内', async () => {
    await mount()
    const msg = container.querySelector('.compose-msg')
    const body = container.querySelector('.compose-body')
    expect(msg, '消息条没有常驻——live region 与内容一起插入 DOM 时读屏不播报').toBeTruthy()
    expect(body).toBeTruthy()
    expect(body?.contains(msg!), '消息条落在 .compose-body 里，写完长正文点发送就看不到它').toBe(
      false,
    )
  })

  it('消息条和发送按钮同属固定区域（两者的公共祖先不是滚动区）', async () => {
    await mount()
    const msg = container.querySelector('.compose-msg')!
    const foot = container.querySelector('.compose-foot')!
    const body = container.querySelector('.compose-body')!
    expect(foot).toBeTruthy()
    // 两者是兄弟：body 结束之后才是消息条与操作栏
    expect(msg.parentElement).toBe(foot.parentElement)
    expect(msg.parentElement).toBe(body.parentElement)
    // 顺序：body → msg → foot
    const kids = [...msg.parentElement!.children]
    expect(kids.indexOf(body)).toBeLessThan(kids.indexOf(msg))
    expect(kids.indexOf(msg)).toBeLessThan(kids.indexOf(foot))
  })

  it('点发送时校验错误落进那个消息条', async () => {
    await mount()
    // 收件人为空 → compose.toRequired
    const send = [...container.querySelectorAll('button')].find(
      (b) => b.textContent === 'compose.send',
    )
    expect(send, '找不到发送按钮').toBeTruthy()
    await act(async () => send!.click())
    await flush()

    const msg = container.querySelector('.compose-msg')
    expect(msg?.textContent).toBe('compose.toRequired')
    expect(msg?.querySelector('.cm-danger'), '错误没有用 danger 语义色').toBeTruthy()
  })

  it('消息条是 live region', async () => {
    await mount()
    const msg = container.querySelector('.compose-msg')
    expect(msg?.getAttribute('role')).toBe('status')
    expect(msg?.getAttribute('aria-live')).toBe('polite')
  })

  it('没有上传在进行时不显示进度条', async () => {
    await mount()
    expect(container.querySelector('.compose-upload')).toBeNull()
  })
})

/**
 * 上传进度的换算。测的是 `queries.ts` 导出的那个函数本身——
 * 此前这里照着实现又写了一遍判据，那样改实现测试也不会红，等于没测。
 */
describe('uploadRatio', () => {
  it('正常情况给出 0~1 的比例', () => {
    expect(uploadRatio({ loaded: 512, total: 1024 })).toBe(0.5)
    expect(uploadRatio({ loaded: 1024, total: 1024 })).toBe(1)
  })

  it('拿不到总长度时返回 null，而不是 NaN', () => {
    // 某些代理不回 Content-Length；空请求体则会除零。
    // 两者都会一路显示成「NaN%」，比没有进度更糟。
    expect(uploadRatio({ loaded: 700 })).toBeNull()
    expect(uploadRatio({ loaded: 900, total: 0 })).toBeNull()
    expect(uploadRatio({ loaded: 900, total: -1 })).toBeNull()
  })
})

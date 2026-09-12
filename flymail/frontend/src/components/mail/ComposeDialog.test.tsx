import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { ComposeDialog, type ComposeInitial } from '@/components/mail/ComposeDialog'
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

  async function mount(initial?: ComposeInitial) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ComposeDialog open accountId={1} onOpenChange={vi.fn()} initial={initial} />
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

/**
 * 上传进度的**正向**路径。
 *
 * 此前这里只断言了「没有上传在进行时不显示进度条」——把
 * `onProgress: setUploadPct` 那一行整个删掉，测试照样全绿。也就是说那条用例
 * 证明的是「不该出现的时候没出现」，而真正会坏的是「该出现的时候没出现」。
 *
 * ⚙ jsdom 里没有真的上传，所以进度事件由 mock adapter 的 reply 回调**主动**
 *    发起：`config.onUploadProgress` 就是 `useSend` 挂上去的那个函数，
 *    调它等于走了一遍真实链路（axios 配置 → uploadRatio → setUploadPct → 渲染）。
 */
describe('ComposeDialog 的上传进度', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  /** 由 reply 回调交出来的进度函数，用例用它模拟上传推进 */
  let emitProgress: ((loaded: number, total: number) => void) | null
  /** 让 /send 挂着不返回，以便在"上传中"这个状态上做断言 */
  let releaseSend: (() => void) | null

  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  beforeEach(() => {
    emitProgress = null
    releaseSend = null
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, [])
    mock.onPost('/send').reply((config) => {
      const onUp = (config as { onUploadProgress?: (e: { loaded: number; total?: number }) => void })
        .onUploadProgress
      emitProgress = (loaded, total) => onUp?.({ loaded, total })
      return new Promise((resolve) => {
        releaseSend = () => resolve([200, { status: 'ok' }])
      })
    })
    mock.onPost(/.*/).reply(200, {})

    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    releaseSend?.()
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  async function mountWithAttachment() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ComposeDialog
            open
            accountId={1}
            onOpenChange={vi.fn()}
            initial={{ to: ['a@example.com'] }}
          />
        </QueryClientProvider>,
      )
    })
    await flush()

    // 附件走 multipart，那是唯一会报进度的一支（纯文本正文一次性发出，没有中间状态）
    const input = container.querySelector<HTMLInputElement>('input[type="file"]')!
    const file = new File(['x'.repeat(64)], 'big.bin', { type: 'application/octet-stream' })
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    await act(async () => input.dispatchEvent(new Event('change', { bubbles: true })))
    await flush()
    return input
  }

  const sendBtn = () =>
    [...container.querySelectorAll('button')].find(
      (b) => b.textContent?.startsWith('compose.send') || b.textContent?.startsWith('compose.sending'),
    )!

  it('上传推进时出现进度条，按钮显示百分比', async () => {
    await mountWithAttachment()
    await act(async () => sendBtn().click())
    await flush()

    expect(emitProgress, '没有走到 multipart 那一支——onUploadProgress 根本没挂上').not.toBeNull()
    await act(async () => emitProgress!(30, 100))
    await flush(1)

    const bar = container.querySelector('.compose-upload-bar')
    expect(bar, '上传推进了却没有进度条').not.toBeNull()
    expect(bar?.getAttribute('aria-valuenow')).toBe('30')
    expect(sendBtn().textContent).toBe('compose.sendingPct')
  })

  it('满格之后按钮换回「发送中…」，而不是停在 100%', async () => {
    // onUploadProgress 量的只是请求体上传，不含服务端把信投出去。局域网里
    // 10MB 附件一两秒就到 100%，后面几十秒的 SMTP 中继里数字一动不动——
    // 停在「发送中… 100%」比停在「发送中…」更误导：它宣称活干完了。
    await mountWithAttachment()
    await act(async () => sendBtn().click())
    await flush()
    await act(async () => emitProgress!(100, 100))
    await flush(1)

    expect(sendBtn().textContent).toBe('compose.sending')
  })

  it('拿不到总长度时不显示进度条，也不显示 NaN%', async () => {
    await mountWithAttachment()
    await act(async () => sendBtn().click())
    await flush()
    await act(async () => emitProgress!(700, 0))
    await flush(1)

    expect(container.querySelector('.compose-upload-bar')).toBeNull()
    expect(sendBtn().textContent).toBe('compose.sending')
  })
})

/**
 * 「附件过大」这条错误的消散路径。
 *
 * 超限的那个附件**不会**被加进列表（onPickFiles 直接 return），所以用户能做的
 * 唯一补救是删掉**已有**的附件腾地方。而 removeAttachment 原先不清消息：
 * 他删完之后错误还挂在那儿，直到再点一次发送或存草稿——而删附件正是他为了
 * 消除这个错误而做的动作。
 */
describe('ComposeDialog 的附件体积错误', () => {
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

  /** 挑一个文件进去；size 可以伪造（jsdom 里造几十 MB 的真文件没必要） */
  async function pick(name: string, size: number) {
    const input = container.querySelector<HTMLInputElement>('input[type="file"]')!
    const file = new File(['x'], name)
    Object.defineProperty(file, 'size', { value: size, configurable: true })
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    await act(async () => input.dispatchEvent(new Event('change', { bubbles: true })))
    await flush(1)
  }

  const msg = () => container.querySelector('.compose-msg')?.textContent ?? ''

  it('删掉已有附件腾出空间后，「附件过大」随之消失', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ComposeDialog open accountId={1} onOpenChange={vi.fn()} />
        </QueryClientProvider>,
      )
    })
    await flush()

    await pick('small.bin', 20 * 1024 * 1024) // 20MB，上限 25MB，收下
    expect(container.querySelectorAll('.attach-chip').length).toBe(1)
    expect(msg()).toBe('')

    await pick('huge.bin', 20 * 1024 * 1024) // 再来 20MB 就超了
    expect(msg(), '超限时没有提示').toContain('compose.attachTooLarge')
    expect(container.querySelectorAll('.attach-chip').length, '超限的附件不该被收下').toBe(1)

    await act(async () => {
      container.querySelector<HTMLButtonElement>('.attach-chip .ac-remove')?.click()
    })
    await flush(1)
    expect(container.querySelectorAll('.attach-chip').length).toBe(0)
    expect(msg(), '附件删了但「附件过大」还挂着').toBe('')
  })
})

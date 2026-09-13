import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { MessageBody } from '@/components/mail/MessageBody'
import { ConfirmProvider } from '@/components/ui/Confirm'
import { ToastProvider } from '@/components/ui/Toast'
import type { Attachment, MessageDetail } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join(',')}` : k,
  }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function att(i: number, size = 1024): Attachment {
  return {
    filename: `file-${i}.pdf`,
    content_type: 'application/pdf',
    size,
    is_inline: false,
  } as Attachment
}

function detailWith(attachments: Attachment[]): MessageDetail {
  return {
    id: 7,
    account_id: 1,
    folder_id: 1,
    uid: 7,
    subject: '带附件的邮件',
    from_name: '张三',
    from_addr: 'z@example.com',
    to: [],
    date: new Date('2026-09-10T08:00:00Z').toISOString(),
    size: 2048,
    seen: true,
    flagged: false,
    has_attachment: true,
    snippet: '',
    text_body: '正文',
    html_body: '',
    attachments,
    body_synced: true,
    remote_count: 0,
    remote_allowed: true,
  } as MessageDetail
}

/**
 * 附件区：折叠与下载。
 *
 * 两件事都属于「点了之后什么都没发生」那一类——
 * 下载失败原先是 `void downloadAttachment(...)`，异常被整个吞掉；
 * 而二十个附件无条件全量渲染，会把邮件正文顶到屏幕外。
 */
describe('MessageBody 的附件区', () => {
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

  async function mount(attachments: Attachment[]) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <ConfirmProvider>
              <MessageBody detail={detailWith(attachments)} />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  const cards = () => container.querySelectorAll('.attach-card')
  const moreBtn = () => container.querySelector<HTMLButtonElement>('.attach-more')

  /** 在确认框上作答（radix Portal 挂在 body，从 container 里查不到） */
  async function answerConfirm(ok: boolean) {
    const dialog = document.querySelector('.confirm-dialog')
    expect(dialog, '确认框没有出现').not.toBeNull()
    const btns = dialog!.querySelectorAll<HTMLButtonElement>('.confirm-actions button')
    await act(async () => btns[ok ? 1 : 0].click())
    await flush()
  }

  /**
   * 注册附件下载的响应。
   *
   * ⚠ 必须在通配之前注册：axios-mock-adapter 按**注册顺序**匹配，
   *   先放通配的话具体 handler 永远命中不到——而那种错法的表现是
   *   「测试照样绿」（下载被通配拦下，返回 200 {}），不是报错。
   */
  function stubDownload(reply: () => [number, unknown]) {
    mock.resetHandlers()
    mock.onGet(/\/messages\/7\/attachments\/0/).reply(reply)
    mock.onGet(/.*/).reply(200, {})
  }

  beforeEach(() => {
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, {})
    // jsdom 没有 createObjectURL，也不实现 <a download> 的点击导航。
    // 不打桩的话 downloadAttachment 在**成功**路径上也会抛，
    // 于是「下载失败会提示」那条用例会因为一个与被测逻辑无关的原因而通过。
    URL.createObjectURL = vi.fn(() => 'blob:stub')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
    vi.restoreAllMocks()
  })

  it('附件不多时全量显示，没有折叠开关', async () => {
    await mount([att(1), att(2), att(3)])
    expect(cards().length).toBe(3)
    expect(moreBtn(), '三个附件也给了折叠开关').toBeNull()
  })

  it('附件很多时先折叠，点开才全显示', async () => {
    // 群发的会议纪要动辄带一二十个附件，无条件全量渲染会把正文顶到屏幕外——
    // 用户得滚过一整屏卡片才看得到邮件本身。
    await mount(Array.from({ length: 20 }, (_, i) => att(i)))
    expect(cards().length, '没有折叠').toBe(6)

    const btn = moreBtn()
    expect(btn?.getAttribute('aria-expanded')).toBe('false')
    expect(btn?.textContent).toContain('20')

    await act(async () => btn!.click())
    expect(cards().length).toBe(20)
    expect(moreBtn()?.getAttribute('aria-expanded')).toBe('true')

    // 还能收回去
    await act(async () => moreBtn()!.click())
    expect(cards().length).toBe(6)
  })

  it('文件名带 title，截断之后还看得到全名', async () => {
    await mount([att(1)])
    expect(container.querySelector('.ac-name')?.getAttribute('title')).toBe('file-1.pdf')
  })

  it('下载失败时给出提示，而不是一声不吭', async () => {
    // 原先是 `void downloadAttachment(...)`：令牌过期、附件已被服务端清理、
    // 网络断，界面上都毫无反应，与「点了没生效」无法区分。
    //
    // ⚙ 这里能读到后端那句话，是因为 mock adapter 把响应体原样交出；真实浏览器里
    //   附件请求是 responseType:'blob'，失败响应体也会是 Blob，那时读不出 error
    //   字段、显示的是兜底文案。两种都算「有提示」，这条守的是**有没有提示**。
    stubDownload(() => [500, { error: '附件已过期' }])
    await mount([att(1)])

    await act(async () => container.querySelector<HTMLElement>('.attach-card')?.click())
    await flush()

    expect(document.body.textContent).toContain('附件已过期')
  })

  it('下载成功时不弹任何提示', async () => {
    // 上一条若因为别的原因（比如 jsdom 缺 createObjectURL）而抛，
    // 它也会"通过"。这条是它的对照：成功路径必须干干净净。
    stubDownload(() => [200, new Blob(['x'])])
    await mount([att(1)])

    await act(async () => container.querySelector<HTMLElement>('.attach-card')?.click())
    await flush()

    expect(document.body.textContent).not.toContain('reader.downloadFailed')
    expect(container.querySelector('[aria-busy="true"]'), '下载完了还停在忙碌态').toBeNull()
  })

  it('大附件先问一句；答取消就不发请求', async () => {
    // 下载没有进度表达（blob 一次性拿完才落盘），200MB 在慢网络上就是
    // 「点了之后十几分钟毫无动静」。
    let hits = 0
    stubDownload(() => {
      hits++
      return [200, new Blob(['x'])]
    })
    await mount([att(1, 200 * 1024 * 1024)])

    await act(async () => container.querySelector<HTMLElement>('.attach-card')?.click())
    await flush()
    await answerConfirm(false)
    expect(hits, '取消了还是发了请求').toBe(0)
  })

  it('等确认框期间点了别的附件，两次下载不会互相清掉忙碌态', async () => {
    // 交错路径：A（>50MB）弹出确认框时闸门还开着 → 用户点小附件 B，B 不走确认框
    // 直接开始下载 → 用户再回答 A 的「是」。原先 A 会把 B 顶掉，
    // 而 B 下完之后的 finally 又把忙碌态清成 null——A 还在下载却显示文件大小、
    // aria-busy 提前消失，读屏不再报忙碌。
    // 放在对象里而不是裸变量：赋值发生在回调内，TS 的控制流分析会把裸变量
    // 在使用处收窄成 null，`release?.()` 报「表达式不可调用」。
    const pending: { release: ((v: [number, unknown]) => void) | null } = { release: null }
    mock.resetHandlers()
    mock.onGet(/\/messages\/7\/attachments\/1/).reply(
      () => new Promise((r) => { pending.release = r }),
    )
    mock.onGet(/.*/).reply(200, {})

    await mount([att(1, 200 * 1024 * 1024), att(2, 1024)])
    const [cardA, cardB] = [...container.querySelectorAll<HTMLElement>('.attach-card')]

    // A：弹确认框，先不回答
    await act(async () => cardA.click())
    await flush()
    expect(document.querySelector('.confirm-dialog')).not.toBeNull()

    // B：小附件，直接开下（挂着不返回）
    await act(async () => cardB.click())
    await flush()
    expect(cardB.getAttribute('aria-busy'), 'B 没有进入忙碌态').toBe('true')

    // 回答 A 的确认框：此时已有下载在进行，A 应当让位而不是顶掉 B
    await act(async () => {
      document
        .querySelectorAll<HTMLButtonElement>('.confirm-dialog .confirm-actions button')[1]
        .click()
    })
    await flush()
    expect(cardB.getAttribute('aria-busy'), 'B 的忙碌态被 A 顶掉了').toBe('true')

    pending.release?.([200, new Blob(['x'])])
    await flush()
  })

  it('小附件不问，直接下载', async () => {
    let hits = 0
    stubDownload(() => {
      hits++
      return [200, new Blob(['x'])]
    })
    await mount([att(1, 1024)])

    await act(async () => container.querySelector<HTMLElement>('.attach-card')?.click())
    await flush()
    expect(document.querySelector('.confirm-dialog'), '小附件也弹了确认框').toBeNull()
    expect(hits).toBe(1)
  })
})

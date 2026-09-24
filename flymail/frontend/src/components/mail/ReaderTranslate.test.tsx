// 阅读区的翻译开关：什么时候花钱、什么时候只是切个视图、失败了怎么说。
//
// 这几条都属于"看起来能用、实际上不对"的那一类：
//   - 已有译文还去调一次 AI —— 界面上完全看不出来，只体现在账单上；
//   - 切换邮件不重置状态 —— 下一封显示着原文，按钮却写着「显示原文」；
//   - 失败只弹一下 toast —— 用户错过了就再也不知道刚才发生了什么。

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { Reader } from '@/components/mail/Reader'
import { ConfirmProvider } from '@/components/ui/Confirm'
import { ToastProvider } from '@/components/ui/Toast'
import { onOpenSettingsRequest } from '@/lib/settings-nav'
import type { MessageDetail, Translation } from '@/lib/types'

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

function detail(id = 7): MessageDetail {
  return {
    id,
    account_id: 1,
    folder_id: 1,
    uid: id,
    subject: 'Invoice ready',
    from_name: 'Acme',
    from_addr: 'billing@acme.com',
    to: [],
    date: new Date('2026-09-10T08:00:00Z').toISOString(),
    size: 2048,
    seen: true,
    flagged: false,
    has_attachment: false,
    snippet: '',
    // 纯文本正文：译文能直接断言在 DOM 上。HTML 正文走沙箱 iframe，
    // 内容在 srcdoc 里，断言它等于在断言 iframe 的实现细节。
    text_body: 'Hello, your invoice is ready.',
    html_body: '',
    attachments: [],
    body_synced: true,
    remote_count: 0,
    remote_allowed: true,
    detect_lang: 'en',
  } as MessageDetail
}

function translation(messageId = 7): Translation {
  return {
    message_id: messageId,
    target_lang: 'zh',
    source_lang: 'en',
    subject: '发票已开好',
    text_body: '您好，您的发票已经开好了。',
    html_body: '',
    partial: false,
    model: 'gpt-4o-mini',
    provider: 'OpenAI',
    cached: false,
    remote_count: 0,
    remote_allowed: true,
    created_at: new Date().toISOString(),
  }
}

describe('阅读区的翻译开关', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let posts: number

  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  /**
   * 装好各路响应。
   *
   * ⚠ 具体的 handler 必须在通配之前注册：axios-mock-adapter 按注册顺序匹配，
   * 顺序反了的话具体 handler 永远命中不到，而表现是"测试照样绿"。
   */
  function stub(opts: {
    enabled?: boolean
    cached?: Translation | null
    postStatus?: number
    postBody?: unknown
  }) {
    mock.resetHandlers()
    posts = 0
    mock.onGet('/translate/languages').reply(200, {
      languages: [
        { code: 'zh', name: 'Simplified Chinese', native: '简体中文' },
        { code: 'en', name: 'English', native: 'English' },
      ],
      default_target: 'zh',
      enabled: opts.enabled ?? true,
    })
    mock.onGet(/\/messages\/\d+\/translation/).reply(() =>
      opts.cached ? [200, opts.cached] : [204, ''],
    )
    mock.onPost(/\/messages\/\d+\/translate/).reply(() => {
      posts++
      return [opts.postStatus ?? 200, opts.postBody ?? translation()]
    })
    mock.onGet(/\/messages\/\d+$/).reply(200, detail())
    mock.onGet(/.*/).reply(200, {})
  }

  async function mount(messageId: number | null = 7) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <ConfirmProvider>
              <Reader messageId={messageId} onDelete={() => {}} onArchive={null} onMove={() => {}} />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await flush()
    return qc
  }

  /** 工具栏上的翻译按钮（唯一一颗带 aria-pressed 的） */
  const translateBtn = () => container.querySelector<HTMLButtonElement>('button[aria-pressed]')
  const bar = () => container.querySelector('.translated-bar')
  const bodyText = () => container.querySelector('.thread-body')?.textContent ?? ''

  beforeEach(() => {
    mock = new MockAdapter(api)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
    vi.clearAllMocks()
  })

  it('点一下翻译，正文换成译文，按钮变成「显示原文」', async () => {
    stub({})
    await mount()

    const btn = translateBtn()
    expect(btn, '工具栏上应当有翻译按钮').not.toBeNull()
    expect(btn!.disabled).toBe(false)
    expect(bodyText()).toContain('Hello, your invoice is ready.')

    await act(async () => btn!.click())
    await flush()

    expect(posts, '第一次翻译要真的调一次接口').toBe(1)
    expect(bodyText()).toContain('您好，您的发票已经开好了。')
    expect(translateBtn()!.textContent).toContain('reader.showOriginal')
    expect(translateBtn()!.getAttribute('aria-pressed')).toBe('true')
    // 提示条要说清楚是从哪门语言、用哪个模型翻的
    expect(bar()?.textContent).toContain('reader.translatedFrom')
  })

  it('再点一下回到原文，且不会再花一次钱', async () => {
    stub({})
    await mount()

    await act(async () => translateBtn()!.click())
    await flush()
    await act(async () => translateBtn()!.click())
    await flush()

    expect(bodyText()).toContain('Hello, your invoice is ready.')
    expect(bodyText()).not.toContain('您好，您的发票已经开好了。')
    expect(posts, '来回切视图是本地动作').toBe(1)
  })

  it('已有译文缓存时，点翻译只是切视图，不再调用 AI', async () => {
    stub({ cached: { ...translation(), cached: true } })
    await mount()

    await act(async () => translateBtn()!.click())
    await flush()

    expect(bodyText()).toContain('您好，您的发票已经开好了。')
    expect(posts, '手里已经有译文还去调一次 AI，只体现在账单上').toBe(0)
  })

  it('翻译失败时在正文上方留下可重试的提示，而不是一闪而过的 toast', async () => {
    stub({ postStatus: 502, postBody: { error: '上游没给出可用的译文' } })
    await mount()

    await act(async () => translateBtn()!.click())
    await flush()

    const b = bar()
    expect(b, '失败必须留在界面上').not.toBeNull()
    expect(b!.className).toContain('is-error')
    expect(b!.textContent).toContain('上游没给出可用的译文')
    // 原文不能因为翻译失败就消失
    expect(bodyText()).toContain('Hello, your invoice is ready.')

    const retry = b!.querySelector<HTMLButtonElement>('button')
    expect(retry?.textContent).toContain('reader.translateRetry')
    await act(async () => retry!.click())
    await flush()
    expect(posts, '重试要真的再发一次').toBe(2)
  })

  it('AI 没配置时按钮仍可点，点了直接打开设置的 AI 页，不发翻译请求', async () => {
    stub({ enabled: false })
    await mount()
    const opened: string[] = []
    const off = onOpenSettingsRequest((page) => opened.push(page))

    const btn = translateBtn()
    expect(btn!.disabled).toBe(false)
    expect(btn!.title).toContain('reader.translateNotConfigured')
    await act(async () => btn!.click())
    await flush()
    off()

    expect(opened).toEqual(['ai'])
    expect(posts, '没配置时不该去请求翻译').toBe(0)
  })

  it('这封信已经是目标语言时给出提示，但按钮照样能点', async () => {
    stub({})
    mock.onGet(/\/messages\/\d+$/).reply(200, { ...detail(), detect_lang: 'zh' })
    // ⚠ 通配要重新排到最后（resetHandlers 之后的注册顺序决定匹配顺序）
    mock.onGet(/.*/).reply(200, {})
    await mount()

    const btn = translateBtn()
    expect(btn!.title).toContain('reader.translateSameLang')
    expect(btn!.disabled, '识别有可能出错，猜错时不该把用户挡在外面').toBe(false)
  })

  it('换一封邮件回到原文视图', async () => {
    stub({})
    const qc = await mount(7)

    await act(async () => translateBtn()!.click())
    await flush()
    expect(bodyText()).toContain('您好，您的发票已经开好了。')

    // 换成第 8 封（没有译文）
    mock.onGet(/\/messages\/8$/).reply(200, detail(8))
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <ConfirmProvider>
              <Reader messageId={8} onDelete={() => {}} onArchive={null} onMove={() => {}} />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await flush()

    expect(translateBtn()!.textContent, '新邮件应当回到「翻译」而不是「显示原文」')
      .toContain('reader.translate')
    expect(bar(), '新邮件不该带着上一封的译文提示条').toBeNull()
  })
})

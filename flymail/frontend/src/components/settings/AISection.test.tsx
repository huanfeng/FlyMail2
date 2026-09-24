// AI 翻译设置：多条配置的列表、排序、状态，以及对话框里密钥的"留空 = 不改动"语义。
//
// 密钥那条是这里最要紧的：密文永不回显，界面上能做的只有覆盖与清除。
// 把"没填"当成"清空"提交的话，用户改一下模型名就会把密钥连带抹掉——
// 而要等到下次点翻译时才会发现。

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AISection } from '@/components/settings/AISection'
import type { AIProvider } from '@/lib/types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join(',')}` : k,
    i18n: { language: 'zh' },
  }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

const toast = vi.fn()
const confirmFn = vi.fn(async () => true)
vi.mock('@/components/ui/Toast', () => ({ useToast: () => ({ toast }) }))
vi.mock('@/components/ui/Confirm', () => ({ useConfirm: () => confirmFn }))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function provider(over: Partial<AIProvider>): AIProvider {
  return {
    id: 1,
    name: 'P',
    base_url: 'https://api.openai.com/v1/chat/completions',
    model: 'gpt-4o-mini',
    enabled: true,
    key_set: true,
    status: { failures: 0 },
    cooling: false,
    created_at: '',
    updated_at: '',
    ...over,
  }
}

describe('AI 翻译设置', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let requests: { method: string; url: string; body: unknown }[]

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  /**
   * @param before 在通配处理器之前注册的专用处理器——axios-mock-adapter 按注册顺序匹配，
   *               后注册的会被前面的通配 /.* / 吃掉。
   */
  function stub(providers: AIProvider[], settings: Record<string, string> = {}, before?: (m: MockAdapter) => void) {
    mock.resetHandlers()
    requests = []
    before?.(mock)
    const record = (method: string) => (cfg: { url?: string; data?: string }) => {
      requests.push({ method, url: cfg.url ?? '', body: cfg.data ? JSON.parse(cfg.data) : null })
      return [200, method === 'post' && cfg.url?.endsWith('/test')
        ? { ok: true, latency_ms: 42, provider: providers[0] }
        : { status: 'ok' }] as [number, unknown]
    }
    mock.onGet('/ai/providers').reply(200, { providers })
    mock.onGet('/settings').reply(200, { settings })
    mock.onGet('/translate/languages').reply(200, {
      languages: [
        { code: 'zh', name: 'Simplified Chinese', native: '简体中文' },
        { code: 'ja', name: 'Japanese', native: '日本語' },
      ],
      default_target: 'zh',
      enabled: providers.some((p) => p.enabled),
    })
    mock.onPost(/.*/).reply(record('post'))
    mock.onPut(/.*/).reply(record('put'))
    mock.onDelete(/.*/).reply(record('delete'))
  }

  async function mount() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <AISection />
        </QueryClientProvider>,
      )
    })
    await flush()
  }

  const q = <T extends Element = HTMLElement>(sel: string) => document.querySelector<T>(sel)
  const buttonByText = (text: string) =>
    [...document.querySelectorAll('button')].find((b) => b.textContent?.includes(text))
  const buttonByLabel = (scope: ParentNode, label: string) =>
    scope.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)

  async function click(el: Element | null | undefined) {
    expect(el, '找不到要点的元素').toBeTruthy()
    await act(async () => (el as HTMLElement).click())
    await flush()
  }

  /** React 受控输入：必须走原生 setter 派发 input 事件，直接改 value 收不到 */
  async function type(el: HTMLInputElement | null, value: string) {
    expect(el).toBeTruthy()
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(el, value)
      el!.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

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

  it('没有配置时说明翻译为何不可用', async () => {
    stub([])
    await mount()
    expect(container.textContent).toContain('settings.ai.statusEmpty')
  })

  it('全部停用时给出另一句提示', async () => {
    stub([provider({ enabled: false })])
    await mount()
    expect(container.textContent).toContain('settings.ai.allDisabled')
  })

  it('按顺序列出配置，并显示模型、主机与状态', async () => {
    stub([
      provider({ id: 1, name: 'DeepSeek', model: 'deepseek-chat', base_url: 'https://api.deepseek.com/v1/chat/completions',
        cooling: true, status: { failures: 1, last_kind: 'quota', cooldown_until: new Date(Date.now() + 3600e3).toISOString() } }),
      provider({ id: 2, name: 'Ollama', model: 'qwen2.5:7b', base_url: 'http://127.0.0.1:11434/v1/chat/completions', key_set: false }),
    ])
    await mount()
    const cards = container.querySelectorAll('[data-testid^="ai-provider-"]')
    expect(cards).toHaveLength(2)
    expect(cards[0].textContent).toContain('DeepSeek')
    expect(cards[0].textContent).toContain('api.deepseek.com')
    expect(cards[0].textContent).toContain('settings.ai.stateCooling')
    expect(cards[0].textContent).toContain('settings.ai.kind_quota')
    // 冷却中的才有「解除暂停」
    expect(cards[0].textContent).toContain('settings.ai.reset')
    expect(cards[1].textContent).not.toContain('settings.ai.reset')
    expect(cards[1].textContent).toContain('settings.ai.noKey')
  })

  it('下移提交完整的新顺序；首行上移置灰', async () => {
    stub([provider({ id: 1 }), provider({ id: 2 }), provider({ id: 3 })])
    await mount()
    const first = container.querySelector('[data-testid="ai-provider-1"]')!
    expect(buttonByLabel(first, 'settings.ai.moveUp')!.disabled).toBe(true)
    await click(buttonByLabel(first, 'settings.ai.moveDown'))
    const put = requests.find((r) => r.url === '/ai/providers/order')
    expect(put?.body).toEqual({ ids: [2, 1, 3] })
  })

  it('开关只提交 enabled', async () => {
    stub([provider({ id: 5 })])
    await mount()
    await click(container.querySelector('[role="switch"]'))
    const put = requests.find((r) => r.url === '/ai/providers/5')
    expect(put?.body).toEqual({ enabled: false })
  })

  it('测试连接后报告耗时', async () => {
    stub([provider({ id: 1, name: 'A' })])
    await mount()
    await click(buttonByLabel(container, 'settings.ai.test'))
    expect(requests.some((r) => r.method === 'post' && r.url === '/ai/providers/1/test')).toBe(true)
    expect(toast).toHaveBeenCalledWith('settings.ai.testOk:A,42')
  })

  it('解除暂停', async () => {
    stub([provider({ id: 1, cooling: true, status: { failures: 1, last_kind: 'rate_limit', cooldown_until: new Date(Date.now() + 60e3).toISOString() } })])
    await mount()
    await click(buttonByText('settings.ai.reset'))
    expect(requests.some((r) => r.url === '/ai/providers/1/reset')).toBe(true)
  })

  it('解除暂停后不再显示成「上次失败」', async () => {
    stub([provider({ id: 1, status: { failures: 0, last_kind: 'quota', last_fail_at: new Date().toISOString() } })])
    await mount()
    const card = container.querySelector('[data-testid="ai-provider-1"]')!
    expect(card.textContent).toContain('settings.ai.stateResumed')
    expect(card.textContent).not.toContain('settings.ai.stateFailed')
  })

  it('开关失败时提示原因', async () => {
    stub([provider({ id: 5 })], {}, (m) => {
      m.onPut('/ai/providers/5').reply(500, { error: '未配置加密器，无法保存密钥' })
    })
    await mount()
    await click(container.querySelector('[role="switch"]'))
    expect(toast).toHaveBeenCalledWith('未配置加密器，无法保存密钥')
  })

  it('删除前先确认', async () => {
    stub([provider({ id: 3 })])
    await mount()
    await click(buttonByLabel(container, 'settings.ai.delete'))
    expect(confirmFn).toHaveBeenCalled()
    expect(requests.some((r) => r.method === 'delete' && r.url === '/ai/providers/3')).toBe(true)
  })

  it('选择目标语言即保存', async () => {
    stub([], { translate_target_lang: 'zh' })
    await mount()
    const sel = q<HTMLSelectElement>('#ai-target-lang')!
    await act(async () => {
      sel.value = 'ja'
      sel.dispatchEvent(new Event('change', { bubbles: true }))
    })
    await flush()
    const put = requests.find((r) => r.url === '/settings')
    expect(put?.body).toEqual({ settings: { translate_target_lang: 'ja' } })
  })

  describe('编辑对话框', () => {
    it('新建：填地址与模型后提交；没填密钥就不带 api_key', async () => {
      stub([])
      await mount()
      await click(buttonByText('settings.ai.add'))
      await type(q('#ai-base-url'), 'https://api.deepseek.com/v1')
      await type(q('#ai-model'), 'deepseek-chat')
      await click(buttonByText('settings.ai.save'))
      const post = requests.find((r) => r.method === 'post' && r.url === '/ai/providers')
      expect(post?.body).toEqual({ name: '', base_url: 'https://api.deepseek.com/v1', model: 'deepseek-chat' })
    })

    it('缺地址或模型时不提交', async () => {
      stub([])
      await mount()
      await click(buttonByText('settings.ai.add'))
      await click(buttonByText('settings.ai.save'))
      expect(requests.filter((r) => r.method === 'post')).toHaveLength(0)
      expect(document.body.textContent).toContain('settings.ai.invalid')
    })

    it('编辑时密钥框为空、占位写「已保存」；只改模型不会抹掉密钥', async () => {
      stub([provider({ id: 4, key_set: true })])
      await mount()
      await click(buttonByLabel(container, 'settings.ai.edit'))
      const key = q<HTMLInputElement>('#ai-api-key')!
      expect(key.value).toBe('')
      expect(key.placeholder).toContain('settings.ai.keySaved')
      await type(q('#ai-model'), 'gpt-4.1-mini')
      await click(buttonByText('settings.ai.save'))
      const put = requests.find((r) => r.url === '/ai/providers/4')
      expect(put?.body).toMatchObject({ model: 'gpt-4.1-mini' })
      expect(put?.body).not.toHaveProperty('api_key')
      expect(put?.body).not.toHaveProperty('clear_key')
    })

    it('勾选清除才提交 clear_key', async () => {
      stub([provider({ id: 4, key_set: true })])
      await mount()
      await click(buttonByLabel(container, 'settings.ai.edit'))
      const box = [...document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')][0]
      await click(box)
      await click(buttonByText('settings.ai.save'))
      const put = requests.find((r) => r.url === '/ai/providers/4')
      expect(put?.body).toMatchObject({ clear_key: true })
      expect(put?.body).not.toHaveProperty('api_key')
    })

    it('后端拒绝理由原样显示', async () => {
      stub([], {}, (m) => {
        m.onPost('/ai/providers').reply(400, { error: 'AI 接口地址必须以 http:// 或 https:// 开头' })
      })
      await mount()
      await click(buttonByText('settings.ai.add'))
      await type(q('#ai-base-url'), 'api.x.com')
      await type(q('#ai-model'), 'm')
      await click(buttonByText('settings.ai.save'))
      expect(document.body.textContent).toContain('AI 接口地址必须以 http:// 或 https:// 开头')
    })
  })
})

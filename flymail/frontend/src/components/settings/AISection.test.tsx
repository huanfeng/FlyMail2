// AI 翻译设置：密钥的"留空 = 不改动"语义，以及语言清单的来源。
//
// 密钥那条是这里最要紧的：密文永不回显，界面上能做的只有覆盖与清除。
// 把"没填"当成"清空"提交的话，用户改一下模型名就会把密钥连带抹掉——
// 而他要等到下次点翻译时才会发现。

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AISection } from '@/components/settings/AISection'

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

describe('AI 翻译设置', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let saved: Record<string, string> | null

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  function stub(settings: Record<string, string>) {
    mock.resetHandlers()
    saved = null
    mock.onGet('/settings').reply(200, { settings })
    mock.onGet('/translate/languages').reply(200, {
      languages: [
        { code: 'zh', name: 'Simplified Chinese', native: '简体中文' },
        { code: 'ja', name: 'Japanese', native: '日本語' },
      ],
      default_target: 'zh',
      enabled: true,
    })
    mock.onPut('/settings').reply((cfg) => {
      saved = JSON.parse(cfg.data as string).settings as Record<string, string>
      return [200, { status: 'ok' }]
    })
    mock.onGet(/.*/).reply(200, {})
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

  const field = (id: string) => container.querySelector<HTMLInputElement>(`#${id}`)!
  const select = () => container.querySelector<HTMLSelectElement>('#ai-target-lang')!

  /** React 受控输入：必须走原生 setter 派发 input 事件，直接改 value 收不到 */
  async function type(el: HTMLInputElement, value: string) {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(el, value)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  async function clickSave() {
    const btns = [...container.querySelectorAll('button')]
    const save = btns.find((b) => b.textContent?.includes('settings.ai.save'))
    expect(save, '找不到保存按钮').toBeTruthy()
    await act(async () => save!.click())
    await flush()
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

  it('回显已保存的配置，密钥只显示「已保存」', async () => {
    stub({
      ai_base_url: 'https://api.openai.com/v1/chat/completions',
      ai_model: 'gpt-4o-mini',
      ai_api_key_set: 'true',
      translate_target_lang: 'ja',
    })
    await mount()

    expect(field('ai-base-url').value).toBe('https://api.openai.com/v1/chat/completions')
    expect(field('ai-model').value).toBe('gpt-4o-mini')
    expect(select().value).toBe('ja')
    // 密钥框永远是空的：后端从不回显密文，能做的只有覆盖
    expect(field('ai-api-key').value).toBe('')
    expect(field('ai-api-key').placeholder).toContain('settings.ai.keySaved')
  })

  it('不填密钥点保存，不会把已存的那份抹掉', async () => {
    stub({ ai_base_url: 'https://x/v1', ai_model: 'm', ai_api_key_set: 'true' })
    await mount()

    await type(field('ai-model'), 'gpt-4o-mini')
    await clickSave()

    expect(saved).not.toBeNull()
    expect(saved!.ai_model).toBe('gpt-4o-mini')
    expect('ai_api_key' in saved!, '没填就不该提交这个键').toBe(false)
  })

  it('填了密钥才提交它', async () => {
    stub({})
    await mount()

    await type(field('ai-base-url'), 'https://api.deepseek.com/v1')
    await type(field('ai-model'), 'deepseek-chat')
    await type(field('ai-api-key'), 'sk-abc')
    await clickSave()

    expect(saved!.ai_api_key).toBe('sk-abc')
    expect(saved!.ai_base_url).toBe('https://api.deepseek.com/v1')
  })

  it('密钥已保存时才出现清除按钮，点它提交空串', async () => {
    stub({ ai_api_key_set: 'true' })
    await mount()

    const clear = [...container.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('settings.ai.clearKey'))
    expect(clear, '已保存密钥时应当给出清除入口').toBeTruthy()

    await act(async () => clear!.click())
    await flush()
    // 空串是后端约定的"清除"，与"没填"（整个键不提交）是两回事
    expect(saved!.ai_api_key).toBe('')
  })

  it('没保存过密钥时不显示清除按钮', async () => {
    stub({})
    await mount()
    const clear = [...container.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('settings.ai.clearKey'))
    expect(clear).toBeUndefined()
  })

  it('语言清单来自后端，显示自称名', async () => {
    stub({})
    await mount()

    const opts = [...select().options].map((o) => `${o.value}:${o.textContent}`)
    expect(opts).toEqual(['zh:简体中文', 'ja:日本語'])
  })

  it('预设按钮把地址填进输入框', async () => {
    stub({})
    await mount()

    const preset = [...container.querySelectorAll('button')].find((b) => b.textContent === 'Ollama')
    expect(preset, '应当给出常见服务的地址预设').toBeTruthy()
    await act(async () => preset!.click())
    expect(field('ai-base-url').value).toBe('http://127.0.0.1:11434/v1')
  })

  it('没配置时把「未配置」说出来，而不是只留一排空框', async () => {
    stub({})
    await mount()
    expect(container.textContent).toContain('settings.ai.statusEmpty')
  })
})

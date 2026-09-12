import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { ConfirmProvider, useConfirm } from '@/components/ui/Confirm'
import { ToastProvider } from '@/components/ui/Toast'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh', changeLanguage: vi.fn() } }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 设置面板与它内部弹出的 radix 浮层之间的 Esc 归属。
 *
 * 面板里有六处删除确认，另有账户 / 规则 / 通知渠道三个对话框，全都是 radix 的
 * Portal 层。radix 处理完 Esc 只调 `preventDefault()`，**不**调 `stopPropagation()`，
 * 事件照常冒泡到 document——面板那个「按 Esc 就 onClose」的监听器于是跟着触发。
 * 用户想取消一次删除，代价是整个设置面板消失，得重新打开、重新找到那个分区。
 *
 * 原生 window.confirm 阻塞 JS 线程、按键根本到不了页面，所以这件事在删除这条
 * 路径上此前是被盖住的；但那三个对话框早就是 radix 的，它们一直都有这个毛病。
 *
 * 这里让确认框由一个兄弟组件弹出，而不是去驱动面板里某个具体的删除按钮：
 * 要验的是**面板那个 document 级监听器**的行为，跟确认框是谁弹的无关，
 * 绕开那一长串分区导航和列表数据只会让用例更稳。
 */
describe('SettingsDialog 的 Esc 归属', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root
  let onClose: Mock<() => void>

  function Ask() {
    const confirm = useConfirm()
    return (
      <button type="button" id="ask" onClick={() => void confirm({ title: '删掉它？' })}>
        ask
      </button>
    )
  }

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  async function pressEscape() {
    await act(async () => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
      )
    })
    await flush(1)
  }

  beforeEach(async () => {
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, {})
    mock.onPost(/.*/).reply(200, {})
    onClose = vi.fn<() => void>()

    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <ConfirmProvider>
              <SettingsDialog
                listStyle="compact"
                onChangeListStyle={vi.fn()}
                conversationView={false}
                onChangeConversationView={vi.fn()}
                alwaysShowSelect={false}
                onChangeAlwaysShowSelect={vi.fn()}
                layoutMode="three"
                onChangeLayoutMode={vi.fn()}
                onClose={onClose}
              />
              <Ask />
            </ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
    await flush()
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    mock.restore()
  })

  it('没有别的浮层时 Esc 关闭设置面板', async () => {
    await pressEscape()
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('确认框开着时 Esc 只关确认框，设置面板留着', async () => {
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
    expect(document.querySelector('.confirm-dialog'), '前提：确认框已弹出').not.toBeNull()

    await pressEscape()

    expect(document.querySelector('.confirm-dialog'), '确认框没关掉').toBeNull()
    expect(onClose, '取消一次删除把整个设置面板也关掉了').not.toHaveBeenCalled()
  })

  it('确认框关掉之后，Esc 重新归设置面板', async () => {
    await act(async () => container.querySelector<HTMLButtonElement>('#ask')?.click())
    await pressEscape()
    expect(onClose).not.toHaveBeenCalled()

    await pressEscape()
    expect(onClose, '确认框关了之后 Esc 不管用了').toHaveBeenCalledTimes(1)
  })
})

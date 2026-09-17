import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MailList } from '@/components/mail/MailList'
import { ToastProvider } from '@/components/ui/Toast'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EMPTY_FILTER } from '@/lib/list-filters'
import type { MessageListItem } from '@/lib/types'

// i18n 换成「原样返回 key」：这里验的是三种数据状态各自有没有出口，文案由 locales.test.ts 兜住。
vi.mock('react-i18next', () => ({
  // MailList 还要读 i18n.language（日期分组标签按语言切换）
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh' } }),
}))

// 记下 scrollToIndex 要求滚到的下标。
//
// jsdom 没有真实布局，验不了「滚到没滚到」（scrollTo 的参数恒为 0）。但本次要验的
// 就是**有没有按正确的下标提出滚动请求**——真正的滚动交给浏览器，实机另有验证。
// 只包一次：virtualizer 实例每渲染都是同一个，重复包会一层层套下去。
const scrollIndexCalls: number[] = []
const patchedVirtualizers = new WeakSet<object>()
vi.mock('@tanstack/react-virtual', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@tanstack/react-virtual')>()
  return {
    ...mod,
    useVirtualizer: ((opts: never) => {
      const v = mod.useVirtualizer(opts)
      if (!patchedVirtualizers.has(v)) {
        patchedVirtualizers.add(v)
        const orig = v.scrollToIndex
        v.scrollToIndex = ((index: number, o?: never) => {
          scrollIndexCalls.push(index)
          return orig(index, o)
        }) as typeof v.scrollToIndex
      }
      return v
    }) as typeof mod.useVirtualizer,
  }
})

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

// jsdom 没实现 Element.scrollTo（切数据源时重置滚动会调它）
Element.prototype.scrollTo = () => {}

// jsdom 里 offsetWidth / offsetHeight 恒为 0，而 @tanstack/virtual 正是用它们量
// 滚动容器（不是 getBoundingClientRect）——不给尺寸就一行都不渲染，
// 下面关于列表语义的断言会全部落空成「没有行所以没问题」。
Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, get: () => 600 })
Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, get: () => 400 })

function msg(id: number): MessageListItem {
  return {
    id,
    account_id: 1,
    folder_id: 1,
    uid: id,
    subject: `邮件 ${id}`,
    from_name: '张三',
    from_addr: 'z@example.com',
    to: [],
    date: new Date('2026-09-10T08:00:00Z').toISOString(),
    size: 1024,
    seen: true,
    flagged: false,
    has_attachment: false,
    snippet: '摘要',
  }
}

type Props = React.ComponentProps<typeof MailList>

const noop = () => {}

const base: Props = {
  folder: null,
  messages: [msg(1), msg(2)],
  threads: null,
  activeThreadId: null,
  onSelectThread: noop,
  onToggleFlagThread: noop,
  onDeleteThread: noop,
  onMarkReadThread: noop,
  onMoveThread: noop,
  selectedThreadIds: new Set(),
  onToggleSelectThread: noop,
  selfAddrs: new Set(),
  loading: false,
  activeMessageId: null,
  onSelectMessage: noop,
  onToggleFlag: noop,
  listStyle: 'card',
  hasNextPage: true,
  isFetchingNextPage: false,
  onLoadMore: noop,
  searchValue: '',
  onSearchChange: noop,
  searching: false,
  filter: EMPTY_FILTER,
  onToggleFilter: noop,
  onClearFilter: noop,
  sourceKey: 'ms-1-card-',
  selectedIds: new Set(),
  onToggleSelect: noop,
  onSelectRange: noop,
  onSelectRangeThread: noop,
  onSelectAllVisible: noop,
  onClearSelection: noop,
  onBatchRead: noop,
  onBatchFlag: noop,
  onBatchDelete: noop,
  onBatchMove: noop,
  moveTargets: [],
  alwaysShowSelect: false,
  onDeleteMessage: noop,
  folders: [],
  onMarkRead: noop,
  onMoveMessage: noop,
}

describe('MailList 的数据状态出口', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount(props: Partial<Props> = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        // MailList 内部用 useToast（撤销条）与 useAddBlock（右键屏蔽发件人），
        // 两个 provider 都得在场
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <MailList {...base} {...props} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
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

  it('翻页失败时底部给出重试入口，而不是静默停在第 N 页', async () => {
    const onRetryNextPage = vi.fn()
    await mount({ nextPageError: true, onRetryNextPage })

    const foot = container.querySelector('.list-foot')!
    expect(foot.textContent).toContain('list.loadMoreFailed')
    // 「没有更多」会把故障说成数据的尽头，必须让位给失败态
    expect(foot.textContent).not.toContain('list.noMore')

    const retry = foot.querySelector<HTMLButtonElement>('.list-foot-retry')!
    await act(async () => retry.click())
    expect(onRetryNextPage).toHaveBeenCalledTimes(1)
  })

  it('翻页失败且整个 query 被置为 error 时，已加载的邮件一封都不能少', async () => {
    // 真实链路里这两个必然同时为真：react-query 的 status 是整个 query 的
    //（见 lib/query-semantics.test.ts）。此前的测试只给了 nextPageError、
    // 没给 error，那个组合不可达，于是「整屏错误态吃掉列表」溜了过去。
    await mount({ nextPageError: true, error: new Error('第二页炸了') })

    // 邮件行一封都没少
    expect(container.querySelectorAll('.mail-item').length).toBeGreaterThan(0)
    // 整屏错误面板不该出现——那会把屏幕上的邮件全换成一句「加载失败」
    expect(container.querySelector('.list-error')).toBeNull()
    // 失败只落在底部那一行
    expect(container.querySelector('.list-foot')!.textContent).toContain('list.loadMoreFailed')
  })

  it('真的翻到底时才说「没有更多」', async () => {
    await mount({ hasNextPage: false })
    const foot = container.querySelector('.list-foot')!
    expect(foot.textContent).toContain('list.noMore')
    expect(foot.querySelector('.list-foot-retry')).toBeNull()
  })

  it('后台刷新的进度条延迟出现，短促的重取不会让它频闪', async () => {
    vi.useFakeTimers()
    try {
      await mount({ refreshing: true })
      // 阈值内先不出现：同步收尾一次 invalidate 多个 key，本地接口几十毫秒就返回
      await act(async () => {
        vi.advanceTimersByTime(150)
      })
      expect(container.querySelector('.list-refresh-bar')).toBeNull()

      // 超过阈值才亮
      await act(async () => {
        vi.advanceTimersByTime(100)
      })
      expect(container.querySelector('.list-refresh-bar')).not.toBeNull()

      // 刷新结束立即收起
      await mount({ refreshing: false })
      expect(container.querySelector('.list-refresh-bar')).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })

  it('首屏失败走整列表错误态，不渲染空态文案', async () => {
    const onRetry = vi.fn()
    await mount({ messages: [], error: new Error('boom'), onRetry })

    expect(container.querySelector('.list-error')).not.toBeNull()
    expect(container.textContent).toContain('boom')
    expect(container.textContent).not.toContain('list.noMessages')
  })

  it('还有下一页且没在加载时，底部不留一条空白', async () => {
    // 原先 .list-foot 无条件渲染，三种状态都不成立时里面是 null，
    // 但容器那 12px 上下内边距照常生效——列表底下恒挂一条空白，
    // 看起来像「还有一行没加载出来」。空的容器不是空的。
    await mount({ hasNextPage: true, isFetchingNextPage: false, nextPageError: false })
    expect(container.querySelector('.list-foot'), '底部挂了一个空容器').toBeNull()
  })

  it('正在加载下一页时底部照常给出提示', async () => {
    // 上一条是「不该出现时没出现」，这条是「该出现时出现了」——
    // 只写前者的话，把整个 .list-foot 删掉测试也会绿。
    await mount({ hasNextPage: true, isFetchingNextPage: true })
    expect(container.querySelector('.list-foot')?.textContent).toContain('list.loadingMore')
  })

  it('没有可投递的文件夹时，移动按钮禁用（而不是弹一个空菜单）', async () => {
    // moveTargets 的空与非空**同时**决定按钮禁不禁用和菜单有没有项。
    // 两处曾经用不同的集合（一处未过滤 \Noselect、一处过滤了），
    // 全是 \Noselect 时按钮可用而菜单为空，点下去是个空白方框。
    await mount({ moveTargets: [], selectedIds: new Set([1]) })
    const btn = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
      (b) => b.getAttribute('aria-label') === 'list.batchMove',
    )
    expect(btn, '找不到批量移动按钮').toBeTruthy()
    expect(btn!.disabled, '没有可投递目标时按钮却是可用的').toBe(true)
  })

  it('没有主题的邮件在列表里显示与阅读区同一句文案', async () => {
    // 原先列表用字面量 '—' 而阅读区用 t('list.noSubject')，
    // 同一封邮件点开前后显示不同；更别扭的是行的 aria-label 里用的一直是后者，
    // 于是读屏念「（无主题）」而屏幕上写着一个破折号。
    await mount({ messages: [{ ...msg(1), subject: '' }] })
    const subject = container.querySelector('.mi-subject')
    expect(subject?.textContent).toBe('list.noSubject')
  })
})

describe('MailList 的列表语义与键盘导航', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount(props: Partial<Props> = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <MailList {...base} {...props} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
  }

  const many = Array.from({ length: 8 }, (_, i) => msg(i + 1))

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('虚拟化下每行仍能说出「第几项，共几项」', async () => {
    await mount({ messages: many })

    // 条目行 = 带 posinset 的那些 listitem（日期分组标题也是 listitem，但不占条目编号）
    const items = container.querySelectorAll('[role="listitem"][aria-posinset]')
    expect(items.length).toBeGreaterThan(0)
    // DOM 里只有视口内的行，所以 setsize 必须显式给出总数而不是让读屏数 DOM
    expect(items.length).toBeLessThanOrEqual(many.length)
    for (const el of items) {
      expect(el.getAttribute('aria-setsize')).toBe(String(many.length))
    }
    expect(items[0].getAttribute('aria-posinset')).toBe('1')
    expect(container.querySelector('[role="list"]')).not.toBeNull()
  })

  it('整份列表在 Tab 序列里只占一个停留点', async () => {
    await mount({ messages: many })

    const rows = [...container.querySelectorAll<HTMLElement>('.mail-item')]
    expect(rows.length).toBeGreaterThan(1)
    const tabbable = rows.filter((r) => r.getAttribute('tabindex') === '0')
    // 每行都 tabIndex=0 的话，键盘用户要按几十次 Tab 才能穿过列表，
    // 而且穿过的内容随滚动位置变化
    expect(tabbable).toHaveLength(1)
    expect(tabbable[0].dataset.roving).toBe('true')
  })

  it('停留点跟着当前打开的那一封走', async () => {
    await mount({ messages: many, activeMessageId: 3 })

    const roving = container.querySelector<HTMLElement>('[data-roving="true"]')!
    expect(roving.classList.contains('selected')).toBe(true)
  })

  it('方向键在列表内移动，Home / End 到首尾', async () => {
    const onSelectMessage = vi.fn()
    await mount({ messages: many, activeMessageId: 3, onSelectMessage })

    const rows = container.querySelector('[role="list"]')!
    function press(key: string) {
      const ev = new KeyboardEvent('keydown', { key, bubbles: true })
      container.querySelector<HTMLElement>('[data-roving="true"]')!.dispatchEvent(ev)
    }

    await act(async () => press('ArrowDown'))
    expect(onSelectMessage).toHaveBeenLastCalledWith(4)

    await act(async () => press('ArrowUp'))
    expect(onSelectMessage).toHaveBeenLastCalledWith(2)

    await act(async () => press('Home'))
    expect(onSelectMessage).toHaveBeenLastCalledWith(1)

    await act(async () => press('End'))
    expect(onSelectMessage).toHaveBeenLastCalledWith(many.length)
    expect(rows).not.toBeNull()
  })

  it('行内按钮的键盘语义不被列表导航抢走', async () => {
    const onSelectMessage = vi.fn()
    await mount({ messages: many, activeMessageId: 3, onSelectMessage })

    // 星标按钮上按方向键不应该移动列表
    const star = container.querySelector<HTMLElement>('.mi-star')!
    await act(async () => {
      star.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    })
    expect(onSelectMessage).not.toHaveBeenCalled()
  })
})

describe('列表序号只认渲染顺序', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount(props: Partial<Props> = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <MailList {...base} {...props} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
  }

  // 日期解析不出来的那封会被 groupByDate 归进「更早」组，而该组排在固定分组之后
  // ——于是渲染顺序与输入顺序不同。序号若回输入数组去数就会错位。
  const reordered = [
    { ...msg(1), date: 'not-a-date', subject: '乱序的那封' },
    { ...msg(2), subject: '正常的那封' },
  ]

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('分组打乱顺序后，停留点仍落在当前打开的那一封上', async () => {
    await mount({ messages: reordered, activeMessageId: 1 })

    const roving = container.querySelector<HTMLElement>('[data-roving="true"]')!
    // 回 messages 数组去数的话，这里会落到「正常的那封」上——选中高亮与焦点分家
    expect(roving.textContent).toContain('乱序的那封')
  })

  it('分组打乱顺序后，方向键打开的是相邻那一行本身', async () => {
    const onSelectMessage = vi.fn()
    await mount({ messages: reordered, activeMessageId: 1, onSelectMessage })

    await act(async () => {
      container
        .querySelector<HTMLElement>('[data-roving="true"]')!
        .dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }))
    })

    // 渲染顺序是「正常的那封」在前、「乱序的那封」在后，所以上一条是 id=2
    expect(onSelectMessage).toHaveBeenLastCalledWith(2)
  })

  it('aria-setsize 数的是条目，分组标题不占编号', async () => {
    await mount({ messages: reordered })
    const items = container.querySelectorAll('[role="listitem"][aria-posinset]')
    expect(items.length).toBe(2)
    for (const el of items) {
      expect(el.getAttribute('aria-setsize')).toBe('2')
    }
    // 分组标题也在 listitem 里（ARIA 只允许 list 直接拥有 listitem），
    // 但不带 posinset/setsize，所以不占条目编号。
    // 限定在 [role="list"] 内：标题栏的列表名也是 heading（level 2），但它不是分组标题。
    const headings = container.querySelectorAll('[role="list"] [role="heading"]')
    expect(headings.length).toBeGreaterThan(0)
    for (const h of headings) {
      expect(h.closest('[role="listitem"]')).not.toBeNull()
      expect(h.closest('[role="listitem"]')!.hasAttribute('aria-posinset')).toBe(false)
    }
  })
})

describe('删除当前邮件后焦点不掉出列表', () => {
  let container: HTMLDivElement
  let root: Root

  async function mount(props: Partial<Props> = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ToastProvider>
            <MailList {...base} {...props} />
          </ToastProvider>
        </QueryClientProvider>,
      )
    })
  }

  const many = Array.from({ length: 8 }, (_, i) => msg(i + 1))

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('带焦点的行被移除后，焦点跟到新的停留点', async () => {
    // 删的必须是最后一封：虚拟行的 key 是索引，删中间一封时 React 复用同一个
    // DOM 节点、焦点根本没离开过，复现不出这条。删最后一封才会真的摘掉节点。
    const three = many.slice(0, 3)
    await mount({ messages: three, activeMessageId: 3 })
    await act(async () => {
      container.querySelector<HTMLElement>('[data-roving="true"]')!.focus()
    })

    // 「处理后自动前进」：当前这封被删掉，active 落到相邻一封。
    // 带焦点的节点在同一次提交里被摘掉，焦点先回到 body——
    // 判据若是「提交后读 activeElement 还在不在列表里」，这里就补不上焦点，
    // 用户按一次删除就被踢回页面顶端，正是 roving tabindex 要解决的问题的反面。
    await mount({ messages: three.filter((m) => m.id !== 3), activeMessageId: 2 })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })

    const roving = container.querySelector<HTMLElement>('[data-roving="true"]')!
    expect(document.activeElement).toBe(roving)
    expect(roving.textContent).toContain('邮件 2')
  })

  it('点过列表外的空白之后，不再把焦点抢回列表', async () => {
    // 阅读区正文这类不可聚焦区域被点击时，行会 blur 且 relatedTarget 为 null
    // ——与「带焦点的行被删掉」同形。靠 blur 分不开，曾经的折中是「为 null 就保持原值」，
    // 代价就是这条：用户点一下阅读区、正在读信，下一次 j/k 或删除前进
    // 会把焦点和滚动位置一起拽回列表。
    const three = many.slice(0, 3)
    await mount({ messages: three, activeMessageId: 1 })
    await act(async () => {
      container.querySelector<HTMLElement>('[data-roving="true"]')!.focus()
    })

    // 点列表外的空白（不可聚焦，所以焦点去了 body）
    const outside = document.createElement('div')
    document.body.appendChild(outside)
    await act(async () => {
      outside.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
      ;(document.activeElement as HTMLElement | null)?.blur()
    })

    // 此后 active 变化（j/k 或删除前进）不该把焦点拽回来
    await mount({ messages: three, activeMessageId: 2 })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })

    expect(document.activeElement).not.toBe(container.querySelector('[data-roving="true"]'))
    outside.remove()
  })

  it('焦点移到列表外的元素上（如阅读区 iframe）时不抢回来', async () => {
    // 阅读区正文是**非同源沙箱 iframe**，事件不跨文档边界：点邮件正文时顶层
    // 一次 pointerdown 都不会触发，靠 pointerdown 消歧的那条路在这里是盲的。
    // 而焦点此刻实实在在落在 <iframe> 元素上——这是 effect 运行时无歧义的事实。
    const three = many.slice(0, 3)
    await mount({ messages: three, activeMessageId: 1 })
    await act(async () => {
      // 先点一下列表内的行，让「焦点在列表里」成立
      const row = container.querySelector<HTMLElement>('[data-roving="true"]')!
      row.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
      row.focus()
    })

    // 焦点移到列表外（模拟点进 iframe：没有顶层 pointerdown，只有焦点转移）
    const frame = document.createElement('iframe')
    document.body.appendChild(frame)
    await act(async () => frame.focus())
    expect(document.activeElement).toBe(frame)

    // 此后 active 变化不该把焦点拽回列表（滚动仍然要做，见下一条）
    await mount({ messages: three, activeMessageId: 2 })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })

    expect(document.activeElement).toBe(frame)
    frame.remove()
  })

  it('焦点在阅读区时，选中行仍然滚进视口', async () => {
    // j / k 是全局快捷键、刻意不动焦点，而点开一封邮件后焦点就落在阅读区。
    // 这两种最常见的情形下「当前读到哪一封」在列表上必须看得见，否则翻几封
    // 高亮就跑到视口外面了。滚动是显示、焦点是交互，判据不能共用——早前两者
    // 合在一个 effect 里共用 focusInListRef，焦点一离开列表就连滚都不滚了。
    await mount({ messages: many, activeMessageId: 1 })
    const frame = document.createElement('iframe')
    document.body.appendChild(frame)
    await act(async () => frame.focus())
    scrollIndexCalls.length = 0

    await mount({ messages: many, activeMessageId: 8 })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })

    // 8 封同一天的邮件 + 1 行日期分组标题，第 8 封在下标 8
    expect(scrollIndexCalls).toContain(8)
    // 滚了，但焦点仍留在阅读区
    expect(document.activeElement).toBe(frame)
    frame.remove()
  })

  it('焦点不在列表里时不抢焦点', async () => {
    await mount({ messages: many, activeMessageId: 3 })
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    await act(async () => outside.focus())

    await mount({ messages: many, activeMessageId: 5 })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })

    expect(document.activeElement).toBe(outside)
    outside.remove()
  })
})

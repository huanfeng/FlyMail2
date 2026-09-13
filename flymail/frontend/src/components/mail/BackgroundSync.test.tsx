import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MockAdapter from 'axios-mock-adapter'
import api from '@/lib/api'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { ConfirmProvider } from '@/components/ui/Confirm'
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
import type { Account, RealtimeEvent } from '@/lib/types'

// 捕获 connectRealtime 的回调，直接把事件喂进去，不必真起一条 SSE
let emit: ((ev: RealtimeEvent) => void) | null = null
let emitState: ((s: 'connecting' | 'open') => void) | null = null
vi.mock('@/lib/sse', () => ({
  connectRealtime: (
    cb: (ev: RealtimeEvent) => void,
    onState?: (s: 'connecting' | 'open') => void,
  ) => {
    emit = cb
    emitState = onState ?? null
    return () => {}
  },
}))

vi.mock('@/lib/browser-notify', () => ({
  showMailNotice: vi.fn(),
  playChime: vi.fn(),
  claimChime: () => false,
  pageHidden: () => false,
  notifyPermission: () => 'default',
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o != null ? `${k}:${Object.values(o).join('/')}` : k,
  }),
}))

vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK', refresh: 'RT', set: vi.fn(), clear: vi.fn(), isAuthenticated: () => true },
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function account(id: number, name: string): Account {
  return {
    id,
    name,
    email: `${name}@example.com`,
    auth_type: 'password',
    imap_host: 'h',
    imap_port: 993,
    imap_security: 'ssl',
    smtp_host: 'h',
    smtp_port: 465,
    smtp_security: 'ssl',
    status: 'ok',
    enabled: true,
  }
}

/**
 * 后台自动同步的可见性——整条链路。
 *
 * 这是这次改动的**全部意义**：Manager 按 pollInterval 定时跑的那一路没有触发者，
 * 前端此前根本不知道它在发生，界面上既不转圈也不显示进度，用户看到的只是
 * 「未读数偶尔自己跳一下」。
 *
 * 所以这里刻意**不**调 useAccountSync（那是手动触发那一路）：只挂 SSE 与侧栏，
 * 模拟"用户什么都没点"，看事件能不能一路走到账户行上。
 * 任何一环断掉（后端不发、useRealtimeSync 不写缓存、账户行不观察缓存），
 * 这条都会红。
 */
describe('后台自动同步在侧栏可见', () => {
  let mock: MockAdapter
  let container: HTMLDivElement
  let root: Root

  function Harness({ accounts }: { accounts: Account[] }) {
    useRealtimeSync({})
    return (
      <AccountSidebar
        accounts={accounts}
        folders={[]}
        activeAccountId={1}
        activeFolderId={null}
        notifOpen={false}
        settingsOpen={false}
        activeAgg={null}
        aggCounts={{ inbox: 0, unread: 0, starred: 0 }}
        onSelectAccount={vi.fn()}
        onSelectFolder={vi.fn()}
        onSelectAggregate={vi.fn()}
        onSync={vi.fn()}
        onAddAccount={vi.fn()}
        onToggleNotif={vi.fn()}
        onToggleSettings={vi.fn()}
        onCompose={vi.fn()}
        onOpenDrafts={vi.fn()}
      />
    )
  }

  async function flush(times = 3) {
    for (let i = 0; i < times; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0))
      })
    }
  }

  async function fire(ev: RealtimeEvent) {
    await act(async () => emit?.(ev))
    await flush(1)
  }

  /** 第 i 个账户行的容器（含同步按钮与进度行） */
  const rows = () => [...container.querySelectorAll('.account-row')]
  const progress = () => container.querySelectorAll('.sync-progress')
  const spinning = () => container.querySelectorAll('.spin-anim')

  beforeEach(async () => {
    emit = null
    emitState = null
    mock = new MockAdapter(api)
    mock.onGet(/.*/).reply(200, [])
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <ConfirmProvider>
            <Harness accounts={[account(1, 'alice'), account(2, 'bob')]} />
          </ConfirmProvider>
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

  it('前提：什么都没发生时既不转圈也没有进度行', () => {
    expect(rows().length).toBe(2)
    expect(spinning().length).toBe(0)
    expect(progress().length).toBe(0)
  })

  it('账户行只观察缓存，绝不发同步状态请求', async () => {
    // 这是整套设计最关键的一条不变量，而 beforeEach 里那条 onGet(/.*/) 正好把它遮住：
    // 哪天 useSyncStatus(acc.id, false) 的 false 被改成 true，通配会让
    // /sync/status 返回 []（phase 为 undefined），SSE 随后再写入，
    // 上面那些断言**照样全过**——而实际行为已经退化成 N 个每秒一次的轮询
    // （N = 账户数）。所以这条要单独钉。
    await fire({ type: 'sync_status', account_id: 2, phase: 'messages', folders_total: 4 })
    const hit = mock.history.get.some((r) => r.url?.includes('sync/status'))
    expect(hit, '账户行发起了同步状态请求——它该只读缓存').toBe(false)
  })

  it('收到后台同步事件后，那个账户转圈并显示进度', async () => {
    await fire({
      type: 'sync_status',
      account_id: 2,
      phase: 'messages',
      folders_total: 12,
      folders_done: 3,
      // 后端给的是服务器原名 + 类型；系统文件夹的展示名由前端按 type 本地化
      current_folder: 'INBOX',
      current_folder_type: 'inbox',
    })

    expect(spinning().length, '没有任何账户在转圈——后台同步依然不可见').toBe(1)
    expect(progress().length).toBe(1)

    const bar = container.querySelector('[role="progressbar"]')!
    expect(bar.getAttribute('aria-valuenow')).toBe('3')
    expect(bar.getAttribute('aria-valuemax')).toBe('12')
    expect(container.textContent).toContain('folder.inbox')
  })

  it('进度只出现在正在同步的那个账户上', async () => {
    await fire({ type: 'sync_status', account_id: 2, phase: 'messages', folders_total: 4 })
    // 侧栏里账户 1 排在前面；进度行必须挂在账户 2 的块里
    const blocks = [...container.querySelectorAll('.sync-progress')]
    expect(blocks.length).toBe(1)
    // 用 parentElement 而不是 closest('div')：后者会先命中元素自身
    // （.sync-progress 本身就是 div），拿到的其实还是父元素，读起来像在找祖先。
    const owner = blocks[0].parentElement
    expect(owner?.textContent).toContain('bob')
    expect(owner?.textContent).not.toContain('alice')
  })

  it('同步结束后转圈与进度一并收起', async () => {
    await fire({ type: 'sync_status', account_id: 2, phase: 'messages', folders_total: 4 })
    expect(spinning().length).toBe(1)

    await fire({ type: 'sync_status', account_id: 2, phase: 'done' })
    expect(spinning().length, 'done 之后还在转圈').toBe(0)
    expect(progress().length).toBe(0)
  })

  it('排队阶段也算在同步——那段时间同样要有表达', async () => {
    // 后端抢不到全局同步名额时会停在 queued。不认它的话，
    // 用户看到的是"什么都没发生"，而实际上后台正在等名额。
    await fire({ type: 'sync_status', account_id: 1, phase: 'queued' })
    expect(spinning().length).toBe(1)
    expect(container.textContent).toContain('sync.queued')
  })

  it('同步失败不残留在同步态', async () => {
    await fire({ type: 'sync_status', account_id: 1, phase: 'messages', folders_total: 3 })
    await fire({ type: 'sync_status', account_id: 1, phase: 'error', error: 'boom' })
    expect(spinning().length).toBe(0)
    expect(progress().length).toBe(0)
  })

  it('在途的旧轮询响应不能把已完成的同步改回进行中', async () => {
    // 这条是本次改动引入的回归：每个账户行改成直接读裸缓存之后，
    // 一个在同步期间发出、SSE 推来 done 之后才落地的轮询响应会把状态改回
    // messages——而那之后轮询已经停了，**再没有东西会来纠正它**，
    // 侧栏那个账户于是一直转圈，直到下一轮后台同步（默认 3 分钟）。
    await fire({
      type: 'sync_status',
      account_id: 2,
      phase: 'messages',
      folders_total: 12,
      folders_done: 5,
      updated_at: '2026-09-13T10:00:03Z',
    })
    expect(spinning().length).toBe(1)

    await fire({
      type: 'sync_status',
      account_id: 2,
      phase: 'done',
      updated_at: '2026-09-13T10:00:05Z',
    })
    expect(spinning().length).toBe(0)

    // 迟到的旧快照
    await fire({
      type: 'sync_status',
      account_id: 2,
      phase: 'messages',
      folders_total: 12,
      folders_done: 5,
      updated_at: '2026-09-13T10:00:03Z',
    })
    expect(spinning().length, '旧快照把已完成的同步改回了进行中，而且不会自愈').toBe(0)
    expect(progress().length).toBe(0)
  })

  it('SSE 重连后对账，错过的 done 不会让账户永久转圈', async () => {
    // SSE 是尽力推送：合盖唤醒、切网、后端重启期间的事件全丢，
    // 慢客户端还会被 hub 主动丢掉进度帧（它是可丢的那一类）。
    // 错过的那条恰好是 done 时，缓存永远停在 messages——
    // 而账户行只读缓存不发请求，手动那一路也早收手了。
    let statusHits = 0
    // ⚠ 具体 handler 必须注册在通配之前：axios-mock-adapter 按注册顺序匹配，
    //   反了的话请求被 beforeEach 里那条 onGet(/.*/) 拦下，statusHits 永远是 0，
    //   而那种错法的表现是「测试红得莫名其妙」或（更糟）某些断言悄悄变成恒真。
    mock.resetHandlers()
    mock.onGet(/\/accounts\/2\/sync\/status$/).reply(() => {
      statusHits++
      return [200, { account_id: 2, phase: 'done', updated_at: '2026-09-13T10:00:09Z' }]
    })
    mock.onGet(/.*/).reply(200, [])

    await fire({
      type: 'sync_status',
      account_id: 2,
      phase: 'messages',
      folders_total: 12,
      folders_done: 5,
      updated_at: '2026-09-13T10:00:03Z',
    })
    expect(spinning().length).toBe(1)

    // 断线重连：done 在断开期间丢了
    await act(async () => emitState?.('connecting'))
    await act(async () => emitState?.('open'))
    await flush()

    expect(statusHits, '重连后没有对账').toBeGreaterThan(0)
    expect(spinning().length, '重连对账之后仍然在转圈').toBe(0)
  })

  it('没有账户处于活跃态时，重连不打任何对账请求', async () => {
    // 弱网下重连很频繁。无差别全拉会在每次重连时打出 N 个请求。
    let statusHits = 0
    mock.resetHandlers()
    mock.onGet(/\/sync\/status$/).reply(() => {
      statusHits++
      return [200, { phase: 'none' }]
    })
    mock.onGet(/.*/).reply(200, [])

    await act(async () => emitState?.('open'))
    await flush()
    expect(statusHits).toBe(0)
  })
})

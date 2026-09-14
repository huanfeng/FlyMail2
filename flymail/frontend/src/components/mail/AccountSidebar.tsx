// 侧边栏组件 — 复刻 MailMaster Sidebar 结构
// 参考 .dev/mailmaster/src_extracted/03_f2308e64.js + app.css
// 所有颜色严格使用 CSS 设计令牌，不写死任何颜色值

import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { useToast } from '@/components/ui/Toast'
import { Icon } from '@/components/ui/Icon'
import type { IconName } from '@/components/ui/Icon'
import { CtxMenu, type CtxMenuItem } from '@/components/ui/ContextMenu'
import { AccountDialog } from '@/components/mail/AccountDialog'
import {
  useNotificationUnread,
  useAccountUnread,
  useMe,
  useSetAccountEnabled,
  useDeleteAccount,
  useSyncStatus,
  useFolderReadAll,
} from '@/lib/queries'
import type { AggregateView } from '@/lib/queries'
import { isSyncActive } from '@/lib/types'
import type { Account, Folder, SyncStatus } from '@/lib/types'
import { auth } from '@/lib/auth'
import { folderName } from '@/lib/mail-format'

/** 实时连接的三态。'offline' = 连不上已持续 ≥ OFFLINE_AFTER_MS。 */
export type ConnState = 'open' | 'connecting' | 'offline'

/**
 * 品牌栏右侧的实时连接指示灯。
 *
 * 这个位置原本是一颗写死成 `var(--ink-4)` 的装饰圆点——不可点击、没有 title、
 * 不随任何状态变化。但它长在标题栏里、又是个小圆点，看上去就像状态灯，
 * 用户第一反应就是问「这个圆点是做什么的」。既然它已经在传达"某种状态"，
 * 那就让它真的传达状态，而不是把误导留在那儿。
 *
 * 它与那条 `.conn-banner` 横幅**不重复**，两者覆盖的时间段不同：
 * 横幅要等断开满 6 秒才弹（它是打扰性的，为一次一秒的重连弹出来只会烦人），
 * 而断开的**头六秒**里界面上此前没有任何迹象——那六秒里新邮件既不刷新列表
 * 也不弹通知。这颗灯补的正是那段空窗。
 *
 * 用 title 而不是 aria-live：连接状态每次重连都会变，做成 live region 就是
 * 每次切网络都对读屏用户念一遍。真正需要打断的情形（断了很久）由横幅负责，
 * 那里已经有 role="status"。
 */
function ConnDot({ state }: { state: ConnState }) {
  const { t } = useTranslation()
  return (
    <div
      className={`brand-dot brand-dot-${state}`}
      title={t(`realtime.state.${state}`)}
      role="img"
      aria-label={t(`realtime.state.${state}`)}
    />
  )
}

/**
 * 同步进度行。
 *
 * 三种表达，按后端给得出什么来分：
 * - 知道文件夹总数（messages 阶段）→ 确定进度条 + 「第 3 / 12 个文件夹 · 收件箱」
 * - 还不知道（queued / folders 阶段）→ 不确定进度条 + 阶段名
 * - 后端没返回状态（刚触发、状态还没建立）→ 也走不确定那一支
 *
 * ⚠ 分母是**文件夹数**不是邮件数，界面上也如实这么写。早先这里用的是
 * `status.total / status.processed`，而后端只在同步**结束**时才写那两个字段——
 * 于是确定态那一支在真实路径上一次都没出现过，而代码看起来像是做了进度。
 *
 * 粒度也只到文件夹：单个文件夹内部的分批抓取没有对外的进度出口，
 * 所以首次导入时 INBOX 那一格会停留很久。够用但不够细。
 */
export function SyncProgress({ status }: { status: SyncStatus | null }) {
  const { t } = useTranslation()
  const total = status?.folders_total ?? 0
  const done = status?.folders_done ?? 0
  const determinate = total > 0

  // 复用已有的 sync.* 文案，不另起一层 sync.phase.*——同一件事两套键是下一个漂移源
  // ⚠ 兜底分支落在 messages 上：调用方用 isSyncActive 门控，进不来 done/error/none。
  // 但万一门控被改动绕过，显示「正在同步邮件…」比显示空白更容易被发现是错的——
  // 空白会被当成"加载中"，而一个明确的错误状态会有人来报。
  const phaseLabel =
    status?.phase === 'queued'
      ? t('sync.queued')
      : status?.phase === 'folders'
        ? t('sync.folders')
        : t('sync.messages')

  // ⚠ 用 folderName 而不是直接显示 current_folder：后端给的是服务器原名
  // （INBOX / Sent Items），而侧栏的文件夹列表对系统文件夹显示的是本地化名。
  // 直接显示的话同一个文件夹在两处叫两个名字。
  const here =
    status?.current_folder != null && status.current_folder !== ''
      ? folderName(status.current_folder_type ?? '', status.current_folder, t)
      : ''
  const detail = determinate
    ? t('sync.folderProgress', { done, total }) + (here ? ` · ${here}` : '')
    : ''

  return (
    // ⚠ 整块不能是 live region：文件夹进度在长同步里会变很多次，
    // 而读屏会把每一次都念出来，把新邮件提醒和操作结果全挤掉。
    // 进度交给 progressbar（读屏按用户自己的节奏查询），live region 里只放阶段名。
    <div className="sync-progress">
      <div
        className={'sync-bar' + (determinate ? '' : ' indeterminate')}
        role="progressbar"
        aria-valuemin={0}
        {...(determinate
          ? {
              'aria-valuemax': total,
              'aria-valuenow': done,
              'aria-valuetext': detail,
            }
          : {})}
        aria-label={phaseLabel}
      >
        {determinate && (
          <span
            className="sync-bar-fill"
            style={{ width: `${Math.min(100, (done / total) * 100)}%` }}
          />
        )}
      </div>
      {/* 可见文本已被上面的 progressbar 完整表达，对读屏是重复的 */}
      <span className="sync-progress-text" aria-hidden="true">
        {determinate ? `${phaseLabel} ${detail}` : phaseLabel}
      </span>
      <span className="sr-only" role="status" aria-live="polite">
        {phaseLabel}
      </span>
    </div>
  )
}

/**
 * 历史正文回补的进度。
 *
 * 与 SyncProgress 的区别是**表达强度**：那个有转圈、有进度条，说的是
 * 「邮件还在收」；这个只有一行小字，说的是「邮件已经收全了，正在把正文也拉下来」。
 * 后者慢得多（5000 封要分 25 轮、跨一个多小时），用同等强度表达会一直在那儿晃。
 *
 * 分母来自后端一轮捞到的待补条数，分子是已落库封数——比文件夹粒度精确得多。
 */
export function BodyPrefetchNote({ status }: { status: SyncStatus | null }) {
  const { t } = useTranslation()
  const total = status?.bodies_total ?? 0
  if (total <= 0) return null
  const done = status?.bodies_done ?? 0
  return (
    <div className="body-prefetch-note">{t('sync.bodies', { done, total })}</div>
  )
}

/** 从名称取首字母（最多 2 个），用于头像占位 */
function nameInitials(name: string): string {
  const s = name.trim()
  if (!s) return '?'
  return s
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0] ?? '')
    .join('')
    .toUpperCase()
}

/** 未读超过这个数时，「全部标为已读」先弹确认。比一屏多一点。 */
const READ_ALL_CONFIRM_AT = 20

// ── 文件夹类型 → 图标名称映射 ─────────────────────────────
const FOLDER_ICON: Record<string, IconName> = {
  inbox: 'inbox',
  sent: 'send',
  drafts: 'draft',
  trash: 'trash',
  junk: 'tag',
  archive: 'archive',
  custom: 'folder',
}


// ── 侧栏错误行 ───────────────────────────────────────────
//
// 侧栏的两条链路失败时 data 都回落成空数组，界面与「一个账户都没有」
// 「这个账户没有文件夹」完全同形。没有这一行，一次 500 就表现为一个空侧栏：
// 既看不出是故障，也没有重试的入口。
function SideError({ text, onRetry }: { text: string; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="side-error" role="status">
      <span>{text}</span>
      {onRetry && (
        <button type="button" className="side-error-retry" onClick={onRetry}>
          {t('app.retry')}
        </button>
      )}
    </div>
  )
}

// ── Props ────────────────────────────────────────────────
interface Props {
  accounts: Account[]
  /** 账户列表加载失败（与「一个账户都没有」是两回事，必须分开表达） */
  accountsError?: unknown
  onRetryAccounts?: () => void
  folders: Folder[]
  /** 当前账户的文件夹加载失败 */
  foldersError?: unknown
  onRetryFolders?: () => void
  activeAccountId: number | null
  activeFolderId: number | null
  /** 通知浮层是否打开，用于高亮铃铛 */
  notifOpen: boolean
  /** 设置浮层是否打开，用于高亮齿轮 */
  settingsOpen: boolean
  /**
   * SSE 实时连接状态，画在品牌栏那颗点上。
   *
   * 'connecting' 与 'offline' 的区别是**持续了多久**（阈值归 useRealtimeSync 管）：
   * 前者是一次寻常的重连，后者是已经断了一会儿、那条横幅也已经弹出来了。
   */
  connState: ConnState
  /** 当前激活的聚合入口（null 表示未选中聚合） */
  activeAgg: AggregateView | null
  /** 聚合入口徽标计数 */
  aggCounts: Record<AggregateView, number>
  onSelectAccount: (id: number) => void
  onSelectFolder: (id: number) => void
  onSelectAggregate: (view: AggregateView) => void
  onSync: (accountId: number) => void
  onAddAccount: () => void
  /** 切换通知浮层 */
  onToggleNotif: () => void
  /** 切换设置浮层 */
  onToggleSettings: () => void
  onCompose: () => void
  onOpenDrafts: (accountId: number) => void
}

// ── 文件夹行 ─────────────────────────────────────────────

interface FolderRowProps {
  iconName: IconName
  label: string
  active: boolean
  count?: number
  onClick: () => void
  /** 右键菜单项（可选） */
  ctxItems?: CtxMenuItem[]
}

function FolderRow({ iconName, label, active, count, onClick, ctxItems }: FolderRowProps) {
  const btn = (
    <button
      type="button"
      className={'folder-row' + (active ? ' active' : '')}
      onClick={onClick}
    >
      {/* 文件夹图标 */}
      <span className="f-icon">
        <Icon name={iconName} size={13} />
      </span>
      {/* 文件夹名 */}
      <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', textAlign: 'left' }}>
        {label}
      </span>
      {/* 未读/数量 badge */}
      {count !== undefined && count > 0 && (
        <span className="f-count">{count > 99 ? '99+' : count}</span>
      )}
    </button>
  )
  return ctxItems && ctxItems.length > 0 ? <CtxMenu trigger={btn} items={ctxItems} /> : btn
}

// ── 账户行（可展开，含文件夹列表）────────────────────────

interface AccountBlockProps {
  acc: Account
  expanded: boolean
  active: boolean
  folders: Folder[]
  /** 文件夹加载失败（仅激活账户会传：非激活账户压根没发这个请求） */
  foldersError?: unknown
  onRetryFolders?: () => void
  activeFolderId: number | null
  /** 账户级未读数（后端去重口径，见 useAccountUnread） */
  unread: number
  onToggleExpand: () => void
  onSync: () => void
  onSelectFolder: (id: number) => void
  onOpenDrafts: () => void
  // ── 右键菜单动作 ──
  onEdit: () => void
  onToggleEnabled: () => void
  onDelete: () => void
}

function AccountBlock({
  acc,
  expanded,
  active,
  folders,
  foldersError,
  onRetryFolders,
  activeFolderId,
  unread,
  onToggleExpand,
  onSync,
  onSelectFolder,
  onOpenDrafts,
  onEdit,
  onToggleEnabled,
  onDelete,
}: AccountBlockProps) {
  const { t } = useTranslation()

  // 每个账户行自己观察自己的同步状态（enabled=false：只读缓存，不发请求）。
  //
  // 这样「谁在同步」不再依赖父组件传下来的那一个 id——后台自动同步没有触发者，
  // 父组件根本不知道它在跑。缓存由三方写入：手动触发那一路的轮询、
  // SSE 推送（手动与后台都走它）、以及触发成功时的乐观播种。
  const confirm = useConfirm()
  const { toast } = useToast()
  const readAll = useFolderReadAll()
  const { data: syncStatus } = useSyncStatus(acc.id, false)
  const syncing = isSyncActive(syncStatus?.phase)

  /**
   * 这一轮同步是不是用户自己点出来的。
   *
   * ── 为什么要分 ───────────────────────────────────────────────────────────
   *
   * 进度块（`.sync-progress`，约 24px）在正常流里。上一轮把后台自动同步也接进
   * 这套表达之后，它变成**每 180 秒每个账户自己插入一次**——用户没做任何操作，
   * 下面的文件夹列表和其它账户整体往下跳一次、几秒后又跳回来。
   *
   * 这个仓库里其实已经解过一次同样的问题，判据就写在 `.list-refresh-bar` 上：
   * 「绝对定位使它不占布局——刷新相当频繁，任何占位的指示都会让标题栏反复抖动」。
   * 上一轮我没把那条判据搬到侧栏来。
   *
   * 但两处不完全一样：列表那条只需要表达"在刷新"，而这里还有一行文字
   * （「第 3 / 12 个文件夹 · 收件箱」），塞不进 2px 的条里。所以按**来源**分：
   *
   *   用户点了同步  → 完整进度块。此刻他正盯着这里，块展开是**反馈**不是抖动。
   *   后台自动同步  → 只留账户行上那条 2px 的绝对定位进度条 + 转圈的点，
   *                   零布局影响。想看细节可以 hover（title 里有）。
   *
   * ── 归属判定 ─────────────────────────────────────────────────────────────
   *
   * 建模成「**这一轮**同步归谁」，而不是「此刻是不是用户点的」。
   *
   * ⚠ 不能写成「同步结束就复位」：BodyPrefetchNote 恰恰是在同步**结束后**
   * （phase=done 且还有待补正文）才显示的，那样两个条件永远凑不齐，
   * 正文回补的进度就再也不会出现——一个看起来很合理、实则把功能改没了的写法。
   *
   * 复位点是**下一轮同步开始**（syncing 假→真）：那时候消费掉 armed 标记，
   * 没被 arm 过的就是后台自己跑的。owned 因此能一直活到下一轮开始，
   * 覆盖住正文回补那段。
   */
  const [owned, setOwned] = useState(false)
  const armedRef = useRef(false)
  const prevSyncingRef = useRef(syncing)
  useEffect(() => {
    if (syncing && !prevSyncingRef.current) {
      setOwned(armedRef.current)
      armedRef.current = false
    }
    prevSyncingRef.current = syncing
  }, [syncing])

  function triggerSync() {
    // 立刻置真而不是只 arm：用户点了就该马上有反馈，不必等状态回来。
    // 右键菜单那一项在同步进行中也可点（后端会 409），此时用户显然是想看进度，
    // 直接显示正合其意。
    setOwned(true)
    armedRef.current = true
    onSync()
  }

  /**
   * 同步按钮的名字：闲时是动作名，同步中换成进度描述。
   *
   * 后台同步那一路不渲染带 aria-live 的进度块（见 owned 处），读屏用户的信息
   * 全靠这里。把文件夹进度也拼进来，Tab 过去就能听到「第 3 / 12 个文件夹 · 收件箱」，
   * 而不是只有一句"正在同步"。
   */
  const syncFolderTotal = syncStatus?.folders_total ?? 0
  const syncLabel = !syncing
    ? t('sync.trigger')
    : syncFolderTotal > 0
      ? `${t('sync.messages')} ${t('sync.folderProgress', {
          done: syncStatus?.folders_done ?? 0,
          total: syncFolderTotal,
        })}${
          syncStatus?.current_folder
            ? ` · ${folderName(syncStatus.current_folder_type ?? '', syncStatus.current_folder, t)}`
            : ''
        }`
      : t(syncStatus?.phase === 'queued' ? 'sync.queued' : 'sync.messages')

  // 账户右键菜单：同步 / 编辑 / 启停 / 删除
  const accountCtxItems: CtxMenuItem[] = [
    { key: 'sync', label: t('ctx.syncNow'), icon: 'circle-dot', onSelect: triggerSync, disabled: !acc.enabled },
    { key: 'edit', label: t('ctx.editAccount'), icon: 'compose', onSelect: onEdit },
    {
      key: 'enabled',
      label: acc.enabled ? t('ctx.disableAccount') : t('ctx.enableAccount'),
      icon: 'circle-dot',
      onSelect: onToggleEnabled,
    },
    { key: 'sep', separator: true },
    { key: 'del', label: t('ctx.deleteAccount'), icon: 'trash', destructive: true, onSelect: onDelete },
  ]
  /**
   * 文件夹右键菜单。按**每个文件夹**构造，不是全账户共用一份——
   * 「全部标为已读」要按该文件夹自己的未读数决定可不可点。
   *
   * 未读为 0 时置灰而不是隐藏：菜单项的位置固定下来，用户不必每次去找它在哪。
   */
  function folderCtxItems(f: Folder): CtxMenuItem[] {
    const unreadHere = f.unread_count ?? 0
    return [
      {
        key: 'read-all',
        label: t('ctx.markFolderRead'),
        icon: 'check',
        disabled: unreadHere === 0 || readAll.isPending,
        onSelect: () => void markFolderRead(f),
      },
      { key: 'sep', separator: true },
      { key: 'sync', label: t('ctx.syncNow'), icon: 'circle-dot', onSelect: triggerSync, disabled: !acc.enabled },
    ]
  }

  /**
   * 全部标为已读。
   *
   * 数量大时先确认：这个操作**不可撤销**（未读状态没有历史），而入口在右键菜单里，
   * 很容易误点。阈值取 20——比一屏多一点，少于这个数用户自己也能一封封点回来。
   */
  async function markFolderRead(f: Folder) {
    const n = f.unread_count ?? 0
    if (n === 0) return
    if (n > READ_ALL_CONFIRM_AT) {
      const ok = await confirm({
        title: t('ctx.markFolderReadConfirmTitle'),
        body: t('ctx.markFolderReadConfirmBody', { count: n, folder: folderName(f.type, f.display_name, t) }),
        confirmLabel: t('ctx.markFolderRead'),
      })
      if (!ok) return
    }
    readAll.mutate(f.id, {
      onSuccess: (res) => toast(t('ctx.markFolderReadDone', { count: res.marked })),
      onError: () => toast(t('ctx.markFolderReadFailed')),
    })
  }

  return (
    <div>
      {/* 账户标题行 — 点击展开/收起，右键弹操作菜单 */}
      <CtxMenu
        items={accountCtxItems}
        trigger={
          <div className="group" style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
        <button
          type="button"
          className={'account-row' + (active ? ' active' : '')}
          onClick={onToggleExpand}
          style={{ flex: 1 }}
        >
          {/* 展开/收起 caret */}
          <span className={'caret' + (expanded ? ' open' : '')}>
            <Icon name="chevron-right" size={12} stroke={2} />
          </span>
          {/* 账户色点（用 accent 色代替自定义颜色，FlyMail 账户无 color 字段） */}
          <span
            className="acct-dot"
            style={{ background: 'var(--accent)' }}
          />
          {/* 账户名（优先显示 name，回落到 email） */}
          <span className="acct-name">{acc.name || acc.email}</span>
          {/* 未读总数（后端去重口径：收件箱+自定义文件夹，同一封跨标签只计一次） */}
          {unread > 0 && (
            <span className="acct-unread">{unread > 99 ? '99+' : unread}</span>
          )}
        </button>

        {/* hover 显示：同步按钮（账户管理移至设置弹框，此处不再放编辑/删除）*/}
        <div
          className="account-row-actions"
          style={{ display: 'flex', alignItems: 'center', flexShrink: 0, paddingRight: 6 }}
        >
          {/* 同步（进行中时旋转） */}
          <button
            type="button"
            className="icon-btn compact"
            // 同步中时按钮名换成进度描述。
            //
            // 后台同步不再渲染那个带 aria-live 的进度块，所以读屏用户需要另一条
            // 通路。**刻意不做成 live region**：后台同步每 180 秒一轮，播报出去
            // 就是每三分钟念一句「正在同步邮件」——那是视觉抖动对读屏用户的等价物，
            // 而且更难忽略。放在按钮名里是「按需可查」：用户 Tab 过来才听到。
            title={syncLabel}
            aria-label={syncLabel}
            // 停用的账户后端会直接拒（500），同步中再点会 409——两种都只换来一条
            // 错误提示。右键菜单里那一项本来就判了 acc.enabled，这个按钮漏了。
            disabled={syncing || !acc.enabled}
            onClick={(e) => { e.stopPropagation(); triggerSync() }}
          >
            {/* 判据只看「这个账户在不在同步」。原先是 `syncing && active`——
                那个 active 让同步非当前账户时屏幕上完全没有变化。 */}
            <Icon
              name="circle-dot"
              size={11}
              className={syncing ? 'spin-anim' : undefined}
            />
          </button>
        </div>

        {/* 后台自动同步的进度：压在账户行下沿的 2px 细条。
            绝对定位，**不占布局**——这正是 .list-refresh-bar 的做法，
            同一个判据：后台同步每 180 秒一轮，任何占位的指示都会让侧栏反复抖动。
            用户主动点的那一路走上面的完整进度块，不重复画这条。 */}
        {syncing && !owned && (
          <span
            className={
              'acct-sync-bar' +
              ((syncStatus?.folders_total ?? 0) > 0 ? '' : ' indeterminate')
            }
            aria-hidden="true"
          >
            {(syncStatus?.folders_total ?? 0) > 0 && (
              <span
                className="acct-sync-fill"
                style={{
                  width: `${Math.min(
                    100,
                    ((syncStatus?.folders_done ?? 0) / (syncStatus?.folders_total ?? 1)) * 100,
                  )}%`,
                }}
              />
            )}
          </span>
        )}
          </div>
        }
      />

      {/* 同步进度。首次导入几千封是分钟级操作，此前全部反馈只有上面那个 11px 的
          圆点在转——用户无从判断是在动、卡住了、还是快好了。
          现在阶段与文件夹进度都来自后端（手动触发走轮询，后台自动同步走 SSE 推送，
          两者写同一个缓存键）。 */}
      {/* ⚠ 只有**用户自己点出来的**同步才展开这个占位的块。
          后台自动同步每 180 秒一轮、没有任何用户操作，展开它就是让侧栏自己跳
          （用户报的第 7 条）。后台那一路的表达在账户行上：一条绝对定位、
          不占布局的 2px 进度条，加上本来就在转的那个点。判据与 .list-refresh-bar
          一致——见 owned 处的注释。 */}
      {syncing && owned && <SyncProgress status={syncStatus ?? null} />}
      {/* 正文回补：与同步**并行**的一条弱表达，刻意不转圈也不做成同步阶段。
          此刻邮件列表已经完整，缺的只是正文——用同步中的转圈表达它，
          会让用户以为邮件还没收全，那是错的信息。
          同样只在用户主动触发后显示：它比同步更长（5000 封跨一个多小时），
          后台冒出来的话侧栏会在整段时间里多一行、结束时再少一行。 */}
      {!syncing && owned && <BodyPrefetchNote status={syncStatus ?? null} />}

      {/* 展开的文件夹列表 */}
      {expanded && (
        <div className="folder-list">
          {foldersError != null && (
            <SideError text={t('sidebar.foldersError')} onRetry={onRetryFolders} />
          )}
          {folders
            .filter((f) => f.selectable)
            .map((f) => {
              const iconName: IconName = FOLDER_ICON[f.type] ?? 'folder'
              // 自定义文件夹用 display_name，系统文件夹走 i18n
              const label = f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
              return (
                <FolderRow
                  key={f.id}
                  iconName={iconName}
                  label={label}
                  active={f.id === activeFolderId}
                  count={
                    // 文件夹行徽标：收件箱/垃圾邮件/自定义显示各自未读（主流客户端行为），
                    // archive（Gmail 所有邮件）/回收站/已发送/草稿不显示
                    f.type === 'inbox' || f.type === 'junk' || f.type === 'custom'
                      ? f.unread_count
                      : undefined
                  }
                  onClick={() => onSelectFolder(f.id)}
                  ctxItems={folderCtxItems(f)}
                />
              )
            })}

          {/* 草稿箱（本地）入口 — 独立于 IMAP 文件夹 */}
          <FolderRow
            iconName="draft"
            label={t('compose.draftsBox')}
            active={false}
            onClick={onOpenDrafts}
          />
        </div>
      )}
    </div>
  )
}

// ── 主组件 ───────────────────────────────────────────────

export function AccountSidebar({
  accounts,
  accountsError,
  onRetryAccounts,
  folders,
  foldersError,
  onRetryFolders,
  activeAccountId,
  activeFolderId,
  notifOpen,
  settingsOpen,
  connState,
  activeAgg,
  aggCounts,
  onSelectAccount,
  onSelectFolder,
  onSelectAggregate,
  onSync,
  onAddAccount,
  onToggleNotif,
  onToggleSettings,
  onCompose,
  onOpenDrafts,
}: Props) {
  const { t } = useTranslation()
  const confirm = useConfirm()
  // 站内未读通知数（铃铛角标）
  const { data: unreadNotifs = 0 } = useNotificationUnread()
  // 各账户未读（后端统一口径，非激活账户也能显示）
  const { data: accountUnread = {} } = useAccountUnread()
  // 当前管理员资料（底部用户卡）
  const { data: me } = useMe()

  function handleLogout() {
    auth.clear()
    window.location.href = '/login'
  }

  // 各账户展开状态（默认展开前两个）
  const [expanded, setExpanded] = useState<Record<number, boolean>>(() => {
    const init: Record<number, boolean> = {}
    accounts.forEach((a, i) => { init[a.id] = i < 2 })
    return init
  })

  function toggleExpand(id: number) {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  // ── 账户右键菜单动作（编辑对话框 + 启停/删除 mutation，自包含于侧栏） ──
  const [editAcc, setEditAcc] = useState<Account | null>(null)
  const setEnabled = useSetAccountEnabled()
  const deleteAccount = useDeleteAccount()

  async function handleDeleteAccount(acc: Account) {
    const ok = await confirm({
      title: t('account.deleteConfirm'),
      body: t('account.deleteConfirmBody'),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    deleteAccount.mutate(acc.id)
  }

  // 聚合入口在邮件视图下才可能高亮
  // 聚合入口的高亮只在没有浮层压在上面时才代表「当前所在位置」
  const inMail = !notifOpen && !settingsOpen

  // 聚合入口配置
  const aggItems: { id: AggregateView; icon: IconName; labelKey: string }[] = [
    { id: 'inbox', icon: 'inbox', labelKey: 'sidebar.allInboxes' },
    { id: 'unread', icon: 'circle-dot', labelKey: 'sidebar.allUnread' },
    { id: 'starred', icon: 'star', labelKey: 'sidebar.starred' },
  ]

  return (
    // .col.sidebar 由 AppLayout 提供外层容器，这里只填充内容
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>

      {/* ── 顶部品牌栏 .sidebar-head ───────────────────────── */}
      <div className="sidebar-head">
        {/* 品牌方块 logo。通知改为浮层后没有「回到邮件视图」这回事了
            （主区域始终是邮件），于是退回纯展示元素，不再是可点击控件。 */}
        <div className="brand-mark" aria-hidden="true">F</div>
        {/* 品牌名 */}
        <div className="brand-name">{t('app.name')}</div>
        <ConnDot state={connState} />
        {/* 铃铛按钮：开关通知浮层 */}
        <button
          type="button"
          className={'icon-btn bell-wrap' + (notifOpen ? ' active' : '')}
          title={t('notif.title')}
          aria-label={t('notif.title')}
          onClick={onToggleNotif}
        >
          <Icon name="bell" size={14} />
          {unreadNotifs > 0 && <span className="bell-badge" />}
        </button>
      </div>

      {/* ── 写邮件按钮 .compose-btn ────────────────────────── */}
      <button
        type="button"
        className="compose-btn"
        onClick={onCompose}
      >
        <Icon name="compose" size={14} />
        <span>{t('sidebar.compose')}</span>
        {/* 快捷键提示 */}
        <span className="kbd">C</span>
      </button>

      {/* ── 滚动区 .sidebar-scroll ─────────────────────────── */}
      <div className="sidebar-scroll">

        {/* 聚合入口：所有收件箱 / 所有未读 / 星标（跨所有账户）*/}
        {aggItems.map((it) => (
          <button
            key={it.id}
            type="button"
            className={'side-btn' + (inMail && activeAgg === it.id ? ' active' : '')}
            onClick={() => onSelectAggregate(it.id)}
          >
            <Icon name={it.icon} size={14} />
            <span style={{ flex: 1, textAlign: 'left' }}>{t(it.labelKey)}</span>
            {aggCounts[it.id] > 0 && (
              <span className="count">{aggCounts[it.id] > 99 ? '99+' : aggCounts[it.id]}</span>
            )}
          </button>
        ))}

        {/* 账户区 section label */}
        <div className="side-section-label">
          <span>{t('sidebar.accounts')}</span>
          {/* 添加账户按钮 */}
          <button
            type="button"
            className="add"
            title={t('account.add')}
            aria-label={t('account.add')}
            onClick={onAddAccount}
          >
            <Icon name="plus" size={12} />
          </button>
        </div>

        {/* 账户列表 */}
        {accountsError != null && (
          <SideError text={t('sidebar.accountsError')} onRetry={onRetryAccounts} />
        )}
        {accounts.map((acc) => (
          <AccountBlock
            key={acc.id}
            acc={acc}
            expanded={!!expanded[acc.id]}
            active={inMail && activeAgg == null && acc.id === activeAccountId}
            // 只有激活账户才传入文件夹，其余传空数组节省渲染
            folders={acc.id === activeAccountId ? folders : []}
            foldersError={acc.id === activeAccountId ? foldersError : undefined}
            onRetryFolders={onRetryFolders}
            activeFolderId={activeFolderId}
            unread={accountUnread[acc.id] ?? 0}
            onToggleExpand={() => {
              // 展开时同时切换账户选中（若点击非激活账户）
              if (acc.id !== activeAccountId) onSelectAccount(acc.id)
              toggleExpand(acc.id)
            }}
            onSync={() => onSync(acc.id)}
            onSelectFolder={onSelectFolder}
            onOpenDrafts={() => onOpenDrafts(acc.id)}
            onEdit={() => setEditAcc(acc)}
            onToggleEnabled={() => setEnabled.mutate({ id: acc.id, enabled: !acc.enabled })}
            onDelete={() => handleDeleteAccount(acc)}
          />
        ))}

        {/* labels 标签区：FlyMail 暂无标签功能，本阶段完全隐藏（不放静态假数据） */}

      </div>

      {/* 账户右键「编辑」对话框 */}
      <AccountDialog
        open={editAcc !== null}
        account={editAcc}
        onOpenChange={(open) => { if (!open) setEditAcc(null) }}
      />

      {/* ── 底部 .sidebar-foot ─────────────────────────────── */}
      <div className="sidebar-foot">
        {/* 当前管理员信息：点击打开设置（资料分区） */}
        <button
          type="button"
          className="me"
          style={{ cursor: 'pointer', background: 'transparent', border: 0, textAlign: 'left' }}
          title={t('settings.navProfile')}
          onClick={onToggleSettings}
        >
          {/* 头像方块（取展示名/用户名首字母） */}
          <div
            className="avatar-sq"
            style={{
              width: 28,
              height: 28,
              borderRadius: 6,
              background: 'var(--accent)',
              display: 'grid',
              placeItems: 'center',
              color: 'white',
              fontSize: 11,
              fontWeight: 600,
              flexShrink: 0,
            }}
          >
            {nameInitials(me?.display_name || me?.username || 'FM')}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="me-name" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {me?.display_name || me?.username || t('app.name')}
            </div>
            <div className="me-mail" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {me?.email
                || (accounts.length > 0
                  ? t('sidebar.accountCount', { count: accounts.length })
                  : t('sidebar.noAccounts'))}
            </div>
          </div>
        </button>

        {/* 设置入口 → 切换设置浮层 */}
        <button
          type="button"
          className={'icon-btn' + (settingsOpen ? ' active' : '')}
          title={t('settings.title')}
          aria-label={t('settings.title')}
          onClick={onToggleSettings}
        >
          <Icon name="settings" size={15} />
        </button>

        {/* 登出 */}
        <button
          type="button"
          className="icon-btn"
          title={t('sidebar.logout')}
          aria-label={t('sidebar.logout')}
          onClick={handleLogout}
        >
          <Icon name="logout" size={15} />
        </button>
      </div>

    </div>
  )
}

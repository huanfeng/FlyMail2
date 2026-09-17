import { useRef, useEffect, useCallback, useMemo, useState } from 'react'
import { shouldLoadMore } from '@/lib/list-guards'
import {
  STACK_WIDTH, HEADER_ROW_H, rowHeight,
} from '@/lib/list-density'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { groupByDate } from '@/lib/date-group'
import type { GroupLabeler } from '@/lib/date-group'
import { isFilterActive } from '@/lib/list-filters'
import type { FilterKey, ListFilter } from '@/lib/list-filters'
import {
  LAYOUT_EVENT,
  LAYOUT_LIMITS,
  clampWidth,
  loadLayoutWidths,
  saveLayoutWidths,
} from '@/lib/layout-prefs'
import type { LayoutWidths } from '@/lib/layout-prefs'
import type { ListStyle } from '@/lib/list-prefs'
import { formatParticipants, pickAvatarParticipant } from '@/lib/thread-format'
import type { Folder, MessageListItem, ThreadListItem } from '@/lib/types'
import { Icon } from '@/components/ui/Icon'
import { DropMenu } from '@/components/ui/DropMenu'
import { highlightText } from '@/components/ui/Highlight'
import { SearchSyntaxHelp } from '@/components/mail/SearchSyntaxHelp'
import { RemoteSearchButton } from '@/components/mail/RemoteSearchButton'
import { extractHighlightTerms } from '@/lib/search-terms'
import { errorText } from '@/lib/format'
import { CtxMenu, type CtxMenuItem } from '@/components/ui/ContextMenu'
import { ResizeHandle } from '@/components/ui/ResizeHandle'
import { useToast } from '@/components/ui/Toast'
import { apiErrorMessage } from '@/lib/api'
import { useAddBlock } from '@/lib/queries'
import { isValidBlockPattern, normalizeBlockPattern } from '@/lib/rules'
import { FOCUS_SEARCH_EVENT } from '@/hooks/useKeyboardShortcuts'
import { searchShortcutHint } from '@/lib/platform'

// ─────────────────────────────────────────────────────────────────────────────
// 类型定义
// ─────────────────────────────────────────────────────────────────────────────

/**
 * 虚拟化行模型：分组标题 / 单封邮件 / 会话（两种条目互斥，由 threads 是否为 null 决定）。
 *
 * `pos` 是该条目在**整个列表**里的序号（从 1 起，跳过分组标题）。虚拟化让 DOM 里
 * 只剩视口内的十几行，读屏据 DOM 推断出的「第几项、共几项」必然是错的，
 * 只能由 aria-posinset / aria-setsize 显式给出——这正是这两个属性存在的理由。
 */
type RowItem =
  | { type: 'header'; label: string }
  | { type: 'item'; msg: MessageListItem; pos: number }
  | { type: 'thread'; item: ThreadListItem; pos: number }

interface Props {
  folder: Folder | null
  messages: MessageListItem[]
  /**
   * 会话模式的数据源：非 null 时列表渲染会话行，messages 被忽略。
   *
   * 为什么不把两种条目塞进同一个数组：会话行的选择集合是 thread_id 字符串，
   * 单封是数字 id，混成 `Set<number | string>` 之后 `messages.find(m => m.id === id)`
   * 这类比较会在类型检查通过的前提下永远为假（数字与字符串永不相等），
   * 而选中项求共同账户正是这么写的。两套集合各自成立，编译期就挡住串用。
   */
  threads: ThreadListItem[] | null
  activeThreadId: string | null
  onSelectThread: (item: ThreadListItem) => void
  onToggleFlagThread: (item: ThreadListItem, flagged: boolean) => void
  onDeleteThread: (item: ThreadListItem) => void
  onMarkReadThread: (item: ThreadListItem, read: boolean) => void
  onMoveThread: (item: ThreadListItem, folderId: number) => void
  /** 已选会话 id 集合（会话模式专用，与 selectedIds 同一时刻只有一个在用）*/
  selectedThreadIds: Set<string>
  onToggleSelectThread: (id: string) => void
  /** 本人邮箱集合（已小写），用于会话行头像避开自己 */
  selfAddrs: Set<string>
  loading: boolean
  /**
   * **首屏**加载失败时的错误（没有任何内容可显示的那种）。
   *
   * 必须有这一路：只看 loading 的话，后端 500 / 断网 / 令牌失效全都会落进
   * itemCount === 0 的空态分支，界面显示「这个文件夹里还没有邮件」——
   * 把服务故障谎报成一个空收件箱，用户既判断不出真相也没有重试的入口。
   *
   * ⚠ 调用方请传 `isLoadingError` 派生的值，别传裸 `error`：react-query 的
   * status 是整个 query 的，翻页失败与后台重取失败同样会填上 error 而数据还在。
   * 组件这边也不会拿它掀掉已有内容（渲染时叠加了 itemCount === 0），
   * 两道都留着——一道表达语义，一道兜住下次有人传错。
   */
  error?: unknown
  /** 重试当前列表请求（错误态里的「重试」按钮） */
  onRetry?: () => void
  /**
   * 后台刷新中（首屏已有内容，但正在重新取数）。
   *
   * 这个应用里后台刷新非常频繁：同步完成后一次性 invalidate 多个 key、SSE 推送，
   * 以及删除/移动/标记已读等 mutation 的收尾。不外露的话，用户会在毫无预期的
   * 时刻看到列表整体换内容。
   */
  refreshing?: boolean
  /**
   * 一个邮箱账户都还没有。
   *
   * 与「这个文件夹是空的」是两回事，必须分开：新用户首次登录时两者都表现为
   * itemCount === 0，若共用同一套文案，他看到的就是一句「暂无邮件」，
   * 而唯一的添加入口是侧栏一个 + 图标——等于没有引导。
   */
  noAccounts?: boolean
  /** 空态里的「添加账户」按钮 */
  onAddAccount?: () => void
  /**
   * 账户识别色查询。仅聚合视图/搜索结果传入——单文件夹视图里所有邮件都属于
   * 同一个账户，点一个到处都一样的色点只是噪声。
   */
  acctColorOf?: (accountId: number) => string | null
  activeMessageId: number | null
  onSelectMessage: (id: number) => void
  onToggleFlag: (id: number, flagged: boolean) => void
  listStyle: ListStyle
  hasNextPage: boolean
  isFetchingNextPage: boolean
  /**
   * 翻页请求失败（首屏成功、第 N 页失败）。
   *
   * 必须单独一路：失败不会改变行数，自动翻页的判据「最末可见行接近底部」
   * 仍然成立，若不拦住就是按帧重发；而拦住之后若不给重试入口，列表就停在
   * 第 N 页处一声不吭——底部既不显示「加载中」也不显示「没有更多」，
   * 看起来像是邮件到这里就没有了。
   */
  nextPageError?: boolean
  onLoadMore: () => void
  /** 重试失败的翻页请求 */
  onRetryNextPage?: () => void
  /** 标题覆盖：聚合视图（无 folder）时使用 */
  titleOverride?: string
  /** 副标题覆盖：聚合视图时使用 */
  subtitleOverride?: string
  /** 搜索框值（受控，由 Shell 管理以驱动后端搜索） */
  searchValue: string
  onSearchChange: (v: string) => void
  /**
   * 当前列表是否为搜索结果。
   * 必须由 Shell 传入而非从 searchValue 推导：Shell 用的是防抖后的串，
   * 刚敲下第一个字时列表还是文件夹内容，此时冒出「在服务器上搜索」会指向错的东西。
   */
  searching: boolean
  /**
   * 筛选条件（受控，由 Shell 管理）。
   * 必须由 Shell 持有：筛选是后端查询条件的一部分，在这里做前端过滤只能筛到
   * 已加载的那一页——用户点「未读」看到 5 条，实际有 32 条未读还没翻到。
   */
  filter: ListFilter
  onToggleFilter: (key: FilterKey) => void
  onClearFilter: () => void
  /**
   * 数据源标识（搜索/聚合/文件夹 + 列表样式）。变化时内部重置滚动并重新测量虚拟列表。
   * 注意：不用 React key 重挂载组件——否则会打断搜索框输入焦点，导致字母键落到全局快捷键。
   */
  sourceKey: string
  // ── 批量选择 ──
  /** 已选邮件 id 集合（由 Shell 管理，切换数据源时清空） */
  selectedIds: Set<number>
  onToggleSelect: (id: number) => void
  /**
   * Shift+点击：把一段连续的条目一次纳入选择。
   *
   * 与 onToggleSelect 分开而不是循环调用它：范围选择的语义是「都选上」，
   * 逐个 toggle 会把段内已选中的那些反而取消掉。
   */
  onSelectRange: (ids: number[]) => void
  onSelectRangeThread: (ids: string[]) => void
  onSelectAllVisible: () => void
  onClearSelection: () => void
  onBatchRead: (read: boolean) => void
  onBatchFlag: (flagged: boolean) => void
  onBatchDelete: () => void
  onBatchMove: (folderId: number) => void
  /** 批量移动目标（已选邮件共同账户的文件夹；跨账户/为空时禁用移动） */
  /** 可作为移动目标的文件夹。调用方已滤掉 \\Noselect：这里的空与非空
   *  同时决定按钮禁不禁用和菜单有没有项，两者必须是同一个集合。 */
  moveTargets: Folder[]
  /** 偏好：始终显示行内选择框（无需先进入选择模式） */
  alwaysShowSelect: boolean
  /** 列表行 hover 快捷删除单封 */
  onDeleteMessage: (id: number) => void
  // ── 右键菜单 ──
  /** 当前账户的文件夹列表（右键「移动到」目标；与邮件账户不符时不展示移动） */
  folders: Folder[]
  onMarkRead: (id: number, read: boolean) => void
  onMoveMessage: (id: number, folderId: number) => void
}

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────

/** 取发件人首字母（大写）*/
function initials(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

/**
 * 相对时间：参考 MailMaster relTimeMM，支持 zh/en
 * - < 1分钟 → 刚刚 / now
 * - < 1小时 → Nm / Nm
 * - 当天     → HH:mm（本地化）
 * - < 7天   → 周N（本地化）
 * - 其他    → M月D日 / Mon D
 */
function relTime(isoStr: string, lang: string): string {
  const ms = new Date(isoStr).getTime()
  if (Number.isNaN(ms)) return ''
  const diff = Date.now() - ms
  const isZh = lang === 'zh' || lang.startsWith('zh')

  if (diff < 60_000) return isZh ? '刚刚' : 'now'
  if (diff < 3_600_000) {
    const n = Math.floor(diff / 60_000)
    return isZh ? `${n}分` : `${n}m`
  }
  const d = new Date(ms)
  const todayStart = new Date()
  todayStart.setHours(0, 0, 0, 0)

  if (ms >= todayStart.getTime()) {
    // 当天：显示时间
    return d.toLocaleTimeString(isZh ? 'zh-CN' : undefined, {
      hour: 'numeric',
      minute: '2-digit',
    })
  }

  const weekAgo = todayStart.getTime() - 6 * 86_400_000
  if (ms >= weekAgo) {
    // 近 7 天：显示星期
    return d.toLocaleDateString(isZh ? 'zh-CN' : undefined, { weekday: 'short' })
  }

  // 更早：月/日
  return d.toLocaleDateString(isZh ? 'zh-CN' : undefined, {
    month: 'short',
    day: 'numeric',
  })
}

// ─────────────────────────────────────────────────────────────────────────────
// 选择复选框：行首独立一列（仅选择模式下由 CSS 显示），点击不触发打开邮件
// ─────────────────────────────────────────────────────────────────────────────

function SelectBox({
  checked,
  onToggle,
  label,
}: {
  checked: boolean
  onToggle: () => void
  /** 无障碍名称。必须点出是哪一封——读屏念「复选框」而看不到旁边那行内容 */
  label: string
}) {
  // 用 <label> 包裹原生 checkbox：点击整块都可靠切换（label 原生联动 input → onChange），
  // label 上 stopPropagation 阻止冒泡到行（避免误打开邮件）。
  return (
    <label className="mi-select" onClick={(e) => e.stopPropagation()}>
      <input
        type="checkbox"
        checked={checked}
        onChange={onToggle}
        aria-label={label}
      />
    </label>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 骨架屏：首屏加载占位
// ─────────────────────────────────────────────────────────────────────────────

function SkeletonList() {
  return (
    <div className="flex flex-col">
      {Array.from({ length: 8 }).map((_, i) => (
        <div
          // eslint-disable-next-line react/no-array-index-key
          key={i}
          className="mail-item animate-pulse"
        >
          {/* 头像占位 */}
          <div
            className="avatar-sq"
            style={{ background: 'var(--bg-sunk)' }}
          />
          <div className="flex flex-col gap-1.5 min-w-0">
            <div className="flex justify-between gap-4">
              <div className="h-3 rounded" style={{ width: '45%', background: 'var(--bg-sunk)' }} />
              <div className="h-3 rounded" style={{ width: '14%', background: 'var(--bg-sunk)' }} />
            </div>
            <div className="h-3 rounded" style={{ width: '70%', background: 'var(--bg-sunk)' }} />
            <div className="h-3 rounded" style={{ width: '55%', background: 'var(--bg-sunk)' }} />
          </div>
        </div>
      ))}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 卡片行（card 模式）：头像 + 三行信息，复刻 MailMaster .mail-item 结构
// ─────────────────────────────────────────────────────────────────────────────

interface CardRowProps {
  msg: MessageListItem
  active: boolean
  lang: string
  selected: boolean
  /** 搜索命中词（非搜索态为空数组，高亮函数会原样返回字符串） */
  terms: string[]
  /** 带事件时按修饰键决定行为；键盘路径不传，等同普通点击 */
  onSelect: (e?: React.MouseEvent) => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
  onDelete: () => void
  /** 账户识别色；null = 单账户上下文，不必区分 */
  acctColor: string | null
  /**
   * 这一行是否是 Tab 序列里的停留点（roving tabindex）。
   *
   * 虚拟化下每行都 tabIndex=0 的话，Tab 序列只含视口里那十几行、还随滚动变化——
   * 键盘用户按 Tab 穿过列表要按几十次，而且穿过的内容取决于他滚到了哪。
   * 整份列表只留一个停留点，进去之后用方向键走。
   */
  rovingTab: boolean
}

function CardRow({ msg, active, lang, selected, terms, onSelect, onToggleSelect, onToggleFlag, onDelete, acctColor, rovingTab }: CardRowProps) {
  const { t } = useTranslation()
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={rovingTab ? 0 : -1}
      data-roving={rovingTab ? 'true' : undefined}
      aria-current={active ? 'true' : undefined}
      onClick={onSelect}
      onKeyDown={(e) => {
        // Enter / Space 打开邮件；Space 需 preventDefault 阻止页面滚动
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect() }
      }}
      className={
        'mail-item' +
        (isUnread ? ' unread' : '') +
        (active ? ' selected' : '') +
        (selected ? ' batch-selected' : '')
      }
    >
      {/* 未读圆点（CSS 控制可见性）*/}
      <span className="mi-unread-dot" />

      {/* 选择复选框（独立列，仅选择模式下显示）*/}
      <SelectBox
        checked={selected}
        onToggle={onToggleSelect}
        label={t('list.selectMessage', { subject: msg.subject || t('list.noSubject') })}
      />

      {/* 方形头像。聚合/搜索视图下右下角点一个账户识别色，
          否则一列邮件全是同一个底色，看不出哪封属于哪个邮箱。 */}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
          {acctColor && <span className="acct-pip" style={{ background: acctColor }} />}
        </div>
      </span>

      <div style={{ minWidth: 0 }}>
        {/* 第一行：发件人 + 附件标记 + 时间。
            附件标记并入本行而非另起一行——行高必须与 estimateSize 恒等，
            不能随「有无附件」浮动。 */}
        <div className="mi-top">
          <span className="mi-sender">{highlightText(msg.from_name || msg.from_addr, terms)}</span>
          {msg.has_attachment && (
            <span className="mi-tags">
              <span className="mi-tag mi-attach">
                <Icon name="attach" size={10} />
              </span>
            </span>
          )}
          <span className="mi-time">{relTime(msg.date, lang)}</span>
        </div>

        {/* 第二行：主题 */}
        <div className="mi-subject">
          {msg.subject ? highlightText(msg.subject, terms) : t('list.noSubject')}
        </div>

        {/* 第三行：摘要（2 行截断由 CSS 控制）*/}
        {msg.snippet && (
          <div className="mi-preview">{highlightText(msg.snippet, terms)}</div>
        )}
      </div>

      {/* hover 快捷删除（星标左侧）*/}
      <button
        type="button"
        className="mi-del icon-btn"
        onClick={(e) => { e.stopPropagation(); onDelete() }}
        aria-label={t('reader.delete')}
        title={t('reader.delete')}
      >
        <Icon name="trash" size={14} />
      </button>

      {/* 星标按钮（hover 显示 / 已标星常显）*/}
      <button
        type="button"
        className={'mi-star icon-btn' + (msg.flagged ? ' starred' : '')}
        onClick={onToggleFlag}
        aria-label={msg.flagged ? t('ctx.unstar') : t('ctx.star')}
        style={{ position: 'absolute', right: 14, top: 14, opacity: msg.flagged ? 1 : undefined }}
      >
        <Icon name={msg.flagged ? 'star-fill' : 'star'} size={14} />
      </button>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 紧凑行（compact 模式）：单行密排，复刻 MailMaster .mail-item.mail-item-row
// grid: [28px] [160-220px] [1fr] [auto] [auto] [auto]
// ─────────────────────────────────────────────────────────────────────────────

interface CompactRowProps {
  /** 见 CardRowProps.rovingTab */
  rovingTab: boolean
  msg: MessageListItem
  active: boolean
  lang: string
  selected: boolean
  /** 搜索命中词（非搜索态为空数组，高亮函数会原样返回字符串） */
  terms: string[]
  /** 带事件时按修饰键决定行为；键盘路径不传，等同普通点击 */
  onSelect: (e?: React.MouseEvent) => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
  onDelete: () => void
  /** 账户识别色；null = 单账户上下文，不必区分 */
  acctColor: string | null
}

function CompactRow({ msg, active, lang, selected, terms, onSelect, onToggleSelect, onToggleFlag, onDelete, acctColor, rovingTab }: CompactRowProps) {
  const { t } = useTranslation()
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={rovingTab ? 0 : -1}
      data-roving={rovingTab ? 'true' : undefined}
      aria-current={active ? 'true' : undefined}
      onClick={onSelect}
      onKeyDown={(e) => {
        // Enter / Space 打开邮件；Space 需 preventDefault 阻止页面滚动
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect() }
      }}
      className={
        'mail-item mail-item-row' +
        (isUnread ? ' unread' : '') +
        (active ? ' selected' : '') +
        (selected ? ' batch-selected' : '')
      }
    >
      {/* 未读圆点 */}
      <span className="mi-unread-dot" />

      {/* 选择复选框（独立列，仅选择模式下显示）*/}
      <SelectBox
        checked={selected}
        onToggle={onToggleSelect}
        label={t('list.selectMessage', { subject: msg.subject || t('list.noSubject') })}
      />

      {/* 方形头像（小）*/}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
          {acctColor && <span className="acct-pip" style={{ background: acctColor }} />}
        </div>
      </span>

      {/* 发件人列 */}
      <div className="mi-top">
        <span className="mi-sender">{highlightText(msg.from_name || msg.from_addr, terms)}</span>
      </div>

      {/* 主题 + 摘要（单行，"— " 由 CSS ::before 注入）*/}
      <div className="mi-subject-preview">
        <span className="mi-subject">{msg.subject ? highlightText(msg.subject, terms) : t('list.noSubject')}</span>
        {msg.snippet && (
          <span className="mi-preview">{highlightText(msg.snippet, terms)}</span>
        )}
      </div>

      {/* 附件标签 */}
      {msg.has_attachment ? (
        <div className="mi-tags">
          <span className="mi-tag mi-attach">
            <Icon name="attach" size={10} />
          </span>
        </div>
      ) : (
        <span />
      )}

      {/* 时间列 */}
      <span className="mi-time-col">{relTime(msg.date, lang)}</span>

      {/* 删除 + 星标（同占最后一列）*/}
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 2 }}>
        <button
          type="button"
          className="mi-del icon-btn"
          onClick={(e) => { e.stopPropagation(); onDelete() }}
          aria-label={t('reader.delete')}
          title={t('reader.delete')}
          style={{ position: 'static' }}
        >
          <Icon name="trash" size={14} />
        </button>
        <button
          type="button"
          className={'mi-star icon-btn' + (msg.flagged ? ' starred' : '')}
          onClick={onToggleFlag}
          aria-label={msg.flagged ? t('ctx.unstar') : t('ctx.star')}
          style={{ position: 'static', opacity: msg.flagged ? 1 : undefined }}
        >
          <Icon name={msg.flagged ? 'star-fill' : 'star'} size={14} />
        </button>
      </span>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 会话行（会话模式）：与单封行同一套 class，只把「发件人」换成参与者、多一枚封数徽标
// ─────────────────────────────────────────────────────────────────────────────

interface ThreadRowProps {
  /** 见 CardRowProps.rovingTab */
  rovingTab: boolean
  item: ThreadListItem
  active: boolean
  lang: string
  selected: boolean
  listStyle: ListStyle
  /** 搜索命中词（非搜索态为空数组，高亮函数会原样返回字符串） */
  terms: string[]
  /** 本人邮箱集合（已小写），用于头像避开自己 */
  selfAddrs: Set<string>
  /** 带事件时按修饰键决定行为；键盘路径不传，等同普通点击 */
  onSelect: (e?: React.MouseEvent) => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
  onDelete: () => void
}

/**
 * 参与者展示串。
 *
 * 只有一个人时就显示这一个人（不写「等 1 人」）；超过 3 人时前 3 人 + 「等 N 人」。
 * 分隔符走 i18n：中文用顿号，英文用逗号。
 */
function participantsText(
  item: ThreadListItem,
  sep: string,
  formatOthers: (n: number) => string,
): string {
  const { names, extra } = formatParticipants(item.participants, 3)
  const head = names.join(sep)
  if (extra <= 0) return head
  return head + sep + formatOthers(extra)
}

function ThreadRow({
  item,
  active,
  lang,
  selected,
  listStyle,
  terms,
  selfAddrs,
  onSelect,
  onToggleSelect,
  onToggleFlag,
  onDelete,
  rovingTab,
}: ThreadRowProps) {
  const { t } = useTranslation()
  const isUnread = item.unread > 0
  const avatarOf = pickAvatarParticipant(item.participants, selfAddrs)
  const people = participantsText(
    item,
    t('list.thread.separator'),
    (n) => t('list.thread.andOthers', { count: n }),
  )
  const compact = listStyle === 'compact'

  // 封数徽标只在多于一封时出现：单封会话挂个「1」纯属噪音
  const countBadge = item.count > 1 ? <span className="mi-thread-count">{item.count}</span> : null

  return (
    <div
      role="button"
      tabIndex={rovingTab ? 0 : -1}
      data-roving={rovingTab ? 'true' : undefined}
      aria-current={active ? 'true' : undefined}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect() }
      }}
      className={
        'mail-item' +
        (compact ? ' mail-item-row' : '') +
        (isUnread ? ' unread' : '') +
        (active ? ' selected' : '') +
        (selected ? ' batch-selected' : '')
      }
    >
      <span className="mi-unread-dot" />
      <SelectBox
        checked={selected}
        onToggle={onToggleSelect}
        label={t('list.selectThread', { subject: item.subject || t('list.noSubject') })}
      />

      <span className="mi-avatar-wrap">
        <div className="avatar-sq" style={{ background: 'var(--accent)' }}>
          {initials(avatarOf?.name ?? '', avatarOf?.email ?? '')}
        </div>
      </span>

      {compact ? (
        <>
          {/* 参与者列（对应单封行的发件人列）*/}
          <div className="mi-top">
            <span className="mi-sender">{highlightText(people, terms)}</span>
            {countBadge}
          </div>

          <div className="mi-subject-preview">
            <span className="mi-subject">{item.subject ? highlightText(item.subject, terms) : t('list.noSubject')}</span>
            {item.snippet && <span className="mi-preview">{highlightText(item.snippet, terms)}</span>}
          </div>

          {item.has_attachment ? (
            <div className="mi-tags">
              <span className="mi-tag mi-attach"><Icon name="attach" size={10} /></span>
            </div>
          ) : (
            <span />
          )}

          <span className="mi-time-col">{relTime(item.date, lang)}</span>

          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 2 }}>
            <button
              type="button"
              className="mi-del icon-btn"
              onClick={(e) => { e.stopPropagation(); onDelete() }}
              aria-label={t('list.thread.delete')}
              title={t('list.thread.delete')}
              style={{ position: 'static' }}
            >
              <Icon name="trash" size={14} />
            </button>
            <button
              type="button"
              className={'mi-star icon-btn' + (item.flagged ? ' starred' : '')}
              onClick={onToggleFlag}
              aria-label={item.flagged ? t('list.thread.unstarAll') : t('list.thread.starAll')}
              style={{ position: 'static', opacity: item.flagged ? 1 : undefined }}
            >
              <Icon name={item.flagged ? 'star-fill' : 'star'} size={14} />
            </button>
          </span>
        </>
      ) : (
        <>
          <div style={{ minWidth: 0 }}>
            <div className="mi-top">
              <span className="mi-sender">{highlightText(people, terms)}</span>
              {countBadge}
              {item.has_attachment && (
                <span className="mi-tags">
                  <span className="mi-tag mi-attach"><Icon name="attach" size={10} /></span>
                </span>
              )}
              <span className="mi-time">{relTime(item.date, lang)}</span>
            </div>
            <div className="mi-subject">
              {item.subject ? highlightText(item.subject, terms) : t('list.noSubject')}
            </div>
            {item.snippet && <div className="mi-preview">{highlightText(item.snippet, terms)}</div>}
          </div>

          <button
            type="button"
            className="mi-del icon-btn"
            onClick={(e) => { e.stopPropagation(); onDelete() }}
            aria-label={t('list.thread.delete')}
            title={t('list.thread.delete')}
          >
            <Icon name="trash" size={14} />
          </button>
          <button
            type="button"
            className={'mi-star icon-btn' + (item.flagged ? ' starred' : '')}
            onClick={onToggleFlag}
            aria-label={item.flagged ? t('list.thread.unstarAll') : t('list.thread.starAll')}
            style={{ position: 'absolute', right: 14, top: 14, opacity: item.flagged ? 1 : undefined }}
          >
            <Icon name={item.flagged ? 'star-fill' : 'star'} size={14} />
          </button>
        </>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 主组件
// ─────────────────────────────────────────────────────────────────────────────

export function MailList({
  folder,
  messages,
  threads,
  activeThreadId,
  onSelectThread,
  onToggleFlagThread,
  onDeleteThread,
  onMarkReadThread,
  onMoveThread,
  selectedThreadIds,
  onToggleSelectThread,
  selfAddrs,
  loading,
  error,
  onRetry,
  refreshing,
  noAccounts,
  onAddAccount,
  acctColorOf,
  activeMessageId,
  onSelectMessage,
  onToggleFlag,
  listStyle,
  hasNextPage,
  isFetchingNextPage,
  nextPageError,
  onLoadMore,
  onRetryNextPage,
  titleOverride,
  subtitleOverride,
  searchValue,
  onSearchChange,
  searching,
  filter,
  onToggleFilter,
  onClearFilter,
  sourceKey,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  onSelectRangeThread,
  onSelectAllVisible,
  onClearSelection,
  onBatchRead,
  onBatchFlag,
  onBatchDelete,
  onBatchMove,
  moveTargets,
  alwaysShowSelect,
  onDeleteMessage,
  folders,
  onMarkRead,
  onMoveMessage,
}: Props) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const scrollRef = useRef<HTMLDivElement>(null)

  // ── 修饰键多选 ──────────────────────────────────────────────────────────────
  //
  // 锚点 = 上一次「普通点击 / Ctrl 点击」落在哪一行；Shift+点击 选的是锚点到
  // 当前行之间的整段。这套语义与文件管理器、主流邮件客户端一致，
  // 用户不需要重新学。
  const anchorRef = useRef<number | string | null>(null)

  function handleMessageClick(id: number, e?: React.MouseEvent) {
    // Ctrl/⌘+点击：只把这一行纳入/移出选择，不打开它
    if (e && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      onToggleSelect(id)
      anchorRef.current = id
      return
    }
    if (e?.shiftKey && anchorRef.current != null) {
      // 浏览器默认会把 Shift+点击当作选中文本，先按住
      e.preventDefault()
      const from = messages.findIndex((m) => m.id === anchorRef.current)
      const to = messages.findIndex((m) => m.id === id)
      if (from !== -1 && to !== -1) {
        const [lo, hi] = from <= to ? [from, to] : [to, from]
        onSelectRange(messages.slice(lo, hi + 1).map((m) => m.id))
        return
      }
    }
    anchorRef.current = id
    onSelectMessage(id)
  }

  function handleThreadClick(item: ThreadListItem, e?: React.MouseEvent) {
    if (e && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      onToggleSelectThread(item.thread_id)
      anchorRef.current = item.thread_id
      return
    }
    if (e?.shiftKey && anchorRef.current != null && threads) {
      e.preventDefault()
      const from = threads.findIndex((th) => th.thread_id === anchorRef.current)
      const to = threads.findIndex((th) => th.thread_id === item.thread_id)
      if (from !== -1 && to !== -1) {
        const [lo, hi] = from <= to ? [from, to] : [to, from]
        onSelectRangeThread(threads.slice(lo, hi + 1).map((th) => th.thread_id))
        return
      }
    }
    anchorRef.current = item.thread_id
    onSelectThread(item)
  }
  // 搜索框 ref：供快捷键 / 聚焦时使用
  const searchInputRef = useRef<HTMLInputElement>(null)

  // 搜索为受控值：由 Shell 管理并驱动后端跨账户搜索（searchValue/onSearchChange）。
  const query = searchValue

  // 「屏蔽此发件人」自带 mutation 与 toast：邮件行与会话行两处右键菜单都要用，
  // 提到 Shell 只是给一个本就臃肿的组件再加两个 prop。
  const { toast } = useToast()
  const { mutate: addBlock } = useAddBlock()
  const blockSender = useCallback((raw: string) => {
    const pattern = normalizeBlockPattern(raw)
    if (!isValidBlockPattern(pattern)) {
      toast(t('ctx.blockSenderInvalid'))
      return
    }
    addBlock({ pattern }, {
      // existed = 后端 409：对用户而言「已经屏蔽过」与「刚屏蔽成功」是同一件事
      onSuccess: (res) => toast(t(res.existed ? 'ctx.blockSenderExists' : 'ctx.blockSenderOk', { addr: pattern })),
      // 后端还会拒绝本地账户自己的邮箱（400），它的中文文案比通用提示准确
      onError: (e) => toast(apiErrorMessage(e, t('ctx.blockSenderFailed'))),
    })
  }, [addBlock, t, toast])

  /**
   * 能不能屏蔽这个地址：非空、格式合法、且不是本地账户自己的邮箱。
   *
   * 「已发送」文件夹里每一封的发件人都是自己，会话行同理——不挡住，用户点一下就把自己拉黑，
   * 之后所有自发自收的邮件都进垃圾箱。后端也会 400，但不该让这个菜单项出现在那里。
   */
  const blockableAddr = useCallback((raw: string | undefined): string | null => {
    const pattern = normalizeBlockPattern(raw ?? '')
    if (!isValidBlockPattern(pattern) || selfAddrs.has(pattern)) return null
    return pattern
  }, [selfAddrs])

  // 命中高亮词：用未防抖的 searchValue（Shell 防抖的是请求，高亮跟着输入走更跟手）。
  // 非搜索态 query 为空 → 空数组 → highlightText 原样返回字符串，不产生额外节点。
  const highlightTerms = useMemo(() => extractHighlightTerms(query), [query])

  /**
   * 把语法帮助里选中的片段追加到搜索框末尾并聚焦。
   * 需要补空格：`from:张三` 后面直接接 `is:unread` 会被后端当成一个 token。
   */
  const appendSyntax = useCallback((fragment: string) => {
    const base = query.length > 0 && !query.endsWith(' ') ? query + ' ' : query
    const next = base + fragment
    onSearchChange(next)
    // 等受控值回流到 DOM 后再把光标移到末尾，否则 setSelectionRange 会被覆盖
    requestAnimationFrame(() => {
      const el = searchInputRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(next.length, next.length)
    })
  }, [query, onSearchChange])

  // 监听快捷键 / 广播的自定义事件，聚焦搜索框
  useEffect(() => {
    function handleFocusSearch() {
      searchInputRef.current?.focus()
      searchInputRef.current?.select()
    }
    window.addEventListener(FOCUS_SEARCH_EVENT, handleFocusSearch)
    return () => {
      window.removeEventListener(FOCUS_SEARCH_EVENT, handleFocusSearch)
    }
  }, [])

  /**
   * 批量移动菜单的开合。
   *
   * 非受控（交给 radix）不够：菜单开着时**由程序**把 trigger 变 disabled 的路径
   * radix 覆盖不到——用快捷键清空选中、或换了数据源，菜单会留在屏幕上，
   * 而关闭时焦点回不到已 disabled 的 trigger，掉到 <body>。
   * 这个开关只在那两处用，日常开合仍由 radix 驱动（onOpenChange 回写）。
   */
  const [batchMoveOpen, setBatchMoveOpen] = useState(false)

  // ── 选择模式：由工具栏开关控制，控制行内复选框列是否出现 ──────────────────
  const [selectMode, setSelectMode] = useState(false)

  // ── 标题 ─────────────────────────────────────────────────────────────────
  // 聚合视图无 folder，使用 titleOverride
  const title = titleOverride ?? (folder
    ? folder.type === 'custom'
      ? folder.display_name
      : t(`folder.${folder.type}`)
    : '')

  // ── 副标题：N封 / N未读 ───────────────────────────────────────────────────
  const totalCount = folder?.total_count ?? messages.length
  const unreadCount = folder?.unread_count ?? 0
  const subLabel = subtitleOverride ?? (unreadCount > 0
    ? `${t('list.totalCount', { count: totalCount })} · ${t('list.unreadCount', { count: unreadCount })}`
    : t('list.totalCount', { count: totalCount }))

  // 是否显示副标题（folder 视图或聚合视图都显示）
  const showSub = folder != null || subtitleOverride != null

  // ── 日期分组标题 ──────────────────────────────────────────────────────────
  // 固定分组（今天/昨天/本周/本月/更早）走 i18n；历史月份交给 Intl 按当前语言渲染
  // ——zh-CN 得到「2024年5月」，en 得到「May 2024」，不必为每种语言的年月语序
  // 单独维护一条模板。
  const groupLabel = useMemo<GroupLabeler>(() => ({
    fixed: (kind) => t(`dateGroup.${kind}`),
    month: (year, month) =>
      new Date(year, month - 1).toLocaleDateString(lang.startsWith('zh') ? 'zh-CN' : lang, {
        year: 'numeric',
        month: 'long',
      }),
  }), [t, lang])

  // 是否有筛选生效（驱动空态文案与「清除筛选」按钮的出现）
  const filterActive = isFilterActive(filter)

  // ── 发件人列宽的就地拖拽 ──────────────────────────────────────────────────
  // 列宽的真相源是 layout-prefs（localStorage + LAYOUT_EVENT 广播），
  // 这里只是第二个编辑入口——设置里的滑块是第一个，两边通过事件互相同步。
  const [senderCol, setSenderCol] = useState(() => loadLayoutWidths().senderCol)
  useEffect(() => {
    function onLayoutChange(e: Event) {
      const d = (e as CustomEvent<LayoutWidths>).detail
      if (d) setSenderCol(d.senderCol)
    }
    window.addEventListener(LAYOUT_EVENT, onLayoutChange)
    return () => window.removeEventListener(LAYOUT_EVENT, onLayoutChange)
  }, [])

  /**
   * 手柄按下：拖拽期间直接改 CSS 变量让列宽跟手，松手才落盘并广播。
   * 每帧写 localStorage 既无必要也会拖慢拖拽手感（与 AppLayout 分栏拖拽同款处理）。
   */
  /** 发件人列宽：列左锚定，向右拖即变宽。 */
  function resizeSenderCol(dx: number) {
    setSenderCol((cur) =>
      clampWidth(cur + dx, LAYOUT_LIMITS.senderCol.min, LAYOUT_LIMITS.senderCol.max),
    )
  }

  // 列宽同步到 CSS 变量 + 防抖落盘（广播让设置里的滑块跟上）。
  // 拖拽期间必须由这里写变量：AppLayout 那边要等广播才知道新值，
  // 不自己写的话列边界会滞后于手柄。
  useEffect(() => {
    document.documentElement.style.setProperty('--sender-col-w', `${senderCol}px`)
    // 只写自己管的这一项（合并交给 saveLayoutWidths），并在关窗时补上未到期的改动。
    // pagehide 与 visibilitychange 都挂，理由同 AppLayout：前者在多种页面终止路径上
    // 并不保证触发，而后者是 Page Lifecycle 里唯一可以指望的那个。
    // pending 见 AppLayout 里同一处的注释：flush 只补「还没到期的」那次，
    // 否则 visibilitychange 每次切标签页都会白写一次盘、白广播一次
    let pending = true
    const id = setTimeout(() => {
      pending = false
      saveLayoutWidths({ senderCol })
    }, 200)
    const flush = () => {
      if (!pending) return
      pending = false
      saveLayoutWidths({ senderCol })
    }
    const onHide = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', onHide)
    return () => {
      clearTimeout(id)
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', onHide)
    }
  }, [senderCol])

  // 会话模式：threads 非 null 即以会话为条目，两种模式共用下面的分组/虚拟化/翻页逻辑。
  const threadMode = threads != null
  // 搜索与筛选都由后端完成，messages/threads 即为最终结果集，此处不再做任何前端过滤。
  const filtered = messages
  // 条目数（翻页守卫与空态判据都看它，不看分组后的行数）
  const itemCount = threadMode ? threads.length : messages.length

  // ── 构造虚拟化行模型 ──────────────────────────────────────────────────────
  // 两种列表样式都按日期分组：低密度的卡片模式更需要分组标题来制造阅读节奏，
  // 否则一屏五六张卡片糊成一片，反而比紧凑模式更难扫读。
  const rows: RowItem[] = (() => {
    const result: RowItem[] = []
    let pos = 0
    if (threadMode) {
      for (const group of groupByDate(threads, (th) => th.date, undefined, groupLabel)) {
        result.push({ type: 'header', label: group.label })
        for (const item of group.items) result.push({ type: 'thread', item, pos: ++pos })
      }
      return result
    }
    for (const group of groupByDate(filtered, (m) => m.date, undefined, groupLabel)) {
      result.push({ type: 'header', label: group.label })
      for (const msg of group.items) result.push({ type: 'item', msg, pos: ++pos })
    }
    return result
  })()

  /**
   * rows 里的条目行（跳过分组标题），顺序即渲染顺序。
   *
   * ⚠ 序号必须一律从这里数，不能回 `filtered` / `threads` 去数：
   * 那是 groupByDate 的**输入**，而 `pos` 数的是**输出**，两者只在
   * 「分组是输入的保序展平」时才相等——这个前提会破。`date-group.ts` 把日期解析
   * 不出来的条目归进 `earlier` 组，而该组不在 fixedKinds 里，是在固定分组之后
   * 按 Map 插入顺序发出的；后端返回顺序不是严格日期降序时（跨页游标遇到同一时刻、
   * 聚合视图跨账户归并）同样会破。错位的后果是方向键打开 A、焦点落到 B。
   */
  const itemRows = rows.filter(
    (r): r is Extract<RowItem, { type: 'item' } | { type: 'thread' }> => r.type !== 'header',
  )

  /** 列表里条目的总数（不含分组标题）。aria-setsize 用它，而不是 rows.length。 */
  const rowItemCount = itemRows.length

  /**
   * Tab 序列里那个唯一停留点的序号（1 起）。
   *
   * 默认落在当前打开的那一封；没有打开任何一封时落在第一条——
   * 否则整份列表都是 tabIndex=-1，键盘根本进不来。
   */
  const rovingPos = (() => {
    const i = itemRows.findIndex((r) =>
      r.type === 'thread' ? r.item.thread_id === activeThreadId : r.msg.id === activeMessageId,
    )
    return i >= 0 ? i + 1 : 1
  })()

  const rowsRef = useRef<HTMLDivElement>(null)
  /**
   * 焦点是否落在列表里。
   *
   * 为什么不在 effect 里读 `document.activeElement`：删除/归档当前邮件时
   * （第二轮加的「处理后自动前进」），带焦点的那个行节点在同一次提交里就被摘掉，
   * 焦点已经回到 body——等 effect 跑时看到的是「焦点不在列表里」于是不补焦点，
   * 用户按一次删除就被踢回页面顶端。那正是 roving tabindex 要解决的问题的反面。
   *
   * 为什么用 pointerdown 而不是 blur：`blur` 的 `relatedTarget` 为 null 有两种来源
   * ——焦点真的去了 body（点了阅读区正文这类不可聚焦的空白），或者带焦点的行刚被删掉。
   * 两者同形，靠 blur 分不开；曾经的折中是「为 null 就保持原值」，
   * 代价是点一下阅读区空白之后，下一次 j / k 或删除前进会把焦点**和滚动位置**
   * 一起拽回列表，而用户正在那边读信。
   * pointerdown 从源头消歧：点空白一定有一次落在列表外的 pointerdown，
   * 行被删除则一次都没有。用捕获阶段，免得被谁 stopPropagation 掉。
   */
  const focusInListRef = useRef(false)
  useEffect(() => {
    function onPointerDown(e: PointerEvent) {
      const el = rowsRef.current
      focusInListRef.current = el != null && el.contains(e.target as Node)
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => document.removeEventListener('pointerdown', onPointerDown, true)
  }, [])

  /**
   * 方向键在列表内移动（ARIA 列表的标准交互，补上 roving tabindex 的另一半）。
   *
   * 行为与既有的 j / k 一致：移动即打开。差别只在 j / k 是全局快捷键、
   * 不动 DOM 焦点，而这里是焦点已经落在列表里时的导航。
   */
  function moveRoving(to: number | 'first' | 'last') {
    if (rowItemCount === 0) return
    const next =
      to === 'first' ? 1
      : to === 'last' ? rowItemCount
      : Math.min(rowItemCount, Math.max(1, to))
    if (next === rovingPos) return
    // 选中哪一条与滚到哪一行都从同一个数组取，不存在两套编号对不上的可能
    const target = itemRows[next - 1]
    if (!target) return
    if (target.type === 'thread') onSelectThread(target.item)
    else onSelectMessage(target.msg.id)
    // 目标行可能还在视口之外（虚拟化没渲染它），不滚过去焦点就无处可落
    const rowIndex = rows.indexOf(target)
    if (rowIndex >= 0) virtualizer.scrollToIndex(rowIndex)
  }

  function onRowsKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    // 行内的按钮（星标/删除/复选框）有自己的键盘语义，别抢。
    // 判据是「焦点在容器本身或任意一行上」而不是「在当前 roving 行上」：
    // 后者在焦点意外落到非 roving 行时会让方向键整个哑掉，且毫无反馈。
    const el = e.target as HTMLElement
    if (el !== e.currentTarget && !el.classList.contains('mail-item')) return
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        moveRoving(rovingPos + 1)
        break
      case 'ArrowUp':
        e.preventDefault()
        moveRoving(rovingPos - 1)
        break
      case 'Home':
        e.preventDefault()
        moveRoving('first')
        break
      case 'End':
        e.preventDefault()
        moveRoving('last')
        break
      default:
        break
    }
  }

  // 焦点跟随停留点。只在焦点本来就在列表里时才动，否则 j / k 会把焦点从
  // 搜索框或工具栏抢过来。
  useEffect(() => {
    if (!focusInListRef.current) return
    // 再对现实核对一次：焦点此刻明确落在列表外的某个元素上就别抢。
    //
    // pointerdown 那条堵不住阅读区——正文是**非同源沙箱 iframe**，事件不跨文档边界，
    // 点邮件正文（读信时最常点的地方）顶层一次 pointerdown 都不会触发，
    // ref 于是原样保持 true。容器的 onFocus 也救不了：焦点移到的是 <iframe> 元素本身，
    // 那在列表外，不会冒泡成容器的 focusin。
    //
    // 这一条用的是 effect 运行时唯一无歧义的事实，不依赖任何事件的具体形状，
    // 因此是整类关闭而不是再堵一个入口。
    // **必须放行 body**：行被删除时 activeElement 正是回落到 body，那一路要补焦点。
    const active = document.activeElement
    if (active && active !== document.body && !rowsRef.current?.contains(active)) return
    // 先把目标行滚进视口：j / k 和「删除后自动前进跨过多行」不经过 moveRoving，
    // 目标行超出 overscan 时虚拟化根本没渲染它，查不到就无处落焦点。
    const rowIndex = rows.findIndex(
      (r) =>
        r.type !== 'header' &&
        (r.type === 'thread' ? r.item.thread_id === activeThreadId : r.msg.id === activeMessageId),
    )
    if (rowIndex >= 0) virtualizer.scrollToIndex(rowIndex)
    // 等一帧让虚拟化把目标行渲染出来
    const id = requestAnimationFrame(() => {
      const target = rowsRef.current?.querySelector<HTMLElement>('[data-roving="true"]')
      if (target && target !== document.activeElement) target.focus()
    })
    return () => cancelAnimationFrame(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeMessageId, activeThreadId])

  // 列表栏是不是窄到要堆叠。
  //
  // CSS 那侧用 @container 判断，但 estimateSize 是 JS、看不到容器查询的结果，
  // 只能自己观察一次栏宽。阈值必须与 index.css 里的 @container 同值，
  // 两者都取自 lib/list-density.ts。
  const listWrapRef = useRef<HTMLDivElement>(null)
  const [stackedRows, setStackedRows] = useState(false)
  useEffect(() => {
    const el = listWrapRef.current
    if (el == null) return
    const apply = (w: number) => setStackedRows(w > 0 && w <= STACK_WIDTH)
    apply(el.getBoundingClientRect().width)
    // 没有 ResizeObserver 就只按首次宽度定一次（jsdom、极老的浏览器）。
    // 不防御的话整个列表在测试环境里直接抛 ReferenceError。
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) apply(e.contentRect.width)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // ── 行高 ─────────────────────────────────────────────────────────────────
  // ⚠ 这不是"估算"而是硬契约：没有接 measureElement，虚拟列表就完全按这里的数值
  // 用 translateY 排布行槽位。行的真实高度一旦超出，就会压到下一行头上，
  // 表现为 hover / 选中高亮与行边界错位重叠。
  // index.css 用 .mail-item{height:100%} 让行严格填满槽位，两边必须同步改。
  // 具体数值与「窄栏堆叠」的阈值都在 lib/list-density.ts，那里同时被 index.css
  // 的 @container 规则引用（见该文件注释）。紧凑行在窄栏下是两行，高度不同——
  // 不跟着变的话行内容会溢出槽位，主题只露出上半截。
  const estimateSize = useCallback(
    (index: number): number => {
      const row = rows[index]
      if (!row) return 52
      if (row.type === 'header') return HEADER_ROW_H
      // 会话行与单封行共用同一套 class 与内部结构，高度自然相同
      return rowHeight(listStyle, stackedRows)
    },
    [rows, listStyle, stackedRows],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize,
    overscan: 5,
  })

  // 后台刷新指示延迟出现：同步收尾会一次 invalidate 多个 key，本地接口几十毫秒
  // 就返回，不加阈值这条 2px 的线只会反复亮灭——比不显示更烦人。
  // 复位放在 cleanup 里：refreshing 转假时 cleanup 先跑，指示随之收起。
  const [showRefresh, setShowRefresh] = useState(false)
  useEffect(() => {
    if (!refreshing) return
    const id = setTimeout(() => setShowRefresh(true), 200)
    return () => {
      clearTimeout(id)
      setShowRefresh(false)
    }
  }, [refreshing])

  // 上一次触发翻页时的底层邮件条数（判据见 list-guards.ts 的 shouldLoadMore）
  const loadedLenRef = useRef(-1)

  // 数据源/样式切换时重置滚动 + 重新测量（替代 React key 重挂载，避免打断搜索框焦点）。
  useEffect(() => {
    scrollRef.current?.scrollTo(0, 0)
    virtualizer.measure()
    // 换数据源时退出选择模式（选中项由 Shell 一并清空）
    setSelectMode(false)
    setBatchMoveOpen(false)
    // 换数据源后条数可能与上一个数据源巧合相同，翻页守卫必须一并归零
    loadedLenRef.current = -1
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sourceKey])

  // ── 无限加载：接近底部时触发 ──────────────────────────────────────────────
  const virtualItems = virtualizer.getVirtualItems()
  const lastIndex = virtualItems.length > 0 ? virtualItems[virtualItems.length - 1].index : -1

  // onLoadMore 是 Shell 里的普通函数声明，每次渲染都是新引用；放进依赖数组会让下面的
  // effect 每渲染必跑（和「仅依赖最末可见行索引」的本意相反）。用 ref 固定住。
  const onLoadMoreRef = useRef(onLoadMore)
  useEffect(() => {
    onLoadMoreRef.current = onLoadMore
  })

  // 仅依赖最末可见行索引，避免每帧滚动都重跑（virtualItems 每次都是新数组引用）
  useEffect(() => {
    const go = shouldLoadMore({
      lastIndex,
      rowCount: rows.length,
      messageCount: itemCount,
      lastLoadedCount: loadedLenRef.current,
      hasNextPage,
      isFetchingNextPage,
      nextPageError: nextPageError ?? false,
    })
    if (!go) return
    loadedLenRef.current = itemCount
    onLoadMoreRef.current()
  }, [lastIndex, rows.length, itemCount, hasNextPage, isFetchingNextPage, nextPageError])

  // ── 批量选择派生状态 ──────────────────────────────────────────────────────
  const selectedCount = threadMode ? selectedThreadIds.size : selectedIds.size
  const allVisibleSelected = threadMode
    ? threads.length > 0 && threads.every((th) => selectedThreadIds.has(th.thread_id))
    : filtered.length > 0 && filtered.every((m) => selectedIds.has(m.id))
  // 选择态 = 偏好设为常显、手动开了选择模式，或已有选中项
  const selecting = alwaysShowSelect || selectMode || selectedCount > 0

  function exitSelectMode() {
    setSelectMode(false)
    // 菜单开着时清空选中会让 trigger 变 disabled，radix 不会因此关闭它
    setBatchMoveOpen(false)
    onClearSelection()
  }

  function toggleSelectMode() {
    if (selecting) exitSelectMode()
    else setSelectMode(true)
  }

  // ── 渲染 ─────────────────────────────────────────────────────────────────
  return (
    <div className="flex h-full flex-col">

      {/* 常驻的播报区。必须常驻：上一轮的教训是 live region 与内容同时插入 DOM 时
          读屏不播报。底部那条「加载失败 · 重试」是翻页失败后唯一的出口，
          读屏用户滚到底不会看到它，只能靠这里说出来。 */}
      <div className="sr-only" role="status">
        {nextPageError ? t('list.loadMoreFailed') : ''}
      </div>

      {/* ── 顶部标题栏：标题占固定宽度，搜索框紧随其后（位置不随标题长短抖动）── */}
      <div className="list-head">
        {/* 后台刷新：贴在标题栏下沿的细进度条。放这里而不是列表里，是因为刷新期间
            列表显示的仍是旧数据，指示必须在内容之外、且不占布局（否则每次轮询都抖一下）。*/}
        {showRefresh && <div className="list-refresh-bar" aria-hidden="true" />}
        <div className="title-wrap">
          {/* 用 role + aria-level 而不是换成 <h2>：后者会带进 UA 的默认 margin /
              font-size，.list-title 那套 font-display / 20px 得再重置一遍。
              属性路线零样式风险。分组标题是 level 3，这里是它们的上一级。 */}
          <div className="list-title" role="heading" aria-level={2}>{title}</div>
          {showSub && (
            <div className="list-sub">{subLabel}</div>
          )}
        </div>

        <div className="search-input">
          <Icon name="search" size={14} />
          <input
            ref={searchInputRef}
            type="text"
            placeholder={t('list.searchPlaceholder')}
            value={query}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label={t('list.search')}
          />

          {/* 语法帮助：限定符不写出来用户不会知道它们存在 */}
          <SearchSyntaxHelp onPick={appendSyntax} />

          {query ? (
            /* 有输入时显示清除按钮 */
            <button
              type="button"
              className="icon-btn mini"
              onClick={() => onSearchChange('')}
              aria-label={t('list.clearSearch')}
            >
              <Icon name="close" size={12} />
            </button>
          ) : (
            /* 无输入时显示快捷键提示（按平台显示 ⌘K / Ctrl K）*/
            <span className="kbd">{searchShortcutHint()}</span>
          )}
        </div>
      </div>

      {/* ── 统一工具栏：选择开关 + 筛选 + 批量操作同处一条 44px ──
           筛选 chips 常驻，选择态只是在同一行追加操作组（两者并存而非互斥），
           高度恒定，列表不会因为进入选择而跳动。 */}
      <div className="list-toolbar">
        {/* 选择模式开关（偏好设为「始终显示选择框」时无需此开关）*/}
        {!alwaysShowSelect && (
          <button
            type="button"
            className={'lt-btn' + (selecting ? ' active' : '')}
            onClick={toggleSelectMode}
            title={selecting ? t('list.exitSelect') : t('list.selectMode')}
            aria-label={selecting ? t('list.exitSelect') : t('list.selectMode')}
            aria-pressed={selecting}
          >
            <Icon name="check" size={16} />
          </button>
        )}

        {/* 筛选 chips：不随选择态消失。
             每个 chip 是独立开关而非三选一——「未读 + 有附件」是一次真实的检索意图，
             互斥单选表达不了。条件送到后端参与查询，不是对已加载分页做前端过滤。
             「不筛选」由所有开关关闭表达，因此没有「全部」按钮。 */}
        <div className="lt-chips">
          {(
            [
              { id: 'unread',     label: t('list.filterUnread') },
              { id: 'flagged',    label: t('list.filterFlagged') },
              { id: 'attachment', label: t('list.filterAttachment') },
            ] as { id: FilterKey; label: string }[]
          ).map((c) => (
            <button
              key={c.id}
              type="button"
              className={'chip' + (filter[c.id] ? ' active' : '')}
              onClick={() => onToggleFilter(c.id)}
              aria-pressed={filter[c.id]}
            >
              {c.label}
            </button>
          ))}
          {/* 一键清空所有筛选：开关多了之后逐个点回去很烦，
              且能明确告诉用户「列表为什么是空的」有个出口。 */}
          {filterActive && (
            <button
              type="button"
              className="chip chip-clear"
              onClick={onClearFilter}
              title={t('list.clearFilter')}
              aria-label={t('list.clearFilter')}
            >
              <Icon name="close" size={11} />
            </button>
          )}
        </div>

        {/* 选择区：进入选择态后在同一行追加，紧跟筛选之后（不右对齐，位置稳定）*/}
        {selecting && (
          <>
            <span className="lt-sep" />

            {/* 全选/取消全选当前可见 */}
            <label className="lt-all" title={t('list.selectAll')}>
              <input
                type="checkbox"
                checked={allVisibleSelected}
                onChange={() => (allVisibleSelected ? onClearSelection() : onSelectAllVisible())}
                aria-label={t('list.selectAll')}
              />
            </label>
            {selectedCount > 0 && (
              <span className="lt-count">{t('list.selectedCount', { count: selectedCount })}</span>
            )}

            <div className="lt-actions">
              <button
                type="button" className="lt-btn" disabled={selectedCount === 0}
                onClick={() => onBatchRead(true)}
                title={t('list.batchRead')} aria-label={t('list.batchRead')}
              >
                <Icon name="check" size={16} />
              </button>
              <button
                type="button" className="lt-btn" disabled={selectedCount === 0}
                onClick={() => onBatchRead(false)}
                title={t('list.batchUnread')} aria-label={t('list.batchUnread')}
              >
                <Icon name="mail" size={16} />
              </button>
              <button
                type="button" className="lt-btn" disabled={selectedCount === 0}
                onClick={() => onBatchFlag(true)}
                title={t('list.batchFlag')} aria-label={t('list.batchFlag')}
              >
                <Icon name="star" size={16} />
              </button>

              {/* 移动下拉（跨账户/无目标时禁用）。
                  原先是手搓的：自绘遮罩层接关闭、硬编码 boxShadow（不是
                  var(--shadow-md)）、靠 onMouseEnter/Leave 手改 style.background
                  模拟 hover，于是同一个应用里两种菜单外观、且这一种没有键盘操作、
                  没有焦点管理、Esc 关不掉。DropMenu 与右键菜单共用
                  .ctx-menu / .ctx-item 与 CtxMenuItem 模型，radix 负责其余。 */}
              <DropMenu
                align="start"
                open={batchMoveOpen}
                onOpenChange={setBatchMoveOpen}
                trigger={
                  <button
                    type="button"
                    className="lt-btn"
                    disabled={selectedCount === 0 || moveTargets.length === 0}
                    title={moveTargets.length === 0 ? t('list.batchMoveDisabled') : t('list.batchMove')}
                    aria-label={t('list.batchMove')}
                  >
                    <Icon name="folder" size={16} />
                  </button>
                }
                items={moveTargets
                  .map((f) => ({
                    key: String(f.id),
                    label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
                    icon: 'folder' as const,
                    onSelect: () => onBatchMove(f.id),
                  }))}
              />

              <button
                type="button" className="lt-btn danger" disabled={selectedCount === 0}
                onClick={onBatchDelete}
                title={t('list.batchDelete')} aria-label={t('list.batchDelete')}
              >
                <Icon name="trash" size={16} />
              </button>
              {/* 始终显示选择框时没有「退出」可言，退化为清空选择 */}
              <button
                type="button" className="lt-btn"
                onClick={exitSelectMode}
                disabled={alwaysShowSelect && selectedCount === 0}
                title={alwaysShowSelect ? t('list.clearSelection') : t('list.exitSelect')}
                aria-label={alwaysShowSelect ? t('list.clearSelection') : t('list.exitSelect')}
              >
                <Icon name="close" size={16} />
              </button>
            </div>
          </>
        )}
      </div>

      {/* ── 列表区域（选择模式下 selecting 让每行显出复选框列）──
           外面这层不滚动，只为给列宽手柄一个定位参照——手柄若放进滚动容器里
           会跟着列表一起滚走。 */}
      <div className="mail-list-wrap" ref={listWrapRef}>

      {/* 发件人/主题分界线上的拖拽手柄：平时透明，hover 才浮现。
          仅紧凑样式有列的概念；卡片样式是三行堆叠，没有列宽可调。
          left 用 calc 跟着 --sender-col-w 走，不靠 JS 定位，
          避免拖拽时手柄与列边界脱节。
          基准 = 行 padding-left(20) + 头像列(28) + gap(14) [+ 选择模式的 20+14]，
          再加半个 gap(7) 落到两列正中，最后减半个手柄宽(4.5)。 */}
      {/* ⚠ 列表为空时不要这个手柄：没有任何一行，也就没有「列」可言，
          而它照样吃 hover、照样能拖——鼠标划过空列表会冒出一条可拖动的分界线，
          拖它还会真的改掉列宽，用户完全不知道自己在调什么。
          窄屏同理（那里的行是堆叠的，没有列），由 CSS 隐藏。 */}
      {listStyle === 'compact' && rows.length > 0 && (
        <ResizeHandle
          className="list-col-resize"
          label={t('settings.page.senderColWidth')}
          value={senderCol}
          min={LAYOUT_LIMITS.senderCol.min}
          max={LAYOUT_LIMITS.senderCol.max}
          onDelta={resizeSenderCol}
          onJump={(to) =>
            setSenderCol(
              to === 'min' ? LAYOUT_LIMITS.senderCol.min : LAYOUT_LIMITS.senderCol.max,
            )
          }
          style={{ left: `calc(${selecting ? 96 : 62}px + var(--sender-col-w, 150px) + 2.5px)` }}
        />
      )}

      <div ref={scrollRef} className={'mail-list' + (selecting ? ' selecting' : '')}>

        {/* 首屏加载骨架 */}
        {loading && <SkeletonList />}

        {/* 错误态。排在空态之前：请求失败时 itemCount 同样是 0，
            先判错误才不会把故障渲染成「这里什么都没有」。
            但**只在没有内容可显示时**接管屏幕：react-query 的 status 是整个 query 的，
            翻页失败、后台重取失败都会让 error 非空而已加载的页还在缓存里，
            那时掀掉整张列表比不报错更糟（见 lib/query-semantics.test.ts）。 */}
        {!loading && error != null && itemCount === 0 && (
          <div className="list-error">
            <div className="list-error-title">{t('list.loadErrorTitle')}</div>
            <div>{t('list.loadErrorHint')}</div>
            {errorText(error) && <div className="list-error-detail">{errorText(error)}</div>}
            {onRetry && (
              <button type="button" className="pill-btn" onClick={onRetry} style={{ marginTop: 14 }}>
                {t('app.retry')}
              </button>
            )}
          </div>
        )}

        {/* 空态：无搜索/过滤结果 */}
        {!loading && error == null && itemCount === 0 && (
          <div
            style={{ padding: '48px 20px', textAlign: 'center', color: 'var(--ink-3)', fontSize: 13 }}
          >
            <div
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: 18,
                color: 'var(--ink-2)',
                marginBottom: 4,
              }}
            >
              {noAccounts ? t('list.noAccountsTitle') : t('list.nothingHere')}
            </div>
            <div>
              {noAccounts
                ? t('list.noAccountsHint')
                : query
                  ? t('list.searchNoResult')
                  : filterActive
                    ? t('list.filterNoResult')
                    : t('list.noMessages')}
            </div>

            {/* 空态得给出口。新用户唯一该做的事就是添加账户，把它摆在他正看着的地方。 */}
            {noAccounts && onAddAccount && (
              <button
                type="button"
                className="pill-btn primary"
                onClick={onAddAccount}
                style={{ marginTop: 14 }}
              >
                {t('list.addAccountCta')}
              </button>
            )}

            {/* 筛选筛空了同理：让他一键退回去，而不是自己去找那几个 chip */}
            {!noAccounts && !query && filterActive && (
              <button
                type="button"
                className="pill-btn"
                onClick={onClearFilter}
                style={{ marginTop: 14 }}
              >
                {t('list.clearFilter')}
              </button>
            )}

            {/* 本地一无所获时，服务端兜底是唯一还能走的路，所以直接摆在空态里 */}
            {searching && (
              <div style={{ marginTop: 16 }}>
                <div style={{ marginBottom: 10 }}>{t('list.remoteSearch.emptyHint')}</div>
                <RemoteSearchButton q={query} />
              </div>
            )}
          </div>
        )}

        {/* 虚拟化列表。role="list" + 每行显式的 posinset / setsize 是虚拟化列表
            唯一能让读屏说对「第几项，共几项」的方式——DOM 里只有视口内那十几行。 */}
        {!loading && rows.length > 0 && (
          <div
            ref={rowsRef}
            className="mail-rows"
            role="list"
            aria-label={threadMode ? t('list.ariaThreadList') : t('list.ariaList')}
            onKeyDown={onRowsKeyDown}
            // 键盘进入列表（Tab）也要算「焦点在列表里」；鼠标那条路由上面的
            // pointerdown 监听负责，两者互不冲突。
            onFocus={() => {
              focusInListRef.current = true
            }}
            style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}
          >
            {virtualItems.map((vItem) => {
              const row = rows[vItem.index]
              if (!row) return null

              // 右键菜单项（仅邮件行）：已读/星标/移动/删除。
              // 「移动到」目标 = 当前账户文件夹中与该邮件同账户的其他可选文件夹
              // （聚合视图里其他账户的邮件不展示移动项）。
              // 会话行的右键菜单打在整条会话上，走 thread batch 接口
              const threadMenuItems: CtxMenuItem[] = row.type === 'thread'
                ? (() => {
                    const th = row.item
                    const targets = folders.filter(
                      (f) => f.account_id === th.account_id && f.selectable,
                    )
                    // 会话里既有已读又有未读时，两个方向都要给。
                    // 只按「有没有未读」给一项的话，混合状态下永远只剩「全部标为
                    // 已读」，想把整条会话重新标成未读就没有入口了；反过来，
                    // 一条全已读的会话又只能标未读。两种都是用户真实会做的操作。
                    const mixed = th.unread > 0 && th.unread < th.count
                    const items: CtxMenuItem[] = []
                    if (mixed) {
                      items.push(
                        {
                          key: 'read',
                          label: t('list.thread.readAll'),
                          icon: 'mail',
                          onSelect: () => onMarkReadThread(th, true),
                        },
                        {
                          key: 'unread',
                          label: t('list.thread.unreadAll'),
                          icon: 'mail',
                          onSelect: () => onMarkReadThread(th, false),
                        },
                      )
                    } else {
                      items.push({
                        key: 'read',
                        label: th.unread > 0 ? t('list.thread.readAll') : t('list.thread.unreadAll'),
                        icon: 'mail',
                        onSelect: () => onMarkReadThread(th, th.unread > 0),
                      })
                    }
                    items.push({
                      key: 'flag',
                      label: th.flagged ? t('list.thread.unstarAll') : t('list.thread.starAll'),
                      icon: th.flagged ? 'star-fill' : 'star',
                      onSelect: () => onToggleFlagThread(th, !th.flagged),
                    })
                    if (targets.length > 0) {
                      items.push({
                        key: 'move',
                        label: t('list.thread.moveAll'),
                        icon: 'folder',
                        children: targets.map((f) => ({
                          key: `mv-${f.id}`,
                          label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
                          onSelect: () => onMoveThread(th, f.id),
                        })),
                      })
                    }
                    // 屏蔽对象取第一个不是本人的参与者（与会话行头像同一口径）：
                    // participants[0] 在自己发起的会话里永远是自己，直接用会把自己拉黑。
                    // 会话行没有携带「最新一封的发件人」，这是现有数据里最接近对方的那个人。
                    const threadSender = blockableAddr(pickAvatarParticipant(th.participants, selfAddrs)?.email)
                    if (threadSender) {
                      items.push({
                        key: 'block',
                        label: t('ctx.blockSender', { addr: threadSender }),
                        icon: 'shield',
                        onSelect: () => blockSender(threadSender),
                      })
                    }
                    items.push({ key: 'sep', separator: true })
                    items.push({
                      key: 'del',
                      label: t('list.thread.delete'),
                      icon: 'trash',
                      destructive: true,
                      onSelect: () => onDeleteThread(th),
                    })
                    return items
                  })()
                : []

              const menuItems: CtxMenuItem[] = row.type === 'item'
                ? (() => {
                    const msg = row.msg
                    const targets = folders.filter(
                      (f) => f.account_id === msg.account_id && f.id !== msg.folder_id && f.selectable,
                    )
                    const items: CtxMenuItem[] = [
                      {
                        key: 'read',
                        label: msg.seen ? t('ctx.markUnread') : t('ctx.markRead'),
                        icon: 'mail',
                        onSelect: () => onMarkRead(msg.id, !msg.seen),
                      },
                      {
                        key: 'flag',
                        label: msg.flagged ? t('ctx.unstar') : t('ctx.star'),
                        icon: msg.flagged ? 'star-fill' : 'star',
                        onSelect: () => onToggleFlag(msg.id, !msg.flagged),
                      },
                    ]
                    if (targets.length > 0) {
                      items.push({
                        key: 'move',
                        label: t('ctx.moveTo'),
                        icon: 'folder',
                        children: targets.map((f) => ({
                          key: `mv-${f.id}`,
                          label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
                          onSelect: () => onMoveMessage(msg.id, f.id),
                        })),
                      })
                    }
                    const msgSender = blockableAddr(msg.from_addr)
                    if (msgSender) {
                      items.push({
                        key: 'block',
                        label: t('ctx.blockSender', { addr: msgSender }),
                        icon: 'shield',
                        onSelect: () => blockSender(msgSender),
                      })
                    }
                    items.push({ key: 'sep', separator: true })
                    items.push({
                      key: 'del',
                      label: t('ctx.delete'),
                      icon: 'trash',
                      destructive: true,
                      onSelect: () => onDeleteMessage(msg.id),
                    })
                    return items
                  })()
                : []

              const positioned = (
                <div
                  key={vItem.key}
                  data-index={vItem.index}
                  // 分组标题行也是 listitem，内部再放 heading。
                  //
                  // 先前用的是 role="presentation"：它只移除该元素本身、不移除子树，
                  // 于是 heading 在可访问性树里成了 list 的直接子元素——而 ARIA 1.2 规定
                  // list 的 required owned element 只能是 listitem（或 group）。
                  // 那是一处确凿的违例，各家读屏对「list 里混进非 listitem」的规整策略不一致，
                  // 有丢掉整个列表语义的先例。listitem > heading 则完全合法。
                  // 计数不受影响：条目的「第几项、共几项」靠下面显式的 posinset/setsize，
                  // 不靠读屏去数 DOM。
                  role="listitem"
                  aria-posinset={row.type === 'header' ? undefined : row.pos}
                  aria-setsize={row.type === 'header' ? undefined : rowItemCount}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    height: `${vItem.size}px`,
                    transform: `translateY(${vItem.start}px)`,
                  }}
                >
                  {row.type === 'header' ? (
                    /* 日期分组标题行（两种列表样式都会分组）*/
                    <div
                      className="side-section-label"
                      role="heading"
                      aria-level={3}
                      style={{
                        padding: '4px 16px',
                        fontSize: 11,
                        fontWeight: 500,
                        color: 'var(--ink-3)',
                        background: 'var(--bg-alt)',
                        borderBottom: '1px solid var(--rule)',
                        height: '100%',
                        display: 'flex',
                        alignItems: 'center',
                      }}
                    >
                      {row.label}
                    </div>
                  ) : row.type === 'thread' ? (
                    <ThreadRow
                      item={row.item}
                      active={row.item.thread_id === activeThreadId}
                      rovingTab={row.pos === rovingPos}
                      lang={lang}
                      listStyle={listStyle}
                      selected={selectedThreadIds.has(row.item.thread_id)}
                      terms={highlightTerms}
                      selfAddrs={selfAddrs}
                      onSelect={(e) => handleThreadClick(row.item, e)}
                      onToggleSelect={() => onToggleSelectThread(row.item.thread_id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlagThread(row.item, !row.item.flagged)
                      }}
                      onDelete={() => onDeleteThread(row.item)}
                    />
                  ) : listStyle === 'compact' ? (
                    <CompactRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      rovingTab={row.pos === rovingPos}
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      terms={highlightTerms}
                      onSelect={(e) => handleMessageClick(row.msg.id, e)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                      onDelete={() => onDeleteMessage(row.msg.id)}
                      acctColor={acctColorOf?.(row.msg.account_id) ?? null}
                    />
                  ) : (
                    <CardRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      rovingTab={row.pos === rovingPos}
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      terms={highlightTerms}
                      onSelect={(e) => handleMessageClick(row.msg.id, e)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                      onDelete={() => onDeleteMessage(row.msg.id)}
                      acctColor={acctColorOf?.(row.msg.account_id) ?? null}
                    />
                  )}
                </div>
              )

              // 邮件行 / 会话行都包一层右键菜单；分组标题行原样返回
              if (row.type === 'item') {
                return <CtxMenu key={vItem.key} items={menuItems} trigger={positioned} />
              }
              if (row.type === 'thread') {
                return <CtxMenu key={vItem.key} items={threadMenuItems} trigger={positioned} />
              }
              return positioned
            })}
          </div>
        )}

        {/* 底部加载状态。失败态排在「没有更多」之前：两者都表现为列表不再增长，
            但一个是到头了、一个是出错了，混在一起就是把故障谎报成数据的尽头。

            ⚠ 外层的条件必须把「三种状态都不成立」也排掉。原先里面渲染 null
            而 .list-foot 照常挂着它那 12px 上下内边距——还有下一页、正准备
            自动加载的那段时间里，列表底下恒挂一条空白，看起来像「还有一行没
            加载出来」。空的容器不是空的。 */}
        {!loading && itemCount > 0 && (isFetchingNextPage || nextPageError || !hasNextPage) && (
          <div className="list-foot">
            {isFetchingNextPage ? (
              t('list.loadingMore')
            ) : nextPageError ? (
              <button
                type="button"
                className="list-foot-retry"
                onClick={() => onRetryNextPage?.()}
              >
                <span>{t('list.loadMoreFailed')}</span>
                <span className="list-foot-retry-cta">{t('app.retry')}</span>
              </button>
            ) : (
              t('list.noMore')
            )}
          </div>
        )}

        {/* 服务端兜底搜索：只在翻到底之后出现——还有下一页时，
            用户该做的是继续加载本地结果，不是花 90 秒去连 IMAP。 */}
        {!loading && searching && itemCount > 0 && !hasNextPage && !isFetchingNextPage && (
          <div className="remote-search-foot">
            <RemoteSearchButton q={query} />
          </div>
        )}
      </div>
      </div>
    </div>
  )
}

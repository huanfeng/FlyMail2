import { useRef, useEffect, useCallback, useMemo, useState } from 'react'
import { shouldLoadMore } from '@/lib/list-guards'
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
import { highlightText } from '@/components/ui/Highlight'
import { SearchSyntaxHelp } from '@/components/mail/SearchSyntaxHelp'
import { RemoteSearchButton } from '@/components/mail/RemoteSearchButton'
import { extractHighlightTerms } from '@/lib/search-terms'
import { CtxMenu, type CtxMenuItem } from '@/components/ui/ContextMenu'
import { useToast } from '@/components/ui/Toast'
import { apiErrorMessage } from '@/lib/api'
import { useAddBlock } from '@/lib/queries'
import { isValidBlockPattern, normalizeBlockPattern } from '@/lib/rules'
import { FOCUS_SEARCH_EVENT } from '@/hooks/useKeyboardShortcuts'
import { searchShortcutHint } from '@/lib/platform'

// ─────────────────────────────────────────────────────────────────────────────
// 类型定义
// ─────────────────────────────────────────────────────────────────────────────

/** 虚拟化行模型：分组标题 / 单封邮件 / 会话（两种条目互斥，由 threads 是否为 null 决定） */
type RowItem =
  | { type: 'header'; label: string }
  | { type: 'item'; msg: MessageListItem }
  | { type: 'thread'; item: ThreadListItem }

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
  activeMessageId: number | null
  onSelectMessage: (id: number) => void
  onToggleFlag: (id: number, flagged: boolean) => void
  listStyle: ListStyle
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onLoadMore: () => void
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
  onSelectAllVisible: () => void
  onClearSelection: () => void
  onBatchRead: (read: boolean) => void
  onBatchFlag: (flagged: boolean) => void
  onBatchDelete: () => void
  onBatchMove: (folderId: number) => void
  /** 批量移动目标（已选邮件共同账户的文件夹；跨账户/为空时禁用移动） */
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

function SelectBox({ checked, onToggle }: { checked: boolean; onToggle: () => void }) {
  // 用 <label> 包裹原生 checkbox：点击整块都可靠切换（label 原生联动 input → onChange），
  // label 上 stopPropagation 阻止冒泡到行（避免误打开邮件）。
  return (
    <label className="mi-select" onClick={(e) => e.stopPropagation()}>
      <input
        type="checkbox"
        checked={checked}
        onChange={onToggle}
        aria-label="select"
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
  onSelect: () => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
  onDelete: () => void
}

function CardRow({ msg, active, lang, selected, terms, onSelect, onToggleSelect, onToggleFlag, onDelete }: CardRowProps) {
  const { t } = useTranslation()
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={0}
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
      <SelectBox checked={selected} onToggle={onToggleSelect} />

      {/* 方形头像 */}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
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
          {msg.subject ? highlightText(msg.subject, terms) : '—'}
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
        aria-label={msg.flagged ? 'Unstar' : 'Star'}
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
  msg: MessageListItem
  active: boolean
  lang: string
  selected: boolean
  /** 搜索命中词（非搜索态为空数组，高亮函数会原样返回字符串） */
  terms: string[]
  onSelect: () => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
  onDelete: () => void
}

function CompactRow({ msg, active, lang, selected, terms, onSelect, onToggleSelect, onToggleFlag, onDelete }: CompactRowProps) {
  const { t } = useTranslation()
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={0}
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
      <SelectBox checked={selected} onToggle={onToggleSelect} />

      {/* 方形头像（小）*/}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
        </div>
      </span>

      {/* 发件人列 */}
      <div className="mi-top">
        <span className="mi-sender">{highlightText(msg.from_name || msg.from_addr, terms)}</span>
      </div>

      {/* 主题 + 摘要（单行，"— " 由 CSS ::before 注入）*/}
      <div className="mi-subject-preview">
        <span className="mi-subject">{msg.subject ? highlightText(msg.subject, terms) : '—'}</span>
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
          aria-label={msg.flagged ? 'Unstar' : 'Star'}
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
  item: ThreadListItem
  active: boolean
  lang: string
  selected: boolean
  listStyle: ListStyle
  /** 搜索命中词（非搜索态为空数组，高亮函数会原样返回字符串） */
  terms: string[]
  /** 本人邮箱集合（已小写），用于头像避开自己 */
  selfAddrs: Set<string>
  onSelect: () => void
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
      tabIndex={0}
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
      <SelectBox checked={selected} onToggle={onToggleSelect} />

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
            <span className="mi-subject">{item.subject ? highlightText(item.subject, terms) : '—'}</span>
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
              aria-label={item.flagged ? 'Unstar' : 'Star'}
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
              {item.subject ? highlightText(item.subject, terms) : '—'}
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
            aria-label={item.flagged ? 'Unstar' : 'Star'}
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
  activeMessageId,
  onSelectMessage,
  onToggleFlag,
  listStyle,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
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

  // ── 批量移动下拉开关 ───────────────────────────────────────────────────────
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
  function onSenderResizeDown(e: React.PointerEvent<HTMLDivElement>) {
    e.preventDefault()
    const el = e.currentTarget
    el.setPointerCapture(e.pointerId)
    el.classList.add('dragging')
    document.body.classList.add('is-resizing')

    let lastX = e.clientX
    // 用局部变量跟踪当前值：setState 是异步的，闭包里读 senderCol 会拿到陈旧值
    let cur = senderCol

    function onMove(ev: PointerEvent) {
      const dx = ev.clientX - lastX
      lastX = ev.clientX
      cur = clampWidth(cur + dx, LAYOUT_LIMITS.senderCol.min, LAYOUT_LIMITS.senderCol.max)
      setSenderCol(cur)
      document.documentElement.style.setProperty('--sender-col-w', `${cur}px`)
    }
    function onUp(ev: PointerEvent) {
      el.releasePointerCapture(ev.pointerId)
      el.classList.remove('dragging')
      document.body.classList.remove('is-resizing')
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointercancel', onUp)
      // 落盘并广播，让设置里的滑块同步到新值
      saveLayoutWidths({ ...loadLayoutWidths(), senderCol: cur })
    }
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointercancel', onUp)
  }

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
    if (threadMode) {
      for (const group of groupByDate(threads, (th) => th.date, undefined, groupLabel)) {
        result.push({ type: 'header', label: group.label })
        for (const item of group.items) result.push({ type: 'thread', item })
      }
      return result
    }
    for (const group of groupByDate(filtered, (m) => m.date, undefined, groupLabel)) {
      result.push({ type: 'header', label: group.label })
      for (const msg of group.items) result.push({ type: 'item', msg })
    }
    return result
  })()

  // ── 行高 ─────────────────────────────────────────────────────────────────
  // ⚠ 这不是"估算"而是硬契约：没有接 measureElement，虚拟列表就完全按这里的数值
  // 用 translateY 排布行槽位。行的真实高度一旦超出，就会压到下一行头上，
  // 表现为 hover / 选中高亮与行边界错位重叠。
  // index.css 用 .mail-item{height:100%} 让行严格填满槽位，两边必须同步改。
  // 卡片行 105 = 28(padding) + 18(发件人) + 2+19(主题) + 2+35(两行摘要) + 1(border)
  // 紧凑行  44 = 22(padding) + 21(单行内容) + 1(border)
  const estimateSize = useCallback(
    (index: number): number => {
      const row = rows[index]
      if (!row) return 52
      if (row.type === 'header') return 28
      // 会话行与单封行共用同一套 class 与内部结构，高度自然相同
      return listStyle === 'compact' ? 44 : 105
    },
    [rows, listStyle],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize,
    overscan: 5,
  })

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
    })
    if (!go) return
    loadedLenRef.current = itemCount
    onLoadMoreRef.current()
  }, [lastIndex, rows.length, itemCount, hasNextPage, isFetchingNextPage])

  // ── 批量选择派生状态 ──────────────────────────────────────────────────────
  const selectedCount = threadMode ? selectedThreadIds.size : selectedIds.size
  const allVisibleSelected = threadMode
    ? threads.length > 0 && threads.every((th) => selectedThreadIds.has(th.thread_id))
    : filtered.length > 0 && filtered.every((m) => selectedIds.has(m.id))
  // 选择态 = 偏好设为常显、手动开了选择模式，或已有选中项
  const selecting = alwaysShowSelect || selectMode || selectedCount > 0

  function exitSelectMode() {
    setSelectMode(false)
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

      {/* ── 顶部标题栏：标题占固定宽度，搜索框紧随其后（位置不随标题长短抖动）── */}
      <div className="list-head">
        <div className="title-wrap">
          <div className="list-title">{title}</div>
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
              className="icon-btn"
              style={{ width: 20, height: 20 }}
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

              {/* 移动下拉（跨账户/无目标时禁用）*/}
              <div style={{ position: 'relative' }}>
                <button
                  type="button"
                  className="lt-btn"
                  onClick={() => setBatchMoveOpen((o) => !o)}
                  disabled={selectedCount === 0 || moveTargets.length === 0}
                  title={moveTargets.length === 0 ? t('list.batchMoveDisabled') : t('list.batchMove')}
                  aria-label={t('list.batchMove')}
                >
                  <Icon name="folder" size={16} />
                </button>
                {batchMoveOpen && moveTargets.length > 0 && (
                  <>
                    <div onClick={() => setBatchMoveOpen(false)} style={{ position: 'fixed', inset: 0, zIndex: 40 }} />
                    <div
                      style={{
                        position: 'absolute', top: '100%', left: 0, marginTop: 4, zIndex: 41,
                        minWidth: 170, maxHeight: 280, overflowY: 'auto',
                        background: 'var(--surface)', border: '1px solid var(--rule)',
                        borderRadius: 8, boxShadow: '0 8px 24px rgba(0,0,0,0.12)', padding: 4,
                      }}
                    >
                      {moveTargets
                        .filter((f) => f.selectable)
                        .map((f) => (
                          <button
                            key={f.id}
                            type="button"
                            onClick={() => { setBatchMoveOpen(false); onBatchMove(f.id) }}
                            style={{
                              display: 'flex', alignItems: 'center', gap: 8, width: '100%',
                              padding: '7px 10px', border: 'none', background: 'transparent',
                              borderRadius: 6, fontSize: 13, color: 'var(--ink)', cursor: 'pointer', textAlign: 'left',
                            }}
                            onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-alt)' }}
                            onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                          >
                            <Icon name="folder" size={13} />
                            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              {f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)}
                            </span>
                          </button>
                        ))}
                    </div>
                  </>
                )}
              </div>

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
      <div className="mail-list-wrap">

      {/* 发件人/主题分界线上的拖拽手柄：平时透明，hover 才浮现。
          仅紧凑样式有列的概念；卡片样式是三行堆叠，没有列宽可调。
          left 用 calc 跟着 --sender-col-w 走，不靠 JS 定位，
          避免拖拽时手柄与列边界脱节。
          基准 = 行 padding-left(20) + 头像列(28) + gap(14) [+ 选择模式的 20+14]，
          再加半个 gap(7) 落到两列正中，最后减半个手柄宽(4.5)。 */}
      {listStyle === 'compact' && (
        <div
          className="list-col-resize"
          role="separator"
          aria-orientation="vertical"
          aria-label={t('settings.page.senderColWidth')}
          style={{ left: `calc(${selecting ? 96 : 62}px + var(--sender-col-w, 150px) + 2.5px)` }}
          onPointerDown={onSenderResizeDown}
        />
      )}

      <div ref={scrollRef} className={'mail-list' + (selecting ? ' selecting' : '')}>

        {/* 首屏加载骨架 */}
        {loading && <SkeletonList />}

        {/* 空态：无搜索/过滤结果 */}
        {!loading && itemCount === 0 && (
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
              {t('list.nothingHere')}
            </div>
            <div>
              {query
                ? t('list.searchNoResult')
                : filterActive
                  ? t('list.filterNoResult')
                  : t('list.noMessages')}
            </div>

            {/* 本地一无所获时，服务端兜底是唯一还能走的路，所以直接摆在空态里 */}
            {searching && (
              <div style={{ marginTop: 16 }}>
                <div style={{ marginBottom: 10 }}>{t('list.remoteSearch.emptyHint')}</div>
                <RemoteSearchButton q={query} />
              </div>
            )}
          </div>
        )}

        {/* 虚拟化列表 */}
        {!loading && rows.length > 0 && (
          <div style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}>
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
                    const items: CtxMenuItem[] = [
                      {
                        key: 'read',
                        label: th.unread > 0 ? t('list.thread.readAll') : t('list.thread.unreadAll'),
                        icon: 'mail',
                        onSelect: () => onMarkReadThread(th, th.unread > 0),
                      },
                      {
                        key: 'flag',
                        label: th.flagged ? t('list.thread.unstarAll') : t('list.thread.starAll'),
                        icon: th.flagged ? 'star-fill' : 'star',
                        onSelect: () => onToggleFlagThread(th, !th.flagged),
                      },
                    ]
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
                    /* 分组标题行（compact 模式专属）*/
                    <div
                      className="side-section-label"
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
                      lang={lang}
                      listStyle={listStyle}
                      selected={selectedThreadIds.has(row.item.thread_id)}
                      terms={highlightTerms}
                      selfAddrs={selfAddrs}
                      onSelect={() => onSelectThread(row.item)}
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
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      terms={highlightTerms}
                      onSelect={() => onSelectMessage(row.msg.id)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                      onDelete={() => onDeleteMessage(row.msg.id)}
                    />
                  ) : (
                    <CardRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      terms={highlightTerms}
                      onSelect={() => onSelectMessage(row.msg.id)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                      onDelete={() => onDeleteMessage(row.msg.id)}
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

        {/* 底部加载状态 */}
        {!loading && itemCount > 0 && (
          <div
            style={{ padding: '12px 0', textAlign: 'center', fontSize: 12, color: 'var(--ink-3)' }}
          >
            {isFetchingNextPage
              ? t('list.loadingMore')
              : !hasNextPage
                ? t('list.noMore')
                : null}
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

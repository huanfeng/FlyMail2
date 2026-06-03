import { useRef, useEffect, useCallback, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { groupByDate } from '@/lib/date-group'
import type { ListStyle } from '@/lib/list-prefs'
import type { Folder, MessageListItem } from '@/lib/types'
import { Icon } from '@/components/ui/Icon'
import { FOCUS_SEARCH_EVENT } from '@/hooks/useKeyboardShortcuts'

// ─────────────────────────────────────────────────────────────────────────────
// 类型定义
// ─────────────────────────────────────────────────────────────────────────────

/** 虚拟化行模型：分组标题 或 邮件条目 */
type RowItem =
  | { type: 'header'; label: string }
  | { type: 'item'; msg: MessageListItem }

/** filter chip 过滤类型 */
type FilterType = 'all' | 'unread' | 'flagged'

interface Props {
  folder: Folder | null
  messages: MessageListItem[]
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
// 选择复选框：覆盖在头像上（hover/已选时显现），点击不触发打开邮件
// ─────────────────────────────────────────────────────────────────────────────

function SelectBox({ checked, onToggle }: { checked: boolean; onToggle: () => void }) {
  return (
    <span
      className="mi-select"
      onClick={(e) => { e.stopPropagation(); onToggle() }}
      role="presentation"
    >
      <input
        type="checkbox"
        checked={checked}
        onChange={() => { /* 选择由外层 span 的 onClick 处理 */ }}
        onClick={(e) => e.stopPropagation()}
        tabIndex={-1}
        aria-label="select"
      />
    </span>
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
  onSelect: () => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
}

function CardRow({ msg, active, lang, selected, onSelect, onToggleSelect, onToggleFlag }: CardRowProps) {
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onSelect() }}
      className={
        'mail-item' +
        (isUnread ? ' unread' : '') +
        (active ? ' selected' : '') +
        (selected ? ' batch-selected' : '')
      }
    >
      {/* 未读圆点（CSS 控制可见性）*/}
      <span className="mi-unread-dot" />

      {/* 方形头像（含选择复选框覆盖层）*/}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
        </div>
        <SelectBox checked={selected} onToggle={onToggleSelect} />
      </span>

      <div style={{ minWidth: 0 }}>
        {/* 第一行：发件人 + 时间 */}
        <div className="mi-top">
          <span className="mi-sender">{msg.from_name || msg.from_addr}</span>
          <span className="mi-time">{relTime(msg.date, lang)}</span>
        </div>

        {/* 第二行：主题 */}
        <div className="mi-subject">
          {msg.subject || '—'}
        </div>

        {/* 第三行：摘要（2 行截断由 CSS 控制）*/}
        {msg.snippet && (
          <div className="mi-preview">{msg.snippet}</div>
        )}

        {/* 标签行：附件 */}
        {msg.has_attachment && (
          <div className="mi-tags">
            <span className="mi-tag mi-attach">
              <Icon name="attach" size={10} />
            </span>
          </div>
        )}
      </div>

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
  onSelect: () => void
  onToggleSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
}

function CompactRow({ msg, active, lang, selected, onSelect, onToggleSelect, onToggleFlag }: CompactRowProps) {
  const isUnread = !msg.seen
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onSelect() }}
      className={
        'mail-item mail-item-row' +
        (isUnread ? ' unread' : '') +
        (active ? ' selected' : '') +
        (selected ? ' batch-selected' : '')
      }
    >
      {/* 未读圆点 */}
      <span className="mi-unread-dot" />

      {/* 方形头像（小，含选择复选框覆盖层）*/}
      <span className="mi-avatar-wrap">
        <div
          className="avatar-sq"
          style={{ background: 'var(--accent)' }}
        >
          {initials(msg.from_name, msg.from_addr)}
        </div>
        <SelectBox checked={selected} onToggle={onToggleSelect} />
      </span>

      {/* 发件人列 */}
      <div className="mi-top">
        <span className="mi-sender">{msg.from_name || msg.from_addr}</span>
      </div>

      {/* 主题 + 摘要（单行，"— " 由 CSS ::before 注入）*/}
      <div className="mi-subject-preview">
        <span className="mi-subject">{msg.subject || '—'}</span>
        {msg.snippet && (
          <span className="mi-preview">{msg.snippet}</span>
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

      {/* 星标按钮 */}
      <button
        type="button"
        className={'mi-star icon-btn' + (msg.flagged ? ' starred' : '')}
        onClick={onToggleFlag}
        aria-label={msg.flagged ? 'Unstar' : 'Star'}
        style={{ position: 'static', opacity: msg.flagged ? 1 : undefined }}
      >
        <Icon name={msg.flagged ? 'star-fill' : 'star'} size={14} />
      </button>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 主组件
// ─────────────────────────────────────────────────────────────────────────────

export function MailList({
  folder,
  messages,
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
}: Props) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const scrollRef = useRef<HTMLDivElement>(null)
  // 搜索框 ref：供快捷键 / 聚焦时使用
  const searchInputRef = useRef<HTMLInputElement>(null)

  // 搜索为受控值：由 Shell 管理并驱动后端跨账户搜索（searchValue/onSearchChange）。
  const query = searchValue

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

  // ── filter chips 状态 ─────────────────────────────────────────────────────
  const [filter, setFilter] = useState<FilterType>('all')

  // ── 批量移动下拉开关 ───────────────────────────────────────────────────────
  const [batchMoveOpen, setBatchMoveOpen] = useState(false)

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

  // ── 前端过滤：仅 chips（搜索已由后端完成，messages 即为结果集）──────────────
  const filtered = (() => {
    let list = messages
    if (filter === 'unread') list = list.filter((m) => !m.seen)
    if (filter === 'flagged') list = list.filter((m) => m.flagged)
    return list
  })()

  // ── 构造虚拟化行模型 ──────────────────────────────────────────────────────
  const rows: RowItem[] = (() => {
    if (listStyle === 'compact') {
      // 紧凑模式：按日期分组，插入分组 header 行
      const groups = groupByDate(filtered, (m) => m.date)
      const result: RowItem[] = []
      for (const group of groups) {
        result.push({ type: 'header', label: group.label })
        for (const msg of group.items) {
          result.push({ type: 'item', msg })
        }
      }
      return result
    }
    // 卡片模式：纯 item 列表，不分组
    return filtered.map((msg): RowItem => ({ type: 'item', msg }))
  })()

  // ── 行高估算 ─────────────────────────────────────────────────────────────
  const estimateSize = useCallback(
    (index: number): number => {
      const row = rows[index]
      if (!row) return 52
      if (row.type === 'header') return 28
      return listStyle === 'compact' ? 44 : 84
    },
    [rows, listStyle],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize,
    overscan: 5,
  })

  // 数据源/样式切换时重置滚动 + 重新测量（替代 React key 重挂载，避免打断搜索框焦点）。
  useEffect(() => {
    scrollRef.current?.scrollTo(0, 0)
    virtualizer.measure()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sourceKey])

  // ── 无限加载：接近底部时触发 ──────────────────────────────────────────────
  const virtualItems = virtualizer.getVirtualItems()
  const lastIndex = virtualItems.length > 0 ? virtualItems[virtualItems.length - 1].index : -1

  // 仅依赖最末可见行索引，避免每帧滚动都重跑（virtualItems 每次都是新数组引用）
  useEffect(() => {
    if (lastIndex >= rows.length - 5 && hasNextPage && !isFetchingNextPage) {
      onLoadMore()
    }
  }, [lastIndex, rows.length, hasNextPage, isFetchingNextPage, onLoadMore])

  // ── 批量选择派生状态 ──────────────────────────────────────────────────────
  const selectedCount = selectedIds.size
  const allVisibleSelected = filtered.length > 0 && filtered.every((m) => selectedIds.has(m.id))

  // ── 渲染 ─────────────────────────────────────────────────────────────────
  return (
    <div className="flex h-full flex-col">

      {/* ── 顶部标题栏 ── */}
      <div className="list-head">
        <div className="title-wrap">
          <div className="list-title">{title}</div>
          {showSub && (
            <div className="list-sub">{subLabel}</div>
          )}
        </div>
      </div>

      {/* ── 批量操作条（有选中时显示）── */}
      {selectedCount > 0 && (
        <div className="batch-bar">
          {/* 全选/取消全选 */}
          <input
            type="checkbox"
            checked={allVisibleSelected}
            onChange={() => (allVisibleSelected ? onClearSelection() : onSelectAllVisible())}
            style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--accent)' }}
            aria-label={t('list.selectAll')}
          />
          <span className="bb-count">{t('list.selectedCount', { count: selectedCount })}</span>

          <button type="button" className="bb-btn" onClick={() => onBatchRead(true)}>
            <Icon name="check" size={13} /> {t('list.batchRead')}
          </button>
          <button type="button" className="bb-btn" onClick={() => onBatchRead(false)}>
            {t('list.batchUnread')}
          </button>
          <button type="button" className="bb-btn" onClick={() => onBatchFlag(true)}>
            <Icon name="star" size={13} /> {t('list.batchFlag')}
          </button>

          {/* 移动下拉（跨账户/无目标时禁用）*/}
          <div style={{ position: 'relative' }}>
            <button
              type="button"
              className="bb-btn"
              onClick={() => setBatchMoveOpen((o) => !o)}
              disabled={moveTargets.length === 0}
              title={moveTargets.length === 0 ? t('list.batchMoveDisabled') : t('list.batchMove')}
            >
              <Icon name="folder" size={13} /> {t('list.batchMove')}
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

          <button type="button" className="bb-btn" onClick={onBatchDelete} style={{ color: 'var(--destructive)' }}>
            <Icon name="trash" size={13} /> {t('list.batchDelete')}
          </button>

          {/* 清除选择 */}
          <button type="button" className="bb-btn" onClick={onClearSelection} title={t('list.clearSelection')}>
            <Icon name="close" size={13} />
          </button>
        </div>
      )}

      {/* ── 搜索栏 ── */}
      <div className="search-bar">
        <div className="search-input">
          <Icon name="search" size={14} />
          <input
            ref={searchInputRef}
            type="text"
            placeholder={t('list.search')}
            value={query}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label={t('list.search')}
          />
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
            /* 无输入时显示快捷键提示 */
            <span className="kbd">⌘K</span>
          )}
        </div>
      </div>

      {/* ── filter chips ── */}
      <div className="filter-chips">
        {(
          [
            { id: 'all',     label: t('list.filterAll') },
            { id: 'unread',  label: t('list.filterUnread') },
            { id: 'flagged', label: t('list.filterFlagged') },
          ] as { id: FilterType; label: string }[]
        ).map((c) => (
          <button
            key={c.id}
            type="button"
            className={'chip' + (filter === c.id ? ' active' : '')}
            onClick={() => setFilter(c.id)}
          >
            {c.label}
          </button>
        ))}
      </div>

      {/* ── 列表区域 ── */}
      <div ref={scrollRef} className="mail-list">

        {/* 首屏加载骨架 */}
        {loading && <SkeletonList />}

        {/* 空态：无搜索/过滤结果 */}
        {!loading && filtered.length === 0 && (
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
              {query || filter !== 'all' ? t('list.searchNoResult') : t('list.noMessages')}
            </div>
          </div>
        )}

        {/* 虚拟化列表 */}
        {!loading && rows.length > 0 && (
          <div style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}>
            {virtualItems.map((vItem) => {
              const row = rows[vItem.index]
              if (!row) return null

              return (
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
                  ) : listStyle === 'compact' ? (
                    <CompactRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      onSelect={() => onSelectMessage(row.msg.id)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                    />
                  ) : (
                    <CardRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      lang={lang}
                      selected={selectedIds.has(row.msg.id)}
                      onSelect={() => onSelectMessage(row.msg.id)}
                      onToggleSelect={() => onToggleSelect(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                    />
                  )}
                </div>
              )
            })}
          </div>
        )}

        {/* 底部加载状态 */}
        {!loading && messages.length > 0 && (
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
      </div>
    </div>
  )
}

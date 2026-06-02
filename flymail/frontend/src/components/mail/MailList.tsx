import { useRef, useEffect, useCallback } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { Paperclip, Star } from 'lucide-react'
import { groupByDate } from '@/lib/date-group'
import type { ListStyle } from '@/lib/list-prefs'
import type { Folder, MessageListItem } from '@/lib/types'

// ─────────────────────────────────────────────────────────────────────────────
// 类型定义
// ─────────────────────────────────────────────────────────────────────────────

/** 虚拟化行模型：分组标题 或 邮件条目 */
type RowItem =
  | { type: 'header'; label: string }
  | { type: 'item'; msg: MessageListItem }

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
}

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────

function initials(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getMonth() + 1}/${d.getDate()}`
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
          className="px-5 py-3.5 animate-pulse"
          style={{ borderBottom: '1px solid var(--rule)' }}
        >
          <div className="flex items-center gap-3">
            {/* 紧凑模式骨架：无头像，左侧小圆点占位 */}
            <div
              className="h-2 w-2 rounded-full flex-shrink-0"
              style={{ background: 'var(--bg-sunk)' }}
            />
            <div className="flex-1 flex flex-col gap-1.5">
              <div className="flex justify-between gap-4">
                <div className="h-3 rounded" style={{ width: '40%', background: 'var(--bg-sunk)' }} />
                <div className="h-3 rounded" style={{ width: '12%', background: 'var(--bg-sunk)' }} />
              </div>
              <div className="h-3 rounded" style={{ width: '65%', background: 'var(--bg-sunk)' }} />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 紧凑行：发件人 + 日期 / 主题（无头像，左侧未读圆点）
// ─────────────────────────────────────────────────────────────────────────────

interface CompactRowProps {
  msg: MessageListItem
  active: boolean
  onSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
}

function CompactRow({ msg, active, onSelect, onToggleFlag }: CompactRowProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="flex w-full items-center gap-2.5 px-4 py-2 text-left transition-colors"
      style={{
        borderBottom: '1px solid var(--rule)',
        background: active ? 'var(--accent-wash)' : 'transparent',
      }}
      onMouseEnter={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
      }}
      onMouseLeave={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'transparent'
      }}
    >
      {/* 未读圆点 */}
      <span
        className="h-2 w-2 flex-shrink-0 rounded-full"
        style={{ background: msg.seen ? 'transparent' : 'var(--accent-color)' }}
      />

      {/* 发件人 */}
      <span
        className="w-28 flex-shrink-0 truncate text-[13px]"
        style={{
          color: msg.seen ? 'var(--ink-2)' : 'var(--ink)',
          fontWeight: msg.seen ? 400 : 600,
        }}
      >
        {msg.from_name || msg.from_addr}
      </span>

      {/* 主题 */}
      <span
        className="min-w-0 flex-1 truncate text-[12.5px]"
        style={{ color: msg.seen ? 'var(--ink-2)' : 'var(--ink)' }}
      >
        {msg.subject || '（无主题）'}
      </span>

      {/* 右侧：附件 / 日期 / 星标 */}
      <span
        className="flex flex-shrink-0 items-center gap-1.5 text-[10.5px]"
        style={{ color: 'var(--ink-3)' }}
      >
        {msg.has_attachment && <Paperclip size={11} />}
        <span>{formatDate(msg.date)}</span>
        <button
          type="button"
          onClick={onToggleFlag}
          className="flex items-center justify-center"
          style={{ lineHeight: 1 }}
          aria-label={msg.flagged ? 'Unflag' : 'Flag'}
        >
          <Star
            size={13}
            fill={msg.flagged ? 'var(--accent-color)' : 'none'}
            stroke={msg.flagged ? 'var(--accent-color)' : 'var(--ink-3)'}
          />
        </button>
      </span>
    </button>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// 卡片行：头像 + 三行信息（沿用原始样式）
// ─────────────────────────────────────────────────────────────────────────────

interface CardRowProps {
  msg: MessageListItem
  active: boolean
  onSelect: () => void
  onToggleFlag: (e: React.MouseEvent) => void
}

function CardRow({ msg, active, onSelect, onToggleFlag }: CardRowProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="grid w-full grid-cols-[32px_1fr] gap-3 px-5 py-3.5 text-left transition-colors"
      style={{
        borderBottom: '1px solid var(--rule)',
        background: active ? 'var(--accent-wash)' : 'transparent',
      }}
      onMouseEnter={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'
      }}
      onMouseLeave={(e) => {
        if (!active) (e.currentTarget as HTMLElement).style.background = 'transparent'
      }}
    >
      {/* 头像 */}
      <div
        className="flex h-8 w-8 items-center justify-center rounded-md text-[12.5px] font-semibold text-white"
        style={{ background: 'var(--accent-color)' }}
      >
        {initials(msg.from_name, msg.from_addr)}
      </div>

      <div className="min-w-0">
        {/* 第一行：发件人 + 日期/附件/星标 */}
        <div className="flex items-center justify-between gap-2">
          <span
            className="truncate text-[13.5px]"
            style={{
              color: msg.seen ? 'var(--ink-2)' : 'var(--ink)',
              fontWeight: msg.seen ? 400 : 600,
            }}
          >
            {msg.from_name || msg.from_addr}
          </span>
          <span
            className="flex flex-shrink-0 items-center gap-1.5 text-[10.5px]"
            style={{ color: 'var(--ink-3)' }}
          >
            {msg.has_attachment && <Paperclip size={11} />}
            {formatDate(msg.date)}
            <button
              type="button"
              onClick={onToggleFlag}
              className="flex items-center justify-center"
              style={{ lineHeight: 1 }}
              aria-label={msg.flagged ? 'Unflag' : 'Flag'}
            >
              <Star
                size={13}
                fill={msg.flagged ? 'var(--accent-color)' : 'none'}
                stroke={msg.flagged ? 'var(--accent-color)' : 'var(--ink-3)'}
              />
            </button>
          </span>
        </div>

        {/* 第二行：主题 */}
        <div
          className="truncate text-[13px]"
          style={{
            color: msg.seen ? 'var(--ink-2)' : 'var(--ink)',
            fontWeight: msg.seen ? 400 : 600,
          }}
        >
          {msg.subject || '（无主题）'}
        </div>

        {/* 第三行：摘要 */}
        {msg.snippet && (
          <div className="truncate text-[12px]" style={{ color: 'var(--ink-3)' }}>
            {msg.snippet}
          </div>
        )}
      </div>
    </button>
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
}: Props) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)

  const title = folder
    ? folder.type === 'custom'
      ? folder.display_name
      : t(`folder.${folder.type}`)
    : ''

  // ── 构造虚拟化行模型 ──────────────────────────────────────────────────────

  const rows: RowItem[] = (() => {
    if (listStyle === 'compact') {
      // 紧凑模式：按日期分组，插入分组 header 行
      const groups = groupByDate(messages, (m) => m.date)
      const result: RowItem[] = []
      for (const group of groups) {
        result.push({ type: 'header', label: group.label })
        for (const msg of group.items) {
          result.push({ type: 'item', msg })
        }
      }
      return result
    } else {
      // 卡片模式：纯 item 列表，不分组
      return messages.map((msg): RowItem => ({ type: 'item', msg }))
    }
  })()

  // ── 行高估算 ─────────────────────────────────────────────────────────────

  const estimateSize = useCallback(
    (index: number): number => {
      const row = rows[index]
      if (!row) return 52
      if (row.type === 'header') return 28
      return listStyle === 'compact' ? 44 : 80
    },
    [rows, listStyle],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize,
    overscan: 5,
  })

  // ── 无限加载：接近底部时触发 ──────────────────────────────────────────────

  const virtualItems = virtualizer.getVirtualItems()
  const lastIndex = virtualItems.length > 0 ? virtualItems[virtualItems.length - 1].index : -1

  // 仅依赖最末可见行索引，避免每帧滚动都重跑（virtualItems 每次都是新数组引用）。
  useEffect(() => {
    if (lastIndex >= rows.length - 5 && hasNextPage && !isFetchingNextPage) {
      onLoadMore()
    }
  }, [lastIndex, rows.length, hasNextPage, isFetchingNextPage, onLoadMore])

  // ── 渲染 ─────────────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col">
      {/* 顶部标题栏 */}
      <div className="px-5 py-3 flex-shrink-0" style={{ borderBottom: '1px solid var(--rule)' }}>
        <div className="text-lg font-medium">{title}</div>
        {folder && folder.unread_count > 0 && (
          <div className="text-[11px]" style={{ color: 'var(--ink-3)' }}>
            {t('list.unreadCount', { count: folder.unread_count })}
          </div>
        )}
      </div>

      {/* 列表区域 */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        {/* 首屏加载骨架 */}
        {loading && <SkeletonList />}

        {/* 空态 */}
        {!loading && messages.length === 0 && (
          <div
            className="px-5 py-8 text-center text-sm"
            style={{ color: 'var(--ink-3)' }}
          >
            {t('list.empty')}
          </div>
        )}

        {/* 虚拟化列表 */}
        {!loading && rows.length > 0 && (
          <div
            style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}
          >
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
                    // 分组标题行（行高固定，不用动态测量）
                    <div
                      className="px-4 py-1 text-[11px] font-medium"
                      style={{
                        color: 'var(--ink-3)',
                        background: 'var(--bg-alt)',
                        borderBottom: '1px solid var(--rule)',
                      }}
                    >
                      {row.label}
                    </div>
                  ) : listStyle === 'compact' ? (
                    <CompactRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      onSelect={() => onSelectMessage(row.msg.id)}
                      onToggleFlag={(e) => {
                        e.stopPropagation()
                        onToggleFlag(row.msg.id, !row.msg.flagged)
                      }}
                    />
                  ) : (
                    <CardRow
                      msg={row.msg}
                      active={row.msg.id === activeMessageId}
                      onSelect={() => onSelectMessage(row.msg.id)}
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
            className="py-3 text-center text-[12px]"
            style={{ color: 'var(--ink-3)' }}
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

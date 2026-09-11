// 会话手风琴：一条会话的全部成员按时间升序排成可折叠的列表。
//
// 与单封 Reader 的分工：工具栏（ReaderToolbar）与正文（MessageBody）两边共用，
// 这里只负责「哪几封展开、每封折叠时显示什么、操作打到整条还是单封」。
//
// 默认展开范围内最新一封 + 所有未读，其余折叠成一行头部——
// 十封往返全展开等于把同一段历史铺满整屏，那正是会话视图要解决的问题。

import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { DropMenu } from '@/components/ui/DropMenu'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'
import { MessageBody } from '@/components/mail/MessageBody'
import { ReaderToolbar } from '@/components/mail/ReaderToolbar'
import { ReaderEmpty, ReaderError, ReaderSkeleton } from '@/components/mail/ReaderStates'
import {
  formatAddresses,
  formatDate,
  senderInitial,
  useDelayedFlag,
} from '@/lib/mail-format'
import {
  useAccounts,
  useBatchRead,
  useDeleteMessage,
  useFolders,
  useMarkRead,
  useMessageDetail,
  useMoveMessage,
  useThreadBatchFlag,
  useThreadBatchRead,
  useThreadMessages,
  useToggleFlag,
} from '@/lib/queries'
import { defaultExpanded } from '@/lib/thread-format'
import type { Folder, MessageDetail, MessageListItem } from '@/lib/types'

/** 文件夹显示名：系统文件夹走 i18n，自定义文件夹用服务器给的名字 */
function folderLabel(
  folders: Folder[],
  folderId: number,
  t: (k: string) => string,
): string | null {
  const f = folders.find((x) => x.id === folderId)
  if (!f) return null
  return f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
}

// ── 手风琴单项 ───────────────────────────────────────────

interface ThreadItemProps {
  msg: MessageListItem
  expanded: boolean
  /** 最近点击的那一封：工具栏的回复/转发以它为准，给一条视觉标记 */
  active: boolean
  onToggle: () => void
  /** 该邮件所在文件夹的显示名；null 表示文件夹信息尚未加载 */
  folderName: string | null
  /** 当前账户自己的邮箱（把收件人里的自己显示成「我」）*/
  selfAddr: string
  /** 单封操作的移动目标（同账户的可选文件夹）*/
  accountFolders: Folder[]
  /** 正文里点到 mailto: 链接时打开撰写器 */
  onMailto?: (href: string) => void
}

function ThreadItem({
  msg,
  expanded,
  active,
  onToggle,
  folderName,
  selfAddr,
  accountFolders,
  onMailto,
}: ThreadItemProps) {
  const { t } = useTranslation()
  // 展开时才请求正文：一条 20 封的会话若一次性全拉，本地库也要扫 20 遍附件表
  const { data: detail } = useMessageDetail(expanded ? msg.id : null)
  const markRead = useMarkRead()
  const toggleFlag = useToggleFlag()
  const deleteMessage = useDeleteMessage()
  const moveMessage = useMoveMessage()

  // 注意：自动标已读不在这里做，由 ThreadReader 统一处理——
  // 默认展开的那批未读若各自发一次请求，打开一条 8 封未读的会话就是 8 个
  // POST /messages/:id/read 加 8 轮全量 invalidate。见 markExpandedRead。

  const senderName = msg.from_name || msg.from_addr
  const initial = senderInitial(msg.from_name, msg.from_addr)

  const meLabel = t('reader.me')
  const toText = detail ? formatAddresses(detail.to ?? [], selfAddr, meLabel) : ''
  const ccText =
    detail?.cc && detail.cc.length > 0 ? formatAddresses(detail.cc, selfAddr, meLabel) : ''

  // 单封「移动到」目标：同账户里除当前文件夹以外的可选文件夹
  const moveTargets = accountFolders.filter((f) => f.selectable && f.id !== msg.folder_id)
  const moreItems: CtxMenuItem[] = [
    {
      key: 'unread',
      label: t('reader.markUnread'),
      icon: 'mail',
      onSelect: () => markRead.mutate({ id: msg.id, read: false }),
    },
  ]
  if (moveTargets.length > 0) {
    moreItems.push({
      key: 'move',
      label: t('reader.move'),
      icon: 'folder',
      disabled: moveMessage.isPending,
      children: moveTargets.map((f) => ({
        key: `mv-${f.id}`,
        label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
        icon: 'folder',
        onSelect: () => moveMessage.mutate({ id: msg.id, folderId: f.id }),
      })),
    })
  }

  function handleDeleteOne() {
    if (!window.confirm(t('reader.deleteConfirm'))) return
    deleteMessage.mutate(msg.id)
  }

  return (
    <div
      className={
        'thread-item' +
        (expanded ? ' expanded' : '') +
        (active ? ' active' : '') +
        (msg.seen ? '' : ' unread')
      }
    >
      {/* 折叠行头：整行可点开合。展开态它仍在，用作这一封的信头。 */}
      <div
        className="ti-head"
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={onToggle}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggle()
          }
        }}
      >
        <div className="avatar-sq" style={{ background: 'var(--accent-color)', color: 'white' }}>
          {initial}
        </div>

        <div className="ti-main">
          <div className="ti-from">
            {senderName}
            {expanded && msg.from_name && (
              <span className="ti-addr">&lt;{msg.from_addr}&gt;</span>
            )}
          </div>
          {expanded ? (
            <div className="ti-to">
              {t('reader.sendTo')} {toText || '—'}
              {ccText && (
                <span style={{ marginLeft: 6 }}>
                  · {t('reader.cc')} {ccText}
                </span>
              )}
            </div>
          ) : (
            <div className="ti-snippet">{msg.snippet || t('reader.noBody')}</div>
          )}
        </div>

        <div className="ti-side">
          {msg.flagged && <Icon name="star-fill" size={12} />}
          {msg.has_attachment && <Icon name="attach" size={12} />}
          {folderName && <span className="ti-folder">{folderName}</span>}
          <span className="ti-time">{formatDate(msg.date)}</span>
        </div>
      </div>

      {/* 展开态：单封小工具栏 + 正文 */}
      {expanded && (
        <>
          <div className="ti-tools">
            <button
              type="button"
              className={'icon-btn' + (msg.flagged ? ' starred' : '')}
              onClick={() => toggleFlag.mutate({ id: msg.id, flagged: !msg.flagged })}
              title={msg.flagged ? t('reader.unstar') : t('reader.star')}
              aria-label={msg.flagged ? t('reader.unstar') : t('reader.star')}
            >
              <Icon name={msg.flagged ? 'star-fill' : 'star'} size={14} />
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={handleDeleteOne}
              disabled={deleteMessage.isPending}
              title={t('reader.thread.deleteOne')}
              aria-label={t('reader.thread.deleteOne')}
            >
              <Icon name="trash" size={14} />
            </button>
            <DropMenu
              items={moreItems}
              trigger={
                <button
                  type="button"
                  className="icon-btn"
                  title={t('reader.more')}
                  aria-label={t('reader.more')}
                >
                  <Icon name="more" size={14} />
                </button>
              }
            />
          </div>

          {detail ? (
            <MessageBody key={detail.id} detail={detail} onMailto={onMailto} />
          ) : (
            <div className="ti-loading">{t('reader.loading')}</div>
          )}
        </>
      )}
    </div>
  )
}

// ── 主组件 ───────────────────────────────────────────────

interface ThreadReaderProps {
  threadId: string | null
  /** 列表行的 latest_id（范围内最新一封），决定默认展开哪一封 */
  latestId: number | null
  /** 会话所属账户；用于移动目标与识别收件人里的「我」 */
  accountId: number | null
  /**
   * 文件夹视图下的作用域：删除/移动只动这个文件夹里的成员。
   * 聚合/搜索视图传 undefined，由后端排除 sent / drafts。
   */
  inFolderId?: number
  onReply?: (d: MessageDetail) => void
  onForward?: (d: MessageDetail) => void
  /**
   * 删除 / 归档 / 移动整条会话。
   *
   * 与 Reader 同理：这三个动作要走延迟提交与撤销，并在成功后把阅读区推进到
   * 下一条会话，只有持有列表的 Shell 知道下一条是谁。
   */
  onDelete: () => void
  /** null = 该账户没有归档文件夹 */
  onArchive: (() => void) | null
  onMove: (folderId: number) => void
  onPrev?: (() => void) | null
  onNext?: (() => void) | null
  /** 「当前这一封」变化时上报，供 Shell 把 r 快捷键接到正确的邮件上 */
  onActiveMessageChange?: (id: number | null) => void
  /** 正文里点到 mailto: 链接时打开撰写器 */
  onMailto?: (href: string) => void
}

export function ThreadReader({
  threadId,
  latestId,
  accountId,
  inFolderId,
  onReply,
  onForward,
  onDelete,
  onArchive,
  onMove,
  onPrev,
  onNext,
  onActiveMessageChange,
  onMailto,
}: ThreadReaderProps) {
  const { t } = useTranslation()
  const {
    data: messages = [],
    isLoading,
    isFetching,
    isError,
    error,
    isPlaceholderData,
  } = useThreadMessages(threadId)

  const { data: accountFolders = [] } = useFolders(accountId)
  const { data: accounts = [] } = useAccounts()
  const threadRead = useThreadBatchRead()
  const threadFlag = useThreadBatchFlag()
  const batchRead = useBatchRead()
  const markRead = useMarkRead()

  // ── 展开集合 ──────────────────────────────────────────
  // 每换一条会话重算一次默认值；同一条会话内用户的开合选择必须保留，
  // 所以不能简单地依赖 messages（标已读会让它回流出新数组）。
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())
  const [activeId, setActiveId] = useState<number | null>(null)
  const initedRef = useRef<string | null>(null)

  /**
   * 「这些邮件的标已读请求已经发过了」。
   *
   * ⚠ 这道闸门不能省：乐观更新落在 onMutate 的 await 之后，而 mutate() 立刻触发
   * 一次重渲染，这段窗口里 seen 仍是 false；照着它重试就会自激成
   * render → mutate → render（同 lib/list-guards.ts 的成因）。
   * 与 `!m.seen` 是两道独立的守卫，缺一不可。
   */
  const readSentRef = useRef<Set<number>>(new Set())

  /** 把一批未读标为已读：一封走单封接口，多封合并成一次批量请求 */
  function markExpandedRead(list: MessageListItem[]) {
    const ids = list.filter((m) => !m.seen && !readSentRef.current.has(m.id)).map((m) => m.id)
    if (ids.length === 0) return
    for (const id of ids) readSentRef.current.add(id)
    if (ids.length === 1) markRead.mutate({ id: ids[0], read: true })
    else batchRead.mutate({ ids, read: true })
  }

  useEffect(() => {
    if (threadId == null || isPlaceholderData || messages.length === 0) return
    if (initedRef.current === threadId) return
    initedRef.current = threadId
    // 换了会话，上一条的闸门记录不再相关
    readSentRef.current = new Set()
    const next = defaultExpanded(messages, latestId)
    setExpanded(next)
    // 「当前这一封」默认取范围内最新一封，回复/转发以它为准
    const fallback = messages[messages.length - 1]?.id ?? null
    setActiveId(latestId != null && messages.some((m) => m.id === latestId) ? latestId : fallback)
    // 默认展开的未读一次标完：逐封发等于打开会话就是一串请求加一串全量 invalidate
    markExpandedRead(messages.filter((m) => next.has(m.id)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId, isPlaceholderData, messages.length, latestId])

  // 上报给 Shell：r 快捷键要回复的是「当前这一封」而不是会话本身。
  // threadId 为空（关闭会话）时必须报 null，否则 Shell 会拿着上一条会话里的
  // 那封邮件继续响应 r，起草出一封指向已经关掉的会话的回复。
  // 回调每次渲染都是新引用，直接进依赖数组等于「每渲染必上报」；
  // 用 ref 固定住（写 ref 必须在 effect 里，渲染期间写会被 react-hooks/refs 拦下）。
  const reportRef = useRef(onActiveMessageChange)
  useEffect(() => {
    reportRef.current = onActiveMessageChange
  })
  useEffect(() => {
    reportRef.current?.(threadId == null ? null : activeId)
  }, [threadId, activeId])
  // 卸载（切回单封视图）时同样清空
  useEffect(() => () => reportRef.current?.(null), [])

  function toggleItem(id: number) {
    // 从当前渲染的 expanded 判断方向，而不是在 setState 更新函数里设标记——
    // 更新函数必须是纯的（严格模式会跑两次），副作用不能挂在里面。
    const opening = !expanded.has(id)
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
    // 手动展开一封未读时单发一次；默认展开的那批已在 init 里合并发过
    if (opening) {
      const msg = messages.find((m) => m.id === id)
      if (msg) markExpandedRead([msg])
    }
    // 点开合就是「我现在关心这一封」，回复/转发随之改指向
    setActiveId(id)
  }

  // 回复/转发针对当前展开且最近点击的那封；详情多半已由展开项拉过，这里命中缓存
  const { data: activeDetail } = useMessageDetail(activeId)

  // ── 会话级操作 ────────────────────────────────────────
  const ids = threadId != null ? [threadId] : []

  // 整条是否已全部已读 / 是否有星标成员：菜单文案据此在两个方向间切换
  const anyUnread = messages.some((m) => !m.seen)
  const anyFlagged = messages.some((m) => m.flagged)

  // 排除当前文件夹：文件夹视图下「移动到收件箱」而成员就在收件箱，是一次空操作
  const moveTargets = accountFolders.filter((f) => f.selectable && f.id !== inFolderId)
  const moreItems: CtxMenuItem[] = [
    {
      key: 'flag',
      label: anyFlagged ? t('reader.thread.unstarAll') : t('reader.thread.starAll'),
      icon: anyFlagged ? 'star-fill' : 'star',
      onSelect: () => threadFlag.mutate({ threadIds: ids, flagged: !anyFlagged }),
    },
    {
      key: 'read',
      label: anyUnread ? t('reader.thread.readAll') : t('reader.thread.unreadAll'),
      icon: 'mail',
      onSelect: () => threadRead.mutate({ threadIds: ids, read: anyUnread }),
    },
  ]
  if (moveTargets.length > 0) {
    moreItems.push({
      key: 'move',
      label: t('reader.thread.moveAll'),
      icon: 'folder',
      children: moveTargets.map((f) => ({
        key: `mv-${f.id}`,
        label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
        icon: 'folder',
        onSelect: () => onMove(f.id),
      })),
    })
  }

  const selfAddr = accounts.find((a) => a.id === accountId)?.email ?? ''
  // 主题取最后一封（时间升序，最后一封即最新）；深链进来时列表里没有这条也能拿到
  const subject = messages[messages.length - 1]?.subject ?? ''

  // 折叠项也要显示文件夹标签，映射一次复用
  const folderNames = useMemo(() => {
    const map = new Map<number, string | null>()
    for (const m of messages) {
      if (!map.has(m.folder_id)) map.set(m.folder_id, folderLabel(accountFolders, m.folder_id, t))
    }
    return map
  }, [messages, accountFolders, t])

  // 与单封视图同款的加载策略：短暂的错位不闪骨架
  const showSkeleton = useDelayedFlag(isLoading || isPlaceholderData, 150)

  if (threadId == null) return <ReaderEmpty />
  if (showSkeleton) return <ReaderSkeleton />
  if (isError) return <ReaderError error={error} />
  // 后端对未知 thread_id 返回 200 + 空数组（会话被重建打散、或整条已删除）。
  // 给一块有说明的面板，而不是一片没有任何解释的空白。
  if (messages.length === 0) {
    // ⚠ 首屏那 150ms 的「不闪骨架」静默窗口里 messages 也是空的，
    // 这里不挡住就会每次打开会话都先闪一下「会话不存在」。
    if (isLoading || isFetching || isPlaceholderData) return <section className="col reader" />
    return (
      <section className="col reader">
        <div className="reader-empty">
          <div className="empty-inner">
            <h3 style={{ color: 'var(--ink-2)' }}>{t('reader.thread.missing')}</h3>
            <p>{t('reader.thread.missingHint')}</p>
          </div>
        </div>
      </section>
    )
  }

  return (
    <section className="col reader">
      <ReaderToolbar
        disabled={isPlaceholderData}
        onPrev={onPrev}
        onNext={onNext}
        onReply={onReply && activeDetail ? () => onReply(activeDetail) : undefined}
        onForward={onForward && activeDetail ? () => onForward(activeDetail) : undefined}
        onArchive={onArchive}
        onDelete={onDelete}
        moreItems={moreItems}
      />

      <div className="reader-scroll">
        <div className="reader-inner">
          <h1 className="reader-subject">{subject || t('list.noSubject')}</h1>

          <div className="reader-meta-row">
            <span className="mi-tag">{t('reader.thread.count', { count: messages.length })}</span>
            {anyUnread && (
              <span className="mi-tag">
                {t('list.unreadCount', { count: messages.filter((m) => !m.seen).length })}
              </span>
            )}
          </div>

          <div className="thread-accordion">
            {messages.map((m) => (
              <ThreadItem
                key={m.id}
                msg={m}
                expanded={expanded.has(m.id)}
                active={m.id === activeId}
                onToggle={() => toggleItem(m.id)}
                folderName={folderNames.get(m.folder_id) ?? null}
                selfAddr={selfAddr}
                accountFolders={accountFolders}
                onMailto={onMailto}
              />
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

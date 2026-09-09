import { useTranslation } from 'react-i18next'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'
import { MessageBody } from '@/components/mail/MessageBody'
import { ReaderToolbar } from '@/components/mail/ReaderToolbar'
import { ReaderEmpty, ReaderError, ReaderSkeleton } from '@/components/mail/ReaderStates'
import { formatAddresses, formatDate, senderInitial, useDelayedFlag } from '@/lib/mail-format'
import { useMessageDetail, useMarkRead, useToggleFlag, useDeleteMessage, useMoveMessage, useFolders, useAccounts } from '@/lib/queries'
import type { MessageDetail } from '@/lib/types'

// ── 主组件 Props ─────────────────────────────────────────

interface ReaderProps {
  messageId: number | null
  onReply?: (d: MessageDetail) => void
  onForward?: (d: MessageDetail) => void
  /** 删除/移动成功后回调（用于清空当前选中邮件） */
  onClose?: () => void
  /**
   * 上一封 / 下一封。null 表示已在列表边界（按钮置灰）。
   * 列表上下文在 Shell 手里，这里只负责触发——与 j/k 快捷键走同一套顺序。
   */
  onPrev?: (() => void) | null
  onNext?: (() => void) | null
  /** 归档成功后的提示回调（Toast 由 Shell 统一发） */
  onArchived?: () => void
  /** 正文里点到 mailto: 链接时打开撰写器（正文 iframe 已不同源，只能由它上报） */
  onMailto?: (href: string) => void
}

// ── 主组件 ───────────────────────────────────────────────

export function Reader({ messageId, onReply, onForward, onClose, onPrev, onNext, onArchived, onMailto }: ReaderProps) {
  const { t } = useTranslation()

  const { data: detail, isLoading, isError, error } = useMessageDetail(messageId)
  const toggleFlag = useToggleFlag()
  const markRead = useMarkRead()
  const deleteMessage = useDeleteMessage()
  const moveMessage = useMoveMessage()
  // 移动目标：当前邮件所属账户的文件夹（detail 未就绪时为 null）
  const { data: accountFolders = [] } = useFolders(detail?.account_id ?? null)
  // 账户列表：用于识别收件人中的「我」（已在别处请求过，这里命中缓存）
  const { data: accounts = [] } = useAccounts()

  // 删除当前邮件（移到回收站/永久删除由后端判定），成功后清空选中
  function handleDelete() {
    if (messageId == null) return
    if (!window.confirm(t('reader.deleteConfirm'))) return
    deleteMessage.mutate(messageId, { onSuccess: () => onClose?.() })
  }

  // 移动当前邮件到目标文件夹，成功后清空选中
  function handleMove(folderId: number) {
    if (messageId == null) return
    moveMessage.mutate({ id: messageId, folderId }, { onSuccess: () => onClose?.() })
  }

  // 归档 = 移动到该账户的 archive 文件夹。
  // 独立成一个按钮而不是让用户走「移动到」下拉：归档是高频动作，
  // 两步下拉对一个每天点几十次的操作来说太重。
  const archiveFolder = accountFolders.find((f) => f.type === 'archive' && f.selectable)
  // 已经在归档文件夹里就没有再归档一次的意义
  const canArchive = archiveFolder != null && archiveFolder.id !== detail?.folder_id

  function handleArchive() {
    if (messageId == null || archiveFolder == null) return
    moveMessage.mutate(
      { id: messageId, folderId: archiveFolder.id },
      { onSuccess: () => { onArchived?.(); onClose?.() } },
    )
  }

  // ── 加载态：宁可短暂留住上一封，也不要闪一帧骨架 ──────────
  // keepPreviousData 让切换瞬间仍有内容可渲染，但那是上一封邮件（id 对不上）。
  // 只有当这个错位状态持续超过 150ms（正文要现从服务器抓）才切骨架屏；
  // 本地已有正文时几毫秒就换好了，全程不闪。
  const stale = detail != null && detail.id !== messageId
  const showSkeleton = useDelayedFlag(isLoading || stale, 150)

  // ── 空态：未选中邮件 ──────────────────────────────────
  if (messageId == null) return <ReaderEmpty />

  // ── 加载中：骨架屏（仅在慢加载时出现，见 showSkeleton）──────
  if (showSkeleton) return <ReaderSkeleton />

  // 首次打开且还没有任何数据可渲染：给一块同尺寸的空栏占位，
  // 150ms 内数据到达就直接出内容，超时才由 showSkeleton 换成骨架。
  if (!detail && !isError) {
    return <section className="col reader" />
  }

  // ── 加载失败 ──────────────────────────────────────────
  if (isError) return <ReaderError error={error} />

  // detail 此时保证非 null
  if (!detail) return null

  // ── 「更多」菜单项 ────────────────────────────────────────
  // 收纳标准：低频 + 文案会变长变短的（星标）+ 需要二级选择的（移动到）。
  // 高频的回复/转发/归档/删除留在工具栏上，保持一眼可点。
  const moveTargets = accountFolders.filter((f) => f.selectable && f.id !== detail.folder_id)
  const moreItems: CtxMenuItem[] = [
    {
      key: 'flag',
      label: detail.flagged ? t('reader.unstar') : t('reader.star'),
      icon: detail.flagged ? 'star-fill' : 'star',
      onSelect: () => toggleFlag.mutate({ id: messageId, flagged: !detail.flagged }),
    },
    {
      key: 'unread',
      label: t('reader.markUnread'),
      icon: 'mail',
      onSelect: () => markRead.mutate({ id: messageId, read: false }),
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
        onSelect: () => handleMove(f.id),
      })),
    })
  }

  // 发件人信息
  const senderName = detail.from_name || detail.from_addr
  const initial = senderInitial(detail.from_name, detail.from_addr)

  // 收件人 / 抄送显示文字
  // 当前账户自己的地址，用于把收件人里的自己显示成「我」
  const selfAddr = accounts.find((a) => a.id === detail.account_id)?.email ?? ''
  const meLabel = t('reader.me')
  const toText = formatAddresses(detail.to ?? [], selfAddr, meLabel)
  const ccText = detail.cc && detail.cc.length > 0 ? formatAddresses(detail.cc, selfAddr, meLabel) : ''

  return (
    <section className="col reader">
      {/* ── 工具条 ───────────────────────────────────────
           stale 期间（屏幕上还是上一封、messageId 已指向新邮件）整条禁用点击，
           否则会出现"看着旧邮件、把操作打到新邮件上"。 */}
      <ReaderToolbar
        disabled={stale}
        onPrev={onPrev}
        onNext={onNext}
        onReply={onReply ? () => onReply(detail) : undefined}
        onForward={onForward ? () => onForward(detail) : undefined}
        onArchive={canArchive ? handleArchive : null}
        archiveBusy={moveMessage.isPending}
        onDelete={handleDelete}
        deleteBusy={deleteMessage.isPending}
        moreItems={moreItems}
      />

      {/* ── 正文滚动区 ──────────────────────────────────── */}
      <div className="reader-scroll">
        <div className="reader-inner">
          {/* 主题大标题 */}
          <h1 className="reader-subject">
            {detail.subject || t('list.noSubject')}
          </h1>

          {/* meta 行：收件人账号/日期等小标签 */}
          <div className="reader-meta-row">
            <span className="mi-tag" style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
              <span
                className="dot"
                style={{
                  width: 7,
                  height: 7,
                  borderRadius: '50%',
                  background: 'var(--accent-color)',
                  display: 'inline-block',
                }}
              />
              {formatDate(detail.date)}
            </span>
          </div>

          {/* ── 单封消息 thread-msg ──────────────────────── */}
          <div className="thread-msg">
            {/* 消息头：头像 + 发件人 + 时间 */}
            <div className="thread-head">
              {/* 方形首字母头像 */}
              <div
                className="avatar-sq"
                style={{ background: 'var(--accent-color)', color: 'white' }}
              >
                {initial}
              </div>

              {/* 发件人 + 收件人 */}
              <div>
                <div className="th-from">
                  {senderName}
                  {detail.from_name && (
                    <span
                      style={{
                        color: 'var(--ink-3)',
                        fontWeight: 400,
                        fontSize: 12,
                        fontFamily: 'var(--font-mono)',
                        marginLeft: 6,
                      }}
                    >
                      &lt;{detail.from_addr}&gt;
                    </span>
                  )}
                </div>
                <div className="th-to">
                  {t('reader.sendTo')} {toText}
                  {ccText && (
                    <span style={{ marginLeft: 6 }}>
                      · {t('reader.cc')} {ccText}
                    </span>
                  )}
                </div>
              </div>

              {/* 时间戳 */}
              <div className="th-time">{formatDate(detail.date)}</div>
            </div>

            {/* 消息正文（远程图拦截 / iframe / 引用折叠 / 附件都在这里）*/}
            <MessageBody key={detail.id} detail={detail} onMailto={onMailto} />
          </div>
          {/* 底部回复框已移除：入口在顶部工具栏已有一份，占着正文空间不划算 */}
        </div>
      </div>
    </section>
  )
}

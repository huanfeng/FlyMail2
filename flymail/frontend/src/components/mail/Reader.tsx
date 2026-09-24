import { useTranslation } from 'react-i18next'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'
import { MessageBody } from '@/components/mail/MessageBody'
import { ReaderToolbar } from '@/components/mail/ReaderToolbar'
import { ReaderEmpty, ReaderError, ReaderSkeleton } from '@/components/mail/ReaderStates'
import {
  folderLabel,
  formatAddresses,
  formatDate,
  senderInitial,
  useDelayedFlag,
} from '@/lib/mail-format'
import { useMessageDetail, useMarkRead, useToggleFlag, useFolders, useAccounts } from '@/lib/queries'
import { useMessageTranslator } from '@/hooks/useMessageTranslator'
import type { MessageDetail } from '@/lib/types'

// ── 主组件 Props ─────────────────────────────────────────

interface ReaderProps {
  messageId: number | null
  onReply?: (d: MessageDetail) => void
  onForward?: (d: MessageDetail) => void
  /**
   * 删除 / 归档 / 移动当前邮件。
   *
   * 这三个动作不在这里实现：它们要走延迟提交与撤销，并在成功后把阅读区推进到
   * 下一封，而"下一封是谁"只有持有列表的 Shell 知道。Reader 只负责触发，
   * 于是工具栏按钮与键盘快捷键走的是同一条路径，不会各做各的。
   */
  onDelete: () => void
  /** null = 该账户没有归档文件夹，或当前邮件已在归档里 */
  onArchive: (() => void) | null
  onMove: (folderId: number) => void
  /**
   * 上一封 / 下一封。null 表示已在列表边界（按钮置灰）。
   * 列表上下文在 Shell 手里，这里只负责触发——与 j/k 快捷键走同一套顺序。
   */
  onPrev?: (() => void) | null
  onNext?: (() => void) | null
  /** 正文里点到 mailto: 链接时打开撰写器（正文 iframe 已不同源，只能由它上报） */
  onMailto?: (href: string) => void
}

// ── 主组件 ───────────────────────────────────────────────

export function Reader({
  messageId,
  onReply,
  onForward,
  onPrev,
  onNext,
  onDelete,
  onArchive,
  onMove,
  onMailto,
}: ReaderProps) {
  const { t } = useTranslation()

  const { data: detail, isLoading, isError, error } = useMessageDetail(messageId)
  const toggleFlag = useToggleFlag()
  const markRead = useMarkRead()
  // 移动目标：当前邮件所属账户的文件夹（detail 未就绪时为 null）
  const { data: accountFolders = [] } = useFolders(detail?.account_id ?? null)
  // 账户列表：用于识别收件人中的「我」（已在别处请求过，这里命中缓存）
  const { data: accounts = [] } = useAccounts()
  const translate = useMessageTranslator(messageId)

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

      children: moveTargets.map((f) => ({
        key: `mv-${f.id}`,
        label: f.type === 'custom' ? f.display_name : t(`folder.${f.type}`),
        icon: 'folder',
        onSelect: () => onMove(f.id),
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

  // meta 行：这一封在哪。与会话手风琴的 .ti-folder 共用同一个 folderLabel，
  // 两个视图不会对同一个文件夹给出不同的名字。
  const currentFolderName = folderLabel(accountFolders, detail.folder_id, t)
  const ownerAccount = accounts.find((a) => a.id === detail.account_id)
  // 单账户用户每封信都看到同一个地址，那是噪音；多账户时它才携带信息
  const showAccountTag = accounts.length > 1 && ownerAccount != null

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
        onTranslate={translate.toggle}
        translateActive={translate.showing}
        translateBusy={translate.busy}
        translateTitle={translate.hint(detail.detect_lang)}
        onArchive={onArchive}
        onDelete={onDelete}
        moreItems={moreItems}
      />

      {/* ── 正文滚动区 ──────────────────────────────────── */}
      <div className="reader-scroll">
        <div className="reader-inner">
          {/* 主题大标题 */}
          <h1 className="reader-subject">
            {detail.subject || t('list.noSubject')}
          </h1>

          {/* meta 行：这一封在哪（账户 / 文件夹）。
              原先这里放的是 formatDate(detail.date)，而它在下面 40px 处的
              .th-time 里又出现一遍——一整行的视觉预算只承载了一条重复信息。
              会话手风琴的折叠行（.ti-folder）早就显示文件夹了，这里补齐。
              账户只在多账户时才显示：单账户用户每封信都看见同一个地址，是噪音。 */}
          {(currentFolderName || showAccountTag) && (
            <div className="reader-meta-row">
              {showAccountTag && (
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
                  {ownerAccount?.email}
                </span>
              )}
              {currentFolderName && <span className="mi-tag">{currentFolderName}</span>}
            </div>
          )}

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
            <MessageBody
              key={detail.id}
              detail={detail}
              onMailto={onMailto}
              translation={translate.translation}
              translating={translate.busy}
              translateError={translate.error}
              onRetranslate={translate.redo}
              onShowOriginal={translate.hide}
            />
          </div>
          {/* 底部回复框已移除：入口在顶部工具栏已有一份，占着正文空间不划算 */}
        </div>
      </div>
    </section>
  )
}

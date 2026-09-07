// 阅读区工具栏：上一封/下一封 · 回复 转发 · 归档 删除 · 更多。
//
// 单封视图与会话视图的按钮布局完全一致，只是每颗按钮打到哪里不同——
// 单封打在当前邮件上，会话打在整条会话（回复/转发例外，见 ThreadReader）。
// 因此这里只描述「有哪些按钮、长什么样」，动作全部由调用方注入。

import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { DropMenu } from '@/components/ui/DropMenu'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'

export interface ReaderToolbarProps {
  /** 整条禁用（数据仍是上一封时避免"看着旧邮件、操作新邮件"） */
  disabled?: boolean
  /**
   * 上一封 / 下一封。undefined = 不显示这组按钮，null = 显示但置灰（已在边界）。
   */
  onPrev?: (() => void) | null
  onNext?: (() => void) | null
  onReply?: () => void
  onForward?: () => void
  /** 归档：null 表示该账户没有归档文件夹或已在归档里，按钮不出现 */
  onArchive?: (() => void) | null
  archiveBusy?: boolean
  onDelete: () => void
  deleteBusy?: boolean
  /** 「更多」菜单项（星标 / 标为未读 / 移动到…） */
  moreItems: CtxMenuItem[]
}

export function ReaderToolbar({
  disabled,
  onPrev,
  onNext,
  onReply,
  onForward,
  onArchive,
  archiveBusy,
  onDelete,
  deleteBusy,
  moreItems,
}: ReaderToolbarProps) {
  const { t } = useTranslation()
  return (
    <div className="reader-toolbar" style={disabled ? { pointerEvents: 'none' } : undefined}>
      {/* 上一封 / 下一封：纯图标（这两个是导航不是操作，不占文字宽度），
          置于最左并与操作组以分隔线隔开。键盘用户走 j/k，这里是给鼠标用户的入口。 */}
      {(onPrev !== undefined || onNext !== undefined) && (
        <>
          <button
            type="button"
            className="tb-btn tb-icon"
            onClick={() => onPrev?.()}
            disabled={!onPrev}
            title={t('reader.prev')}
            aria-label={t('reader.prev')}
          >
            <Icon name="chevron-up" size={15} />
          </button>
          <button
            type="button"
            className="tb-btn tb-icon"
            onClick={() => onNext?.()}
            disabled={!onNext}
            title={t('reader.next')}
            aria-label={t('reader.next')}
          >
            <Icon name="chevron-down" size={15} />
          </button>
          <div className="tb-sep" />
        </>
      )}

      {/* 回复 */}
      {onReply && (
        <button type="button" className="tb-btn" onClick={onReply} title={t('reader.reply')}>
          <Icon name="reply" size={14} />
          <span>{t('reader.reply')}</span>
        </button>
      )}
      {/* 转发 */}
      {onForward && (
        <button type="button" className="tb-btn" onClick={onForward} title={t('reader.forward')}>
          <Icon name="forward" size={14} />
          <span>{t('reader.forward')}</span>
        </button>
      )}

      <div className="tb-sep" />

      {/* 一键归档（仅当账户有归档文件夹、且当前不在归档里时出现）*/}
      {onArchive && (
        <button
          type="button"
          className="tb-btn"
          onClick={onArchive}
          title={t('reader.archive')}
          disabled={archiveBusy}
        >
          <Icon name="archive" size={14} />
          <span>{t('reader.archive')}</span>
        </button>
      )}

      {/* 删除 */}
      <button
        type="button"
        className="tb-btn"
        onClick={onDelete}
        title={t('reader.delete')}
        disabled={deleteBusy}
        style={{ color: 'var(--destructive)' }}
      >
        <Icon name="trash" size={14} />
        <span>{t('reader.delete')}</span>
      </button>

      {/* 更多：低频动作收进菜单，工具栏只留回复/转发/归档/删除四个高频项。
          顺带解决了工具栏宽度不稳定——星标按钮的文案会在「星标 / 取消星标」
          之间变长变短，留在栏上就是一个随邮件抖动的宽度源。 */}
      <DropMenu
        items={moreItems}
        trigger={
          <button
            type="button"
            className="tb-btn tb-icon"
            title={t('reader.more')}
            aria-label={t('reader.more')}
          >
            <Icon name="more" size={16} />
          </button>
        }
      />
    </div>
  )
}

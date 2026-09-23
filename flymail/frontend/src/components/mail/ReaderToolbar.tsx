// 阅读区工具栏：上一封/下一封 · 回复 转发 · 翻译 · 归档 删除 · 更多。
//
// 单封视图与会话视图的按钮布局完全一致，只是每颗按钮打到哪里不同——
// 单封打在当前邮件上，会话打在整条会话（回复/转发例外，见 ThreadReader）。
// 因此这里只描述「有哪些按钮、长什么样」，动作全部由调用方注入。

import { useTranslation } from 'react-i18next'
import { KEY, withShortcut } from '@/lib/shortcuts'
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
  /**
   * 翻译 / 显示原文。undefined = 整颗按钮不出现（后端没有这个能力）。
   *
   * 它是一个**开关**，不是一次性动作：点开显示译文，再点回到原文。
   * 翻译中的转圈、失败的提示都在正文区表达——工具栏上只保留"现在在看哪一版"。
   */
  onTranslate?: () => void
  /** 当前正显示译文 */
  translateActive?: boolean
  /** 正在翻译（按钮置灰并显示进行中的文案） */
  translateBusy?: boolean
  /** 不可用（AI 未配置 / 这封信没有可翻译的文字），按钮置灰但仍可见——
      隐藏起来的话用户只会以为功能坏了，置灰配上 title 才说得清为什么 */
  translateDisabled?: boolean
  /** 按钮的悬浮说明，用来解释置灰的原因或已识别的源语言 */
  translateTitle?: string
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
  onTranslate,
  translateActive,
  translateBusy,
  translateDisabled,
  translateTitle,
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
            title={withShortcut(t('reader.prev'), KEY.prev)}
            aria-label={t('reader.prev')}
            aria-keyshortcuts={KEY.prev}
          >
            <Icon name="chevron-up" size={15} />
          </button>
          <button
            type="button"
            className="tb-btn tb-icon"
            onClick={() => onNext?.()}
            disabled={!onNext}
            title={withShortcut(t('reader.next'), KEY.next)}
            aria-label={t('reader.next')}
            aria-keyshortcuts={KEY.next}
          >
            <Icon name="chevron-down" size={15} />
          </button>
          <div className="tb-sep" />
        </>
      )}

      {/* 回复 */}
      {onReply && (
        <button
          type="button"
          className="tb-btn"
          onClick={onReply}
          title={withShortcut(t('reader.reply'), KEY.reply)}
          aria-keyshortcuts={KEY.reply}
        >
          <Icon name="reply" size={14} />
          <span>{t('reader.reply')}</span>
        </button>
      )}
      {/* 转发 */}
      {onForward && (
        <button
          type="button"
          className="tb-btn"
          onClick={onForward}
          title={withShortcut(t('reader.forward'), KEY.forward)}
          aria-keyshortcuts={KEY.forward}
        >
          <Icon name="forward" size={14} />
          <span>{t('reader.forward')}</span>
        </button>
      )}

      <div className="tb-sep" />

      {/* 翻译：自成一组放在回复/转发之后。
          不塞进「更多」菜单，是因为它在外文邮件上是**每封都要点**的动作，
          藏进二级菜单等于每封信多点一次。 */}
      {onTranslate && (
        <>
          <button
            type="button"
            className={'tb-btn' + (translateActive ? ' is-active' : '')}
            onClick={onTranslate}
            disabled={translateDisabled || translateBusy}
            title={translateTitle}
            aria-pressed={translateActive}
          >
            <Icon name="languages" size={14} />
            <span>
              {translateBusy
                ? t('reader.translating')
                : translateActive
                  ? t('reader.showOriginal')
                  : t('reader.translate')}
            </span>
          </button>
          <div className="tb-sep" />
        </>
      )}

      {/* 一键归档（仅当账户有归档文件夹、且当前不在归档里时出现）*/}
      {onArchive && (
        <button
          type="button"
          className="tb-btn"
          onClick={onArchive}
          title={withShortcut(t('reader.archive'), KEY.archive)}
          aria-keyshortcuts={KEY.archive}
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
        title={withShortcut(t('reader.delete'), KEY.delete)}
        aria-keyshortcuts={KEY.delete}
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

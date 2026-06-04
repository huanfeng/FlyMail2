// 快捷键速查浮层（`?` 触发）
// 数据源：lib/shortcuts.ts getShortcutGroups()，与设置内键位表共用同一目录。
// 关闭：点击遮罩 / 关闭按钮 / Esc（Esc 由全局 useKeyboardShortcuts 统一处理，见 Shell）。

import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { getShortcutGroups } from '@/lib/shortcuts'

interface Props {
  onClose: () => void
}

export function ShortcutsCheatsheet({ onClose }: Props) {
  const { t } = useTranslation()
  const groups = getShortcutGroups()

  return (
    <div
      className="shortcuts-backdrop"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div
        className="shortcuts-card"
        role="dialog"
        aria-modal="true"
        aria-label={t('shortcuts.title')}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {/* 头部：标题 + 关闭 */}
        <div className="sc-head">
          <h2>{t('shortcuts.title')}</h2>
          <button
            type="button"
            className="icon-btn"
            onClick={onClose}
            title={t('account.cancel')}
            aria-label={t('account.cancel')}
          >
            <Icon name="close" size={14} />
          </button>
        </div>

        {/* 分组键位表 */}
        <div className="sc-groups">
          {groups.map((g) => (
            <section key={g.id} className="sc-group">
              <h3>{t(g.titleKey)}</h3>
              <div className="sc-rows">
                {g.items.map((it) => (
                  <div key={it.id} className="sc-row">
                    <span className="sc-keys">
                      {it.keys.map((k) => (
                        <kbd key={k} className="sc-kbd">{k}</kbd>
                      ))}
                    </span>
                    <span className="sc-desc">{t(it.descKey)}</span>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>

        {/* 脚注提示 */}
        <div className="sc-foot">{t('shortcuts.hint')}</div>
      </div>
    </div>
  )
}

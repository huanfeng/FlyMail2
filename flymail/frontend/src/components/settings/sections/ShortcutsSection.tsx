// 设置 → 快捷键：静态键位表。

import { useTranslation } from 'react-i18next'
import { getShortcutGroups } from '@/lib/shortcuts'

export function ShortcutsSection() {
  const { t } = useTranslation()
  // 键位目录复用全局单一真相源（与 `?` 速查浮层同一数据源）。
  const groups = getShortcutGroups()
  return (
    <div className="settings-block">
      <h3>{t('settings.shortcuts.title')}</h3>
      <p className="help">{t('shortcuts.hint')}</p>
      <div className="sc-groups" style={{ marginTop: 10 }}>
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
    </div>
  )
}

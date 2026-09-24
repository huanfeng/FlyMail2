// 设置 → 关于。

import { useTranslation } from 'react-i18next'

export function AboutSection() {
  const { t } = useTranslation()
  return (
    <div className="settings-block">
      <h3>{t('settings.about.title')}</h3>
      <p className="help">{t('settings.about.desc')}</p>
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--ink-3)', marginTop: 16 }}>
        {t('settings.about.version')}
      </div>
    </div>
  )
}

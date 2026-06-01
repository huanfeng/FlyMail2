import { useTranslation } from 'react-i18next'

export function Reader() {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
      <p className="text-sm" style={{ color: 'var(--ink-2)' }}>{t('reader.welcome')}</p>
      <p className="text-xs" style={{ color: 'var(--ink-3)' }}>{t('reader.notReady')}</p>
    </div>
  )
}

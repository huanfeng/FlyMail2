import { useTranslation } from 'react-i18next'

export function AccountsSection() {
  const { t } = useTranslation()
  return (
    <div className="p-4 text-sm" style={{ color: 'var(--ink-3)' }}>
      {t('settings.accountsPlaceholder')}
    </div>
  )
}

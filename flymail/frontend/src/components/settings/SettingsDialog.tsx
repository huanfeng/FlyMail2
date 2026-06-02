import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { AccountsSection } from './AccountsSection'
import { GeneralSection } from './GeneralSection'
import { SecuritySection } from './SecuritySection'
import { MailSection } from './MailSection'
import type { ListStyle } from '@/lib/list-prefs'

// ────────────────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────────────────

type Section = 'accounts' | 'general' | 'security' | 'mail'

export interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 当前列表样式（由 Shell 管理，传给 MailSection 即时生效） */
  listStyle?: ListStyle
  onChangeListStyle?: (style: ListStyle) => void
}

// ────────────────────────────────────────────────────────────────────────────────
// NavItem
// ────────────────────────────────────────────────────────────────────────────────

interface NavItemProps {
  label: string
  active: boolean
  onClick: () => void
}

function NavItem({ label, active, onClick }: NavItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full text-left px-3 py-2 rounded-md text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring"
      style={{
        background: active ? 'var(--accent-wash)' : 'transparent',
        color: active ? 'var(--accent-color)' : 'var(--ink-2)',
        fontWeight: active ? 500 : 400,
      }}
    >
      {label}
    </button>
  )
}

// ────────────────────────────────────────────────────────────────────────────────
// SettingsDialog
// ────────────────────────────────────────────────────────────────────────────────

export function SettingsDialog({ open, onOpenChange, listStyle, onChangeListStyle }: SettingsDialogProps) {
  const { t } = useTranslation()
  const [section, setSection] = React.useState<Section>('accounts')

  const navItems: Array<{ key: Section; label: string }> = [
    { key: 'accounts', label: t('settings.navAccounts') },
    { key: 'general', label: t('settings.navGeneral') },
    { key: 'security', label: t('settings.navSecurity') },
    { key: 'mail', label: t('settings.navMail') },
  ]

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        {/* 遮罩 */}
        <Dialog.Overlay
          className="fixed inset-0 z-40"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />

        {/* 对话框内容 */}
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 w-[720px] max-w-[calc(100vw-2rem)] rounded-xl shadow-xl flex flex-col outline-none overflow-hidden"
          style={{
            background: 'var(--surface)',
            color: 'var(--ink)',
            height: '80vh',
            maxHeight: '80vh',
          }}
          aria-describedby={undefined}
        >
          {/* 标题栏 */}
          <div
            className="flex items-center justify-between px-6 py-4 shrink-0"
            style={{ borderBottom: '1px solid var(--rule)' }}
          >
            <Dialog.Title
              className="text-base font-semibold"
              style={{ margin: 0 }}
            >
              {t('settings.title')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('account.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          {/* 主体：左侧导航 + 右侧内容 */}
          <div className="flex flex-1 min-h-0">
            {/* 左侧导航 */}
            <nav
              className="w-44 shrink-0 flex flex-col gap-0.5 p-3 overflow-y-auto"
              style={{ borderRight: '1px solid var(--rule)' }}
            >
              {navItems.map((item) => (
                <NavItem
                  key={item.key}
                  label={item.label}
                  active={section === item.key}
                  onClick={() => setSection(item.key)}
                />
              ))}
            </nav>

            {/* 右侧内容 */}
            <div className="flex-1 overflow-y-auto">
              {section === 'accounts' && <AccountsSection />}
              {section === 'general' && <GeneralSection />}
              {section === 'security' && <SecuritySection />}
              {section === 'mail' && (
                <MailSection
                  listStyle={listStyle}
                  onChangeListStyle={onChangeListStyle}
                />
              )}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

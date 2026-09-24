// 设置页注册表：分组、页面、图标与渲染，导航与内容都由它生成。
//
// ── 为什么要一张表 ─────────────────────────────────────────────────────────
//
// 此前导航是一个数组、内容是一串 `section === 'x' && <X/>`，加一页要在两处各补一段，
// 漏了哪处都不会报错——只会多出一个点了没内容的导航项，或者一个永远进不去的页面。
// 现在加一页只需在这里加一行。
//
// ── 分组的依据 ─────────────────────────────────────────────────────────────
//
// 按「谁在什么时候来改」：个人偏好常改；邮箱行为偶尔改；集成与系统配一次就不再动。
// 设计记录见 docs/flymail/ai-providers-and-settings-ia.md §2。

import type * as React from 'react'
import type { IconName } from '@/components/ui/Icon'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import { AISection } from './AISection'
import { MonitoringSection } from './MonitoringSection'
import { OAuthSection } from './OAuthSection'
import { AboutSection } from './sections/AboutSection'
import { AccountsSection } from './sections/AccountsSection'
import { AppearanceSection } from './sections/AppearanceSection'
import { ComposeSection } from './sections/ComposeSection'
import { FiltersSection } from './sections/FiltersSection'
import { NotifySection } from './sections/NotifySection'
import { ProfileSection } from './sections/ProfileSection'
import { ReadingSection } from './sections/ReadingSection'
import { ServerSection } from './sections/ServerSection'
import { ShortcutsSection } from './sections/ShortcutsSection'
import { SyncSection } from './sections/SyncSection'
import type { SettingsPageId } from '@/lib/settings-nav'

/** 由 Shell 管理、改动需立即作用于主界面的那几项偏好 */
export interface SettingsShellProps {
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
  conversationView: boolean
  onChangeConversationView: (on: boolean) => void
  alwaysShowSelect: boolean
  onChangeAlwaysShowSelect: (on: boolean) => void
  layoutMode: LayoutMode
  onChangeLayoutMode: (mode: LayoutMode) => void
}

export interface SettingsPage {
  id: SettingsPageId
  labelKey: string
  icon: IconName
  render: (p: SettingsShellProps) => React.ReactNode
}

export interface SettingsGroup {
  id: string
  labelKey: string
  pages: SettingsPage[]
}

export const SETTINGS_GROUPS: readonly SettingsGroup[] = [
  {
    id: 'personal',
    labelKey: 'settings.group.personal',
    pages: [
      { id: 'profile', labelKey: 'settings.navProfileSecurity', icon: 'user', render: () => <ProfileSection /> },
      {
        id: 'appearance',
        labelKey: 'settings.navAppearance',
        icon: 'sun',
        render: (p) => (
          <AppearanceSection
            listStyle={p.listStyle}
            onChangeListStyle={p.onChangeListStyle}
            alwaysShowSelect={p.alwaysShowSelect}
            onChangeAlwaysShowSelect={p.onChangeAlwaysShowSelect}
            layoutMode={p.layoutMode}
            onChangeLayoutMode={p.onChangeLayoutMode}
          />
        ),
      },
      { id: 'shortcuts', labelKey: 'settings.navShortcuts', icon: 'help', render: () => <ShortcutsSection /> },
    ],
  },
  {
    id: 'mailbox',
    labelKey: 'settings.group.mailbox',
    pages: [
      { id: 'accounts', labelKey: 'settings.navAccounts', icon: 'inbox', render: () => <AccountsSection /> },
      {
        id: 'reading',
        labelKey: 'settings.navReading',
        icon: 'eye',
        render: (p) => (
          <ReadingSection
            conversationView={p.conversationView}
            onChangeConversationView={p.onChangeConversationView}
          />
        ),
      },
      { id: 'compose', labelKey: 'settings.navCompose', icon: 'compose', render: () => <ComposeSection /> },
      { id: 'filters', labelKey: 'settings.navFilters', icon: 'filter', render: () => <FiltersSection /> },
      { id: 'sync', labelKey: 'settings.navSync', icon: 'cloud', render: () => <SyncSection /> },
      // 通知不单独成组：一组只有一页时，组名和页名是同一个词，读起来像排版错误
      { id: 'notify', labelKey: 'settings.navNotify', icon: 'bell', render: () => <NotifySection /> },
    ],
  },
  {
    id: 'integrations',
    labelKey: 'settings.group.integrations',
    pages: [
      { id: 'ai', labelKey: 'settings.navAI', icon: 'languages', render: () => <AISection /> },
      // 自成一页而不是挂在「账户」末尾：那里是「我有哪些邮箱」，
      // 这里是「这台部署允许用哪些方式登录邮箱」——一次性的部署配置，
      // 而且带着大段的外部操作说明。
      { id: 'oauth', labelKey: 'settings.navOAuth', icon: 'shield', render: () => <OAuthSection /> },
    ],
  },
  {
    id: 'system',
    labelKey: 'settings.group.system',
    pages: [
      { id: 'server', labelKey: 'settings.navServer', icon: 'settings', render: () => <ServerSection /> },
      { id: 'monitoring', labelKey: 'settings.navMonitoring', icon: 'circle-dot', render: () => <MonitoringSection /> },
      { id: 'about', labelKey: 'settings.navAbout', icon: 'more', render: () => <AboutSection /> },
    ],
  },
]

/** 全部页面（按导航顺序） */
export const SETTINGS_PAGES: readonly SettingsPage[] = SETTINGS_GROUPS.flatMap((g) => g.pages)

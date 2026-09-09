// 设置弹框组件（modal）
// 参考蓝本：.dev/mailmaster/src_extracted/06_87910dfb.js (SettingsScreen + THEMES_LIST)
// 原版设置是覆盖层弹框（.settings-backdrop > .settings-dialog），左侧分栏导航 + 右侧内容。
// 所有颜色严格使用 CSS 令牌，不写死任何颜色值。复用现有 Section 子组件数据逻辑。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { NotifyChannelsSection } from '@/components/settings/NotifyChannelsSection'
import { MonitoringSection } from '@/components/settings/MonitoringSection'
import { RulesSection } from '@/components/settings/RulesSection'
import { BlocklistSection } from '@/components/settings/BlocklistSection'
import { TrustedSendersSection } from '@/components/settings/TrustedSendersSection'
import { SignatureSection } from '@/components/settings/SignatureSection'
import { AliasesSection } from '@/components/settings/AliasesSection'
import { getTheme, applyTheme, TONES } from '@/lib/theme'
import { getShortcutGroups } from '@/lib/shortcuts'
import { setListStyle } from '@/lib/list-prefs'
import { getRemoteImageDefault, setRemoteImageDefault } from '@/lib/privacy-prefs'
import { LAYOUT_LIMITS, loadLayoutWidths, saveLayoutWidths } from '@/lib/layout-prefs'
import type { LayoutWidths } from '@/lib/layout-prefs'
import {
  useAccounts,
  useSettings,
  useUpdateSettings,
  useDeleteAccount,
  useSetAccountEnabled,
  useTriggerSync,
  useSyncStatus,
  useChangePassword,
  useAccountStats,
  useMe,
  useUpdateProfile,
  useReindexSearch,
  useRebuildThreads,
} from '@/lib/queries'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import type { Account, BodySyncMode, SyncPhase } from '@/lib/types'

// ── 常量 ─────────────────────────────────────────────────
const SYNC_DEPTH_MIN = 100
const SYNC_DEPTH_MAX = 5000
const POLL_INTERVAL_MIN = 30
const POLL_INTERVAL_MAX = 3600
// 正文预取的天数窗口上下限（与后端 body_sync_recent_days 校验一致）
const BODY_DAYS_MIN = 1
const BODY_DAYS_MAX = 3650

/** 主题预览色表，来自蓝本 THEME_PREVIEW */
const THEME_PREVIEW: Record<string, { l: { bg: string; side: string; accent: string }; d: { bg: string; side: string; accent: string } }> = {
  warm:     { l: { bg: '#fbfaf7', side: '#f5f3ee', accent: '#b5886b' }, d: { bg: '#1f1d18', side: '#26231d', accent: '#d2a482' } },
  sky:      { l: { bg: '#f8fafc', side: '#f1f5f9', accent: '#4a86c2' }, d: { bg: '#161a21', side: '#1c2029', accent: '#7aaad8' } },
  lavender: { l: { bg: '#faf9fc', side: '#f3f1f7', accent: '#8d6cc4' }, d: { bg: '#1c1925', side: '#221f2d', accent: '#b593e0' } },
  coral:    { l: { bg: '#fdf9f7', side: '#f8f0ec', accent: '#d27a63' }, d: { bg: '#1f1917', side: '#261f1c', accent: '#e89a84' } },
  slate:    { l: { bg: '#f7f8fa', side: '#eef0f3', accent: '#5b6470' }, d: { bg: '#14161a', side: '#1a1d22', accent: '#b6bdc8' } },
  mint:     { l: { bg: '#f4faf6', side: '#e7f3eb', accent: '#3d9970' }, d: { bg: '#131a16', side: '#18211c', accent: '#6dc99c' } },
  butter:   { l: { bg: '#fefbf3', side: '#faf3df', accent: '#d4a72c' }, d: { bg: '#1c1a13', side: '#232017', accent: '#e8c560' } },
  rose:     { l: { bg: '#fdf6f8', side: '#f9e9ee', accent: '#d6628a' }, d: { bg: '#1d1518', side: '#251b1f', accent: '#e88aac' } },
  aqua:     { l: { bg: '#f3fafc', side: '#e3f2f6', accent: '#2ba9b5' }, d: { bg: '#0f1a1c', side: '#142225', accent: '#6cd1d8' } },
}

/** 设置分区 ID */
type SettingSection = 'profile' | 'appearance' | 'general' | 'accounts' | 'mail' | 'signature' | 'aliases' | 'rules' | 'blocklist' | 'privacy' | 'notify' | 'monitoring' | 'security' | 'shortcuts' | 'about'

// ── Props ─────────────────────────────────────────────────
interface SettingsDialogProps {
  /** 当前列表样式（Shell 管理），使改动立即对邮件列表生效 */
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
  /** 会话视图开关（Shell 管理，改动立即生效） */
  conversationView: boolean
  onChangeConversationView: (on: boolean) => void
  /** 行内选择框是否常显（Shell 管理，改动立即生效） */
  alwaysShowSelect: boolean
  onChangeAlwaysShowSelect: (on: boolean) => void
  /** 当前布局模式（三栏 / 双栏浮动阅读） */
  layoutMode: LayoutMode
  onChangeLayoutMode: (mode: LayoutMode) => void
  /** 关闭弹框的回调 */
  onClose: () => void
}

// ════════════════════════════════════════════════════════════
// 子组件：主题预览卡片
// ════════════════════════════════════════════════════════════

interface ThemeCardProps {
  id: string
  label: string
  mode: ThemeMode
  active: boolean
  onClick: () => void
}

function ThemeCard({ id, label, mode, active, onClick }: ThemeCardProps) {
  const p = THEME_PREVIEW[id]?.[mode === 'dark' ? 'd' : 'l']
  if (!p) return null
  return (
    <button
      type="button"
      className={'theme-card' + (active ? ' active' : '')}
      onClick={onClick}
      aria-pressed={active}
      aria-label={label}
    >
      {/* 颜色预览区：侧栏色 + 主区底色 + accent 条 + 模拟文本线 */}
      <div className="tc-preview" style={{ background: p.bg }}>
        <div className="tc-side" style={{ background: p.side }} />
        <div className="tc-main" style={{ background: p.bg }}>
          <div className="tc-accent" style={{ background: p.accent }} />
          <div className="tc-line" style={{ width: '70%' }} />
          <div className="tc-line" style={{ width: '50%' }} />
        </div>
      </div>
      {/* 脚部：名称 + 亮/暗标签 */}
      <div className="tc-foot">
        <span className="tc-name">{label}</span>
        <span className="tc-mode">{mode === 'dark' ? 'dark' : 'light'}</span>
      </div>
    </button>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：Toggle 开关
// ════════════════════════════════════════════════════════════

interface ToggleProps {
  on: boolean
  onChange: (next: boolean) => void
  ariaLabel?: string
}

function Toggle({ on, onChange, ariaLabel }: ToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={ariaLabel}
      className={'toggle' + (on ? ' on' : '')}
      onClick={() => onChange(!on)}
    />
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：设置行
// ════════════════════════════════════════════════════════════

interface RowProps {
  label: string
  help?: string
  children: React.ReactNode
}

function Row({ label, help, children }: RowProps) {
  return (
    <div className="settings-row">
      <div>
        <div className="sr-label">{label}</div>
        {help && <div className="sr-help">{help}</div>}
      </div>
      <div>{children}</div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：外观分区
// ════════════════════════════════════════════════════════════

interface AppearanceSectionProps {
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
  alwaysShowSelect: boolean
  onChangeAlwaysShowSelect: (on: boolean) => void
  layoutMode: LayoutMode
  onChangeLayoutMode: (mode: LayoutMode) => void
}

function AppearanceSection({
  listStyle,
  onChangeListStyle,
  alwaysShowSelect,
  onChangeAlwaysShowSelect,
  layoutMode,
  onChangeLayoutMode,
}: AppearanceSectionProps) {
  const { t } = useTranslation()
  const initial = getTheme()
  const [currentMode, setCurrentMode] = React.useState<ThemeMode>(initial.mode)
  const [currentTone, setCurrentTone] = React.useState<ToneId>(initial.tone)
  // 栏宽（与三栏拖拽共用 layout-prefs）
  const [widths, setWidths] = React.useState<LayoutWidths>(() => loadLayoutWidths())

  /** 切换色调（同时保留当前亮/暗） */
  function handleTone(tone: ToneId) {
    applyTheme({ mode: currentMode, tone })
    setCurrentTone(tone)
  }

  /** 切换亮/暗模式（同时保留当前色调，重新绘制卡片预览） */
  function handleMode(mode: ThemeMode) {
    applyTheme({ mode, tone: currentTone })
    setCurrentMode(mode)
  }

  /** 调整栏宽：本地 state + 写 localStorage + 广播（AppLayout 即时同步） */
  function handleWidth(key: keyof LayoutWidths, value: number) {
    const next = { ...widths, [key]: value }
    setWidths(next)
    saveLayoutWidths(next)
  }

  function handleListStyle(style: ListStyle) {
    setListStyle(style)
    onChangeListStyle(style)
  }

  return (
    <>
      {/* 主题卡片区 */}
      <div className="settings-block">
        <h3>{t('settings.page.theme')}</h3>
        <p className="help">{t('settings.page.themeHelp')}</p>
        <div className="theme-grid-large">
          {TONES.map((tone) => (
            <ThemeCard
              key={tone.id}
              id={tone.id}
              label={t(tone.nameKey)}
              mode={currentMode}
              active={tone.id === currentTone}
              onClick={() => handleTone(tone.id)}
            />
          ))}
        </div>
      </div>

      {/* 布局模式：三栏 / 双栏 + 右侧浮动阅读 */}
      <div className="settings-block">
        <h3>{t('settings.page.layout')}</h3>
        <p className="help">{t('settings.page.layoutHelp')}</p>
        <div className="layout-swatches">
          <button
            type="button"
            className={'layout-sw' + (layoutMode === 'three' ? ' active' : '')}
            onClick={() => onChangeLayoutMode('three')}
          >
            <svg viewBox="0 0 52 32" width="52" height="32">
              <rect x="1" y="1" width="12" height="30" rx="2" fill="var(--bg-alt)" stroke="var(--rule)" />
              <rect x="15" y="1" width="16" height="30" rx="2" fill="var(--bg-alt)" stroke="var(--rule)" />
              <rect x="33" y="1" width="18" height="30" rx="2" fill="var(--surface)" stroke="var(--rule)" />
            </svg>
            <span>{t('settings.page.layoutThree')}</span>
          </button>
          <button
            type="button"
            className={'layout-sw' + (layoutMode === 'two-slide' ? ' active' : '')}
            onClick={() => onChangeLayoutMode('two-slide')}
          >
            <svg viewBox="0 0 52 32" width="52" height="32">
              <rect x="1" y="1" width="12" height="30" rx="2" fill="var(--bg-alt)" stroke="var(--rule)" />
              <rect x="15" y="1" width="36" height="30" rx="2" fill="var(--bg-alt)" stroke="var(--rule)" />
              <rect x="31" y="3" width="20" height="28" rx="2" fill="var(--surface)" stroke="var(--accent)" strokeWidth="1.2" />
            </svg>
            <span>{t('settings.page.layoutTwoSlide')}</span>
          </button>
        </div>
      </div>

      {/* 亮/暗模式 + 栏宽 + 列表密度 */}
      <div className="settings-block">
        <h3>{t('settings.page.mode')}</h3>
        <Row label={t('settings.page.colorMode')}>
          <div className="mode-toggle">
            <button
              type="button"
              className={currentMode === 'light' ? 'active' : ''}
              onClick={() => handleMode('light')}
            >
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                <Icon name="sun" size={12} />
                {t('settings.general.light')}
              </span>
            </button>
            <button
              type="button"
              className={currentMode === 'dark' ? 'active' : ''}
              onClick={() => handleMode('dark')}
            >
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                <Icon name="moon" size={12} />
                {t('settings.general.dark')}
              </span>
            </button>
          </div>
        </Row>

        {/* 侧栏宽度滑块（也可直接拖拽栏间分隔线） */}
        <Row label={t('settings.page.sidebarWidth')} help={t('settings.page.widthHint')}>
          <div className="slider-row" style={{ width: 220 }}>
            <input
              type="range"
              min={LAYOUT_LIMITS.sidebar.min}
              max={LAYOUT_LIMITS.sidebar.max}
              step={4}
              value={widths.sidebar}
              onChange={(e) => handleWidth('sidebar', Number(e.target.value))}
            />
            <span className="slider-val">{widths.sidebar}px</span>
          </div>
        </Row>

        {/* 列表宽度滑块 */}
        <Row label={t('settings.page.listWidth')}>
          <div className="slider-row" style={{ width: 220 }}>
            <input
              type="range"
              min={LAYOUT_LIMITS.list.min}
              max={LAYOUT_LIMITS.list.max}
              step={4}
              value={widths.list}
              onChange={(e) => handleWidth('list', Number(e.target.value))}
            />
            <span className="slider-val">{widths.list}px</span>
          </div>
        </Row>

        {/* 发件人列宽（仅紧凑列表样式生效）：调窄它就是把宽度让给主题 */}
        <Row label={t('settings.page.senderColWidth')} help={t('settings.page.senderColHint')}>
          <div className="slider-row" style={{ width: 220 }}>
            <input
              type="range"
              min={LAYOUT_LIMITS.senderCol.min}
              max={LAYOUT_LIMITS.senderCol.max}
              step={5}
              value={widths.senderCol}
              onChange={(e) => handleWidth('senderCol', Number(e.target.value))}
            />
            <span className="slider-val">{widths.senderCol}px</span>
          </div>
        </Row>

        {/* 列表密度（紧凑/卡片）*/}
        <Row label={t('settings.page.density')}>
          <div className="mode-toggle">
            {(['compact', 'card'] as ListStyle[]).map((style) => (
              <button
                key={style}
                type="button"
                className={listStyle === style ? 'active' : ''}
                onClick={() => handleListStyle(style)}
              >
                {style === 'compact' ? t('settings.mail.listCompact') : t('settings.mail.listCard')}
              </button>
            ))}
          </div>
        </Row>

        {/* 行内选择框常显（关闭时需先点工具栏的选择开关）*/}
        <Row label={t('settings.page.alwaysShowSelect')} help={t('settings.page.alwaysShowSelectHint')}>
          <Toggle on={alwaysShowSelect} onChange={onChangeAlwaysShowSelect} />
        </Row>
      </div>
    </>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：通用分区（语言 + 默认远程图片）
// ════════════════════════════════════════════════════════════

function GeneralSection() {
  const { t, i18n } = useTranslation()
  const currentLang = i18n.language.startsWith('zh') ? 'zh' : 'en'

  function handleLang(lng: string) {
    void i18n.changeLanguage(lng)
    localStorage.setItem('flymail_lang', lng)
  }

  return (
    <>
      <div className="settings-block">
        <h3>{t('settings.page.language')}</h3>
        <Row label={t('settings.page.uiLanguage')}>
          <div className="lang-toggle">
            <button
              type="button"
              className={currentLang === 'en' ? 'active' : ''}
              onClick={() => handleLang('en')}
            >
              English
            </button>
            <button
              type="button"
              className={currentLang === 'zh' ? 'active' : ''}
              onClick={() => handleLang('zh')}
            >
              中文
            </button>
          </div>
        </Row>
      </div>
    </>
  )
}

// ════════════════════════════════════════════════════════
// 子组件：隐私分区（M12）
// ════════════════════════════════════════════════════════

/**
 * 阅读隐私：远程图片默认开关 + 发件人信任名单。
 *
 * 开关从「通用」搬到这里而不是两处都放：它存在 localStorage 里，
 * 两个入口各自持一份 state 早晚会对不上，而隐私开关显示错值比不显示更糟。
 */
function PrivacySection() {
  const { t } = useTranslation()
  const [loadRemoteImages, setLoadRemoteImages] = React.useState<boolean>(() =>
    getRemoteImageDefault(),
  )

  function handleRemoteImages(next: boolean) {
    setLoadRemoteImages(next)
    // 写完就结束：开关是可订阅的（见 privacy-prefs），已挂载的 useMessageDetail
    // 会因此重新渲染、把 remote 换进 query key，新 key 自然去取新口径的正文。
    // ⚙ 不能在这里 invalidate ['message']：那一瞬间阅读器还没重渲染，失效的是旧 key，
    // 结果是按旧口径白白多打一趟网络。
    setRemoteImageDefault(next)
  }

  return (
    <>
      <div className="settings-block">
        <h3>{t('settings.privacy.reading')}</h3>
        <Row label={t('settings.privacy.remoteImages')} help={t('settings.privacy.remoteImagesHint')}>
          <Toggle on={loadRemoteImages} onChange={handleRemoteImages} />
        </Row>
      </div>

      <TrustedSendersSection />
    </>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：快捷键分区（静态键位表）
// ════════════════════════════════════════════════════════════

function ShortcutsSection() {
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

// ════════════════════════════════════════════════════════════
// 子组件：关于分区
// ════════════════════════════════════════════════════════════

function AboutSection() {
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

// ════════════════════════════════════════════════════════════
// 子组件：单个账户行（用于整页账户分区）
// ════════════════════════════════════════════════════════════

interface AccountCardRowProps {
  account: Account
  onEdit: () => void
  onDelete: () => void
}

function AccountCardRow({ account, onEdit, onDelete }: AccountCardRowProps) {
  const { t } = useTranslation()
  const [syncing, setSyncing] = React.useState(false)
  const setEnabled = useSetAccountEnabled()
  const triggerSync = useTriggerSync()
  const syncStatus = useSyncStatus(account.id, syncing)
  const statsQuery = useAccountStats(account.id)

  const syncPhase: SyncPhase = syncStatus.data?.phase ?? 'none'

  React.useEffect(() => {
    if (syncing && (syncPhase === 'done' || syncPhase === 'error')) {
      setSyncing(false)
    }
  }, [syncing, syncPhase])

  function handleSync() {
    setSyncing(true)
    triggerSync.mutate(account.id)
  }

  function handleToggle() {
    setEnabled.mutate({ id: account.id, enabled: !account.enabled })
  }

  /** 从名称取首字母（最多 2 个） */
  function initials(name: string): string {
    return name
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0] ?? '')
      .join('')
      .toUpperCase()
  }

  const stats = statsQuery.data

  return (
    <div className="account-card">
      {/* 头像 */}
      <div
        className="ac-avatar"
        style={{ background: 'var(--accent)' }}
        aria-hidden="true"
      >
        {initials(account.name || account.email)}
      </div>

      {/* 名称 + 邮箱 */}
      <div style={{ minWidth: 0 }}>
        <div className="ac-name">{account.name || account.email}</div>
        <div className="ac-mail">{account.email}</div>
        {stats && (
          <div style={{ fontSize: 11, color: 'var(--ink-3)', marginTop: 3, fontFamily: 'var(--font-mono)' }}>
            {t('settings.account.messages')}: {stats.message_count}
            &nbsp;·&nbsp;
            {t('settings.account.folders')}: {stats.folder_count}
          </div>
        )}
      </div>

      {/* 右侧操作区 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
        {/* 状态徽标 */}
        <span
          className={'ac-status' + (account.enabled ? ' live' : '')}
        >
          {account.enabled ? t('settings.account.enabled') : t('settings.account.disabled')}
        </span>

        {/* 启/停 toggle */}
        <Toggle
          on={account.enabled}
          onChange={handleToggle}
          ariaLabel={account.enabled ? t('settings.account.disable') : t('settings.account.enable')}
        />

        {/* 立即同步 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.sync')}
          aria-label={t('settings.account.sync')}
          onClick={handleSync}
          disabled={syncing}
        >
          <Icon
            name="circle-dot"
            size={13}
            className={syncing ? 'spin-anim' : undefined}
          />
        </button>

        {/* 编辑 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.edit')}
          aria-label={t('settings.account.edit')}
          onClick={onEdit}
        >
          <Icon name="compose" size={13} />
        </button>

        {/* 删除 */}
        <button
          type="button"
          className="icon-btn"
          title={t('settings.account.delete')}
          aria-label={t('settings.account.delete')}
          onClick={onDelete}
          style={{ color: 'var(--destructive)' }}
        >
          <Icon name="trash" size={13} />
        </button>
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：账户分区
// ════════════════════════════════════════════════════════════

function AccountsSection() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const deleteAccount = useDeleteAccount()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<Account | null>(null)

  function handleAdd() {
    setEditingAccount(null)
    setDialogOpen(true)
  }

  function handleEdit(account: Account) {
    setEditingAccount(account)
    setDialogOpen(true)
  }

  function handleDelete(account: Account) {
    if (window.confirm(t('settings.account.deleteConfirm'))) {
      deleteAccount.mutate(account.id)
    }
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.navAccounts')}</h3>
      <p className="help">{t('settings.page.accountsHelp')}</p>

      {/* 账户卡片列表 */}
      {accounts.length === 0 ? (
        <div style={{ color: 'var(--ink-3)', fontSize: 13, padding: '12px 0' }}>
          {t('settings.account.none')}
        </div>
      ) : (
        accounts.map((account) => (
          <AccountCardRow
            key={account.id}
            account={account}
            onEdit={() => handleEdit(account)}
            onDelete={() => handleDelete(account)}
          />
        ))
      )}

      {/* 添加账户按钮 */}
      <button
        type="button"
        className="pill-btn"
        style={{ marginTop: 14 }}
        onClick={handleAdd}
      >
        <Icon name="plus" size={12} />
        {t('settings.account.add')}
      </button>

      {/* AccountDialog 复用 */}
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：邮件同步分区（同步深度 + 轮询间隔）
// ════════════════════════════════════════════════════════════

interface MailSectionProps {
  conversationView: boolean
  onChangeConversationView: (on: boolean) => void
}

function MailSection({ conversationView, onChangeConversationView }: MailSectionProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  const reindex = useReindexSearch()
  const rebuildThreads = useRebuildThreads()

  const [syncDepth, setSyncDepth] = React.useState<number>(settings?.sync_depth ?? 1000)
  const [pollInterval, setPollInterval] = React.useState<number>(settings?.sync_poll_interval ?? 180)
  const [bodyMode, setBodyMode] = React.useState<BodySyncMode>(settings?.body_sync_mode ?? 'new')
  const [bodyDays, setBodyDays] = React.useState<number>(settings?.body_sync_recent_days ?? 30)
  const [depthError, setDepthError] = React.useState<string | null>(null)
  const [intervalError, setIntervalError] = React.useState<string | null>(null)
  const [bodyDaysError, setBodyDaysError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)

  // 服务端数据加载后同步到本地
  React.useEffect(() => {
    if (settings?.sync_depth != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSyncDepth(settings.sync_depth)
    }
  }, [settings?.sync_depth])

  React.useEffect(() => {
    if (settings?.sync_poll_interval != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPollInterval(settings.sync_poll_interval)
    }
  }, [settings?.sync_poll_interval])

  React.useEffect(() => {
    if (settings?.body_sync_mode != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBodyMode(settings.body_sync_mode)
    }
  }, [settings?.body_sync_mode])

  React.useEffect(() => {
    if (settings?.body_sync_recent_days != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBodyDays(settings.body_sync_recent_days)
    }
  }, [settings?.body_sync_recent_days])

  function handleSave() {
    setDepthError(null)
    setIntervalError(null)
    setBodyDaysError(null)
    setSaved(false)

    if (syncDepth < SYNC_DEPTH_MIN || syncDepth > SYNC_DEPTH_MAX) {
      setDepthError(t('settings.mail.invalidDepth'))
      return
    }
    if (pollInterval < POLL_INTERVAL_MIN || pollInterval > POLL_INTERVAL_MAX) {
      setIntervalError(t('settings.mail.invalidInterval'))
      return
    }
    if (bodyMode === 'recent' && (bodyDays < BODY_DAYS_MIN || bodyDays > BODY_DAYS_MAX)) {
      setBodyDaysError(t('settings.mail.invalidBodyDays'))
      return
    }

    updateSettings.mutate(
      {
        sync_depth: String(syncDepth),
        sync_poll_interval: String(pollInterval),
        body_sync_mode: bodyMode,
        body_sync_recent_days: String(bodyDays),
      },
      {
        onSuccess: () => {
          setSaved(true)
          setTimeout(() => setSaved(false), 2500)
        },
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.mail.title')}</h3>

      {/* 同步深度 */}
      <Row
        label={t('settings.mail.syncDepth')}
        help={t('settings.mail.syncDepthHint')}
      >
        <div className="slider-row" style={{ width: 200 }}>
          <input
            type="range"
            min={SYNC_DEPTH_MIN}
            max={SYNC_DEPTH_MAX}
            step={100}
            value={syncDepth}
            onChange={(e) => { setDepthError(null); setSyncDepth(Number(e.target.value)) }}
          />
          <span className="slider-val">{syncDepth}</span>
        </div>
      </Row>
      {depthError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {depthError}
        </div>
      )}

      {/* 轮询间隔 */}
      <Row
        label={t('settings.mail.syncInterval')}
        help={t('settings.mail.syncIntervalHint')}
      >
        <div className="slider-row" style={{ width: 200 }}>
          <input
            type="range"
            min={POLL_INTERVAL_MIN}
            max={POLL_INTERVAL_MAX}
            step={30}
            value={pollInterval}
            onChange={(e) => { setIntervalError(null); setPollInterval(Number(e.target.value)) }}
          />
          <span className="slider-val">{pollInterval}s</span>
        </div>
      </Row>
      {intervalError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {intervalError}
        </div>
      )}

      {/* 正文同步范围：决定同步时把哪些邮件的正文一并下载到本地 */}
      <Row
        label={t('settings.mail.bodySync')}
        help={t('settings.mail.bodySyncHint')}
      >
        <div className="mode-toggle">
          {(['new', 'recent', 'all'] as BodySyncMode[]).map((mode) => (
            <button
              key={mode}
              type="button"
              className={bodyMode === mode ? 'active' : ''}
              onClick={() => { setBodyDaysError(null); setBodyMode(mode) }}
            >
              {t(`settings.mail.bodySync_${mode}`)}
            </button>
          ))}
        </div>
      </Row>

      {/* 天数窗口只在「最近」档有意义 */}
      {bodyMode === 'recent' && (
        <Row label={t('settings.mail.bodyDays')} help={t('settings.mail.bodyDaysHint')}>
          <div className="slider-row" style={{ width: 200 }}>
            <input
              type="range"
              min={BODY_DAYS_MIN}
              max={365}
              step={5}
              value={bodyDays}
              onChange={(e) => { setBodyDaysError(null); setBodyDays(Number(e.target.value)) }}
            />
            <span className="slider-val">{t('settings.mail.daysValue', { count: bodyDays })}</span>
          </div>
        </Row>
      )}
      {bodyDaysError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {bodyDaysError}
        </div>
      )}

      {/* 保存按钮 */}
      <div style={{ marginTop: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <button
          type="button"
          className="pill-btn"
          onClick={handleSave}
          disabled={updateSettings.isPending}
        >
          {t('settings.mail.save')}
        </button>
        {saved && (
          <span style={{ fontSize: 13, color: 'var(--accent)' }}>
            {t('settings.mail.saved')}
          </span>
        )}
      </div>

      {/* 会话视图：纯前端偏好，改完立即生效，不需要点「保存」——
          因此放在保存按钮之下、用分隔线与同步偏好隔开，避免被误当成要保存的一项。 */}
      <div style={{ marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--rule)' }}>
        <Row
          label={t('settings.mail.conversationView')}
          help={t('settings.mail.conversationViewHint')}
        >
          <Toggle on={conversationView} onChange={onChangeConversationView} />
        </Row>
      </div>

      {/* 重建搜索索引：全文索引与邮件表失配时的兜底，与上面的同步偏好无关，
          因此单独用一条分隔线隔开，避免被误当成「保存」的一部分。 */}
      <div style={{ marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--rule)' }}>
        <Row label={t('settings.mail.reindex')} help={t('settings.mail.reindexHint')}>
          <button
            type="button"
            className="pill-btn"
            onClick={() => {
              reindex.mutate(undefined, {
                onSuccess: () => toast(t('settings.mail.reindexDone')),
                onError: () => toast(t('settings.mail.reindexFailed')),
              })
            }}
            disabled={reindex.isPending}
          >
            {reindex.isPending ? t('settings.mail.reindexing') : t('settings.mail.reindexAction')}
          </button>
        </Row>

        {/* 重建会话归属：老库里的邮件没有 In-Reply-To/References 头，
            只有跑一趟按主题兜底的重放才能把它们并成会话。 */}
        <Row label={t('settings.mail.rebuildThreads')} help={t('settings.mail.rebuildThreadsHint')}>
          <button
            type="button"
            className="pill-btn"
            onClick={() => {
              rebuildThreads.mutate(undefined, {
                onSuccess: (n) => toast(t('settings.mail.rebuildThreadsDone', { count: n })),
                onError: () => toast(t('settings.mail.rebuildThreadsFailed')),
              })
            }}
            disabled={rebuildThreads.isPending}
          >
            {rebuildThreads.isPending
              ? t('settings.mail.rebuildingThreads')
              : t('settings.mail.rebuildThreadsAction')}
          </button>
        </Row>
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：资料分区（管理员展示名 / 邮箱）
// ════════════════════════════════════════════════════════════

/** 从名称取首字母（最多 2 个），用于头像占位 */
function nameInitials(name: string): string {
  const s = name.trim()
  if (!s) return '?'
  return s
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0] ?? '')
    .join('')
    .toUpperCase()
}

function ProfileSection() {
  const { t, i18n } = useTranslation()
  const { toast } = useToast()
  const { data: me } = useMe()
  const updateProfile = useUpdateProfile()

  const [displayName, setDisplayName] = React.useState('')
  const [email, setEmail] = React.useState('')

  // 资料加载后填充表单
  React.useEffect(() => {
    if (me) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDisplayName(me.display_name)
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setEmail(me.email)
    }
  }, [me])

  /** 本地化日期；无值回落到「从未」 */
  function fmtDate(s?: string): string {
    if (!s) return t('settings.account.never')
    try {
      return new Date(s).toLocaleString(i18n.language)
    } catch {
      return s
    }
  }

  function handleSave() {
    updateProfile.mutate(
      { display_name: displayName, email },
      { onSuccess: () => toast(t('settings.profile.saved')) },
    )
  }

  const avatarName = (displayName || me?.username || '').trim()

  return (
    <div className="settings-block">
      <h3>{t('settings.profile.title')}</h3>
      <p className="help">{t('settings.profile.help')}</p>

      {/* 头像 + 用户名概览 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 14, margin: '12px 0 6px' }}>
        <div
          className="ac-avatar"
          style={{ background: 'var(--accent)', width: 48, height: 48, fontSize: 18 }}
          aria-hidden="true"
        >
          {nameInitials(avatarName)}
        </div>
        <div style={{ minWidth: 0 }}>
          <div className="ac-name" style={{ fontSize: 15 }}>{me?.username ?? '—'}</div>
          <div className="ac-mail">{me?.email || t('settings.profile.noEmail')}</div>
        </div>
      </div>

      <div style={{ maxWidth: 380, marginTop: 8 }}>
        {/* 用户名（只读，登录账号不可改）*/}
        <Row label={t('settings.profile.username')} help={t('settings.profile.usernameHint')}>
          <input type="text" value={me?.username ?? ''} readOnly disabled className="inline-input" />
        </Row>

        {/* 展示名 */}
        <Row label={t('settings.profile.displayName')}>
          <input
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder={me?.username ?? ''}
            className="inline-input"
          />
        </Row>

        {/* 联系邮箱 */}
        <Row label={t('settings.profile.email')}>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            className="inline-input"
          />
        </Row>

        {/* 元信息：创建时间 / 最后登录 */}
        <div style={{ fontSize: 12, color: 'var(--ink-3)', fontFamily: 'var(--font-mono)', marginTop: 4, lineHeight: 1.7 }}>
          <div>{t('settings.profile.created')}: {fmtDate(me?.created_at)}</div>
          <div>{t('settings.profile.lastLogin')}: {fmtDate(me?.last_login_at)}</div>
        </div>

        <div style={{ marginTop: 16 }}>
          <button
            type="button"
            className="pill-btn"
            onClick={handleSave}
            disabled={updateProfile.isPending}
          >
            {t('settings.profile.save')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：安全分区（改密码）
// ════════════════════════════════════════════════════════════

function SecuritySection() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const changePassword = useChangePassword()

  const [oldPwd, setOldPwd] = React.useState('')
  const [newPwd, setNewPwd] = React.useState('')
  const [confirmPwd, setConfirmPwd] = React.useState('')
  const [status, setStatus] = React.useState<{ type: 'success' | 'error'; text: string } | null>(null)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setStatus(null)

    if (!oldPwd.trim() || !newPwd.trim() || !confirmPwd.trim()) {
      setStatus({ type: 'error', text: t('settings.security.required') })
      return
    }
    if (newPwd !== confirmPwd) {
      setStatus({ type: 'error', text: t('settings.security.mismatch') })
      return
    }

    changePassword.mutate(
      { oldPassword: oldPwd, newPassword: newPwd },
      {
        onSuccess: () => {
          setStatus({ type: 'success', text: t('settings.security.success') })
          toast(t('settings.security.success'))
          setOldPwd('')
          setNewPwd('')
          setConfirmPwd('')
        },
        onError: () => {
          setStatus({ type: 'error', text: t('settings.security.wrongOld') })
        },
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.security.title')}</h3>
      <form onSubmit={handleSubmit} style={{ maxWidth: 380, marginTop: 8 }}>
        {/* 当前密码 */}
        <Row label={t('settings.security.oldPwd')}>
          <input
            type="password"
            value={oldPwd}
            onChange={(e) => setOldPwd(e.target.value)}
            autoComplete="current-password"
            className="inline-input"
          />
        </Row>

        {/* 新密码 */}
        <Row label={t('settings.security.newPwd')}>
          <input
            type="password"
            value={newPwd}
            onChange={(e) => setNewPwd(e.target.value)}
            autoComplete="new-password"
            className="inline-input"
          />
        </Row>

        {/* 确认新密码 */}
        <Row label={t('settings.security.confirmPwd')}>
          <input
            type="password"
            value={confirmPwd}
            onChange={(e) => setConfirmPwd(e.target.value)}
            autoComplete="new-password"
            className="inline-input"
          />
        </Row>

        {/* 状态消息 */}
        {status && (
          <div
            style={{
              fontSize: 13,
              padding: '8px 12px',
              borderRadius: 6,
              marginTop: 4,
              background: status.type === 'success' ? 'var(--accent-wash)' : 'oklch(0.577 0.245 27.325 / 0.1)',
              color: status.type === 'success' ? 'var(--accent)' : 'var(--destructive)',
            }}
          >
            {status.text}
          </div>
        )}

        <div style={{ marginTop: 16 }}>
          <button
            type="submit"
            className="pill-btn"
            disabled={changePassword.isPending}
          >
            {t('settings.security.submit')}
          </button>
        </div>
      </form>
    </div>
  )
}

// ════════════════════════════════════════════════════════════
// 主组件：SettingsDialog（覆盖层弹框）
// ════════════════════════════════════════════════════════════

export function SettingsDialog({
  listStyle,
  onChangeListStyle,
  conversationView,
  onChangeConversationView,
  alwaysShowSelect,
  onChangeAlwaysShowSelect,
  layoutMode,
  onChangeLayoutMode,
  onClose,
}: SettingsDialogProps) {
  const { t } = useTranslation()
  const [section, setSection] = React.useState<SettingSection>('appearance')

  // 分区导航配置（对照 MailMaster：外观/通用/账户/邮件/安全/快捷键/关于）
  const sections: { id: SettingSection; labelKey: string; icon: string }[] = [
    { id: 'profile',    labelKey: 'settings.navProfile',          icon: 'user' },
    { id: 'appearance', labelKey: 'settings.page.sectAppearance', icon: 'sun' },
    { id: 'general',    labelKey: 'settings.navGeneral',          icon: 'settings' },
    { id: 'accounts',   labelKey: 'settings.navAccounts',         icon: 'inbox' },
    { id: 'mail',       labelKey: 'settings.navMail',             icon: 'send' },
    { id: 'signature',  labelKey: 'settings.navSignature',        icon: 'draft' },
    { id: 'aliases',    labelKey: 'settings.navAliases',          icon: 'mail' },
    { id: 'rules',      labelKey: 'settings.navRules',            icon: 'filter' },
    { id: 'blocklist',  labelKey: 'settings.navBlocklist',        icon: 'shield' },
    { id: 'privacy',    labelKey: 'settings.navPrivacy',          icon: 'cloud' },
    { id: 'notify',     labelKey: 'settings.navNotify',           icon: 'bell' },
    { id: 'monitoring', labelKey: 'settings.navMonitoring',       icon: 'circle-dot' },
    { id: 'security',   labelKey: 'settings.navSecurity',         icon: 'tag' },
    { id: 'shortcuts',  labelKey: 'settings.navShortcuts',        icon: 'compose' },
    { id: 'about',      labelKey: 'settings.navAbout',            icon: 'more' },
  ]

  // Esc 键关闭弹框
  React.useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const currentLabel = sections.find((s) => s.id === section)?.labelKey ?? 'settings.title'

  return (
    // 遮罩层：点击空白处关闭
    <div
      className="settings-backdrop"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      {/* 弹框本体：阻止冒泡，避免点击内部误关 */}
      <div className="settings-dialog" onMouseDown={(e) => e.stopPropagation()}>
        {/* 关闭按钮 */}
        <button
          type="button"
          className="icon-btn sd-close"
          onClick={onClose}
          title={t('account.cancel')}
          aria-label={t('account.cancel')}
        >
          <Icon name="close" size={14} />
        </button>

        {/* 左侧分栏导航 */}
        <aside className="sd-nav">
          <div className="sd-nav-head">{t('settings.title')}</div>
          {sections.map((s) => (
            <button
              key={s.id}
              type="button"
              className={'sd-nav-item' + (section === s.id ? ' active' : '')}
              onClick={() => setSection(s.id)}
            >
              <Icon name={s.icon as Parameters<typeof Icon>[0]['name']} size={14} />
              <span>{t(s.labelKey)}</span>
            </button>
          ))}
        </aside>

        {/* 右侧内容区 */}
        <div className="sd-body">
          <div className="sd-body-head">
            <div className="sd-body-title">{t(currentLabel)}</div>
          </div>
          <div className="sd-body-scroll">
            {section === 'profile' && <ProfileSection />}
            {section === 'appearance' && (
              <AppearanceSection
                listStyle={listStyle}
                onChangeListStyle={onChangeListStyle}
                alwaysShowSelect={alwaysShowSelect}
                onChangeAlwaysShowSelect={onChangeAlwaysShowSelect}
                layoutMode={layoutMode}
                onChangeLayoutMode={onChangeLayoutMode}
              />
            )}
            {section === 'general' && <GeneralSection />}
            {section === 'accounts' && <AccountsSection />}
            {section === 'mail' && (
              <MailSection
                conversationView={conversationView}
                onChangeConversationView={onChangeConversationView}
              />
            )}
            {section === 'signature' && <SignatureSection />}
            {section === 'aliases' && <AliasesSection />}
            {section === 'rules' && <RulesSection />}
            {section === 'blocklist' && <BlocklistSection />}
            {section === 'privacy' && <PrivacySection />}
            {section === 'notify' && <NotifyChannelsSection />}
            {section === 'monitoring' && <MonitoringSection />}
            {section === 'security' && <SecuritySection />}
            {section === 'shortcuts' && <ShortcutsSection />}
            {section === 'about' && <AboutSection />}
          </div>
        </div>
      </div>
    </div>
  )
}

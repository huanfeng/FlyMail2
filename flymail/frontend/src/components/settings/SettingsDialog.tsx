// 设置弹框组件（modal）
import { useFocusTrap } from '@/hooks/useFocusTrap'
// 参考蓝本：.dev/mailmaster/src_extracted/06_87910dfb.js (SettingsScreen + THEMES_LIST)
// 原版设置是覆盖层弹框（.settings-backdrop > .settings-dialog），左侧分栏导航 + 右侧内容。
// 所有颜色严格使用 CSS 令牌，不写死任何颜色值。复用现有 Section 子组件数据逻辑。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { PortabilityDialog } from '@/components/settings/PortabilityDialog'
import { NotifyChannelsSection } from '@/components/settings/NotifyChannelsSection'
import { BrowserNotifySection } from '@/components/settings/BrowserNotifySection'
import { BaseUrlSection } from '@/components/settings/BaseUrlSection'
import { MonitoringSection } from '@/components/settings/MonitoringSection'
import { RulesSection } from '@/components/settings/RulesSection'
import { BlocklistSection } from '@/components/settings/BlocklistSection'
import { TrustedSendersSection } from '@/components/settings/TrustedSendersSection'
import { SignatureSection } from '@/components/settings/SignatureSection'
import { AliasesSection } from '@/components/settings/AliasesSection'
import { getTheme, applyTheme, TONES } from '@/lib/theme'
import { getShortcutGroups } from '@/lib/shortcuts'
import { setListStyle } from '@/lib/list-prefs'
import { modalLayerOpen } from '@/lib/overlay-layers'
import { useAccountSync } from '@/hooks/useAccountSync'
import {
  getDarkBody,
  getRemoteImageDefault,
  setDarkBody,
  setRemoteImageDefault,
} from '@/lib/privacy-prefs'
import { LAYOUT_LIMITS, loadLayoutWidths, saveLayoutWidths } from '@/lib/layout-prefs'
import type { LayoutWidths } from '@/lib/layout-prefs'
import {
  useAccounts,
  useSettings,
  useUpdateSettings,
  useDeleteAccount,
  useReorderAccounts,
  useSetAccountEnabled,
  useChangePassword,
  useAccountStats,
  useMe,
  useUpdateProfile,
  useReindexSearch,
  useRebuildThreads,
} from '@/lib/queries'
import { moveItem } from '@/lib/reorder'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import type { Account, BodySyncMode } from '@/lib/types'

// ── 常量 ─────────────────────────────────────────────────
const SYNC_DEPTH_MIN = 100
const SYNC_DEPTH_MAX = 5000
const POLL_INTERVAL_MIN = 30
const POLL_INTERVAL_MAX = 3600
// 正文预取的天数窗口上下限（与后端 body_sync_recent_days 校验一致）
const BODY_DAYS_MIN = 1
const BODY_DAYS_MAX = 3650

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
  /** 被预览的色调。用 ToneId 而不是 string：这个值直接写进 data-theme，
      拼错就是 9 张卡全部渲染成当前主题、看起来一模一样，而且不会有任何报错。 */
  id: ToneId
  label: string
  mode: ThemeMode
  active: boolean
  onClick: () => void
}

export function ThemeCard({ id, label, mode, active, onClick }: ThemeCardProps) {
  return (
    <button
      type="button"
      className={'theme-card' + (active ? ' active' : '')}
      onClick={onClick}
      aria-pressed={active}
      aria-label={label}
    >
      {/* 颜色预览区：侧栏色 + 主区底色 + accent 条 + 模拟文本线。
          这里不写任何颜色——把目标主题的 data-theme/data-mode 挂在预览区上，
          让 index.css 的令牌在这个子树内重新定义一遍，预览自己就长成那个主题的样子。
          色板因此只有 index.css 一份权威定义，改主题不必再同步第二处。
          （属性挂在预览区而不是整张卡上：卡片外框与名称要跟随**当前**主题。） */}
      <div className="tc-preview" data-theme={id} data-mode={mode}>
        <div className="tc-side" />
        <div className="tc-main">
          <div className="tc-accent" />
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
  const [darkBody, setDarkBodyState] = React.useState<boolean>(() => getDarkBody())

  function handleRemoteImages(next: boolean) {
    setLoadRemoteImages(next)
    // 写完就结束：开关是可订阅的（见 privacy-prefs），已挂载的 useMessageDetail
    // 会因此重新渲染、把 remote 换进 query key，新 key 自然去取新口径的正文。
    // ⚙ 不能在这里 invalidate ['message']：那一瞬间阅读器还没重渲染，失效的是旧 key，
    // 结果是按旧口径白白多打一趟网络。
    setRemoteImageDefault(next)
  }

  function handleDarkBody(next: boolean) {
    setDarkBodyState(next)
    // 与上面同理：开关可订阅，已挂载的正文组件会重新渲染并重建 iframe 文档。
    setDarkBody(next)
  }

  return (
    <>
      <div className="settings-block">
        <h3>{t('settings.privacy.reading')}</h3>
        <Row label={t('settings.privacy.remoteImages')} help={t('settings.privacy.remoteImagesHint')}>
          <Toggle on={loadRemoteImages} onChange={handleRemoteImages} />
        </Row>
        <Row label={t('settings.privacy.darkBody')} help={t('settings.privacy.darkBodyHint')}>
          <Toggle on={darkBody} onChange={handleDarkBody} />
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
  /** 上移/下移一位；为 null 表示已在首/末位，按钮置灰 */
  onMoveUp: (() => void) | null
  onMoveDown: (() => void) | null
}

function AccountCardRow({ account, onEdit, onDelete, onMoveUp, onMoveDown }: AccountCardRowProps) {
  const { t } = useTranslation()
  const setEnabled = useSetAccountEnabled()
  const statsQuery = useAccountStats(account.id)
  // 与侧栏那个同步按钮共用同一套触发/轮询/收手逻辑，见 useAccountSync 的头注释。
  // 原先这里是自己写的第二份，同样踩了「上一轮遗留的 done 让第二次点击直接空转」
  // 和「触发失败照样开轮询」两个坑。
  const accountSync = useAccountSync()
  const syncing = accountSync.syncing && accountSync.accountId === account.id

  function handleSync() {
    accountSync.start(account.id)
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
        {/* 排序：上移 / 下移。
            首行的上移与末行的下移置灰而不是隐藏——隐藏会让按钮列
            在不同行之间错位，眼睛得重新找一次「下移」在哪。 */}
        <div className="ac-reorder">
          <button
            type="button"
            className="icon-btn"
            onClick={() => onMoveUp?.()}
            disabled={onMoveUp == null}
            title={t('settings.account.moveUp')}
            aria-label={t('settings.account.moveUp')}
          >
            <Icon name="chevron-up" size={13} />
          </button>
          <button
            type="button"
            className="icon-btn"
            onClick={() => onMoveDown?.()}
            disabled={onMoveDown == null}
            title={t('settings.account.moveDown')}
            aria-label={t('settings.account.moveDown')}
          >
            <Icon name="chevron-down" size={13} />
          </button>
        </div>

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
          disabled={syncing || !account.enabled}
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
  const confirm = useConfirm()
  const { toast } = useToast()
  const { data: accounts = [] } = useAccounts()
  const deleteAccount = useDeleteAccount()
  const reorder = useReorderAccounts()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editingAccount, setEditingAccount] = React.useState<Account | null>(null)
  // null = 未打开；'export' / 'import' 决定进哪一半。
  // 两个动作共用一个对话框：它们的账户勾选列表与交互形态一致，
  // 拆成两个组件会把那份列表抄两遍。
  const [portable, setPortable] = React.useState<'export' | 'import' | null>(null)

  function handleAdd() {
    setEditingAccount(null)
    setDialogOpen(true)
  }

  function handleEdit(account: Account) {
    setEditingAccount(account)
    setDialogOpen(true)
  }

  function handleMove(index: number, delta: number) {
    const next = moveItem(accounts, index, delta)
    // moveItem 越界时原样返回入参，此时一个请求都不该发
    if (next === accounts) return
    // 失败时 onSettled 的重取会把列表弹回原位。不说一声的话，
    // 用户看到的就是「点了箭头没反应」，只会以为按钮坏了。
    reorder.mutate(next.map((a) => a.id), {
      onError: () => toast(t('settings.account.reorderFailed')),
    })
  }

  async function handleDelete(account: Account) {
    // 文案与侧栏那处合并到 account.*：两处说的是同一件事，此前是一字不差的两份
    const ok = await confirm({
      title: t('account.deleteConfirm'),
      body: t('account.deleteConfirmBody'),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    deleteAccount.mutate(account.id)
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
        accounts.map((account, i) => (
          <AccountCardRow
            key={account.id}
            account={account}
            onEdit={() => handleEdit(account)}
            onDelete={() => handleDelete(account)}
            onMoveUp={i > 0 ? () => handleMove(i, -1) : null}
            onMoveDown={i < accounts.length - 1 ? () => handleMove(i, 1) : null}
          />
        ))
      )}

      {/* 账户操作：添加 / 导出 / 导入。
          三个按钮同排：它们都是"对账户整体做点什么"，分开放会让用户
          以为导出属于某一个账户。 */}
      <div className="settings-actions">
        <button type="button" className="pill-btn" onClick={handleAdd}>
          <Icon name="plus" size={12} />
          {t('settings.account.add')}
        </button>
        <button
          type="button"
          className="pill-btn"
          onClick={() => setPortable('export')}
          disabled={accounts.length === 0}
        >
          <Icon name="archive" size={12} />
          {t('settings.portable.exportTitle')}
        </button>
        <button type="button" className="pill-btn" onClick={() => setPortable('import')}>
          <Icon name="cloud" size={12} />
          {t('settings.portable.importTitle')}
        </button>
      </div>

      {/* AccountDialog 复用 */}
      <AccountDialog
        open={dialogOpen}
        account={editingAccount}
        onOpenChange={setDialogOpen}
      />

      {/* 打开时才挂载：对话框内部的初始状态因此每次都是干净的，
          不需要一个「open 变真就重置一遍」的 effect（那会在 effect 里
          同步 setState、触发级联渲染）。 */}
      {portable != null && (
        <PortabilityDialog
          open
          mode={portable}
          accounts={accounts}
          onOpenChange={(o) => { if (!o) setPortable(null) }}
        />
      )}
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

      {/* 头像 + 用户名概览。尺寸与形状全部交给 .settings-identity：
          此前尺寸写在内联样式里、形状（圆角/居中/白字）指望 .account-card 下的
          规则，而这里根本不在 .account-card 内——于是只剩一个直角色块。 */}
      <div className="settings-identity">
        <div className="ac-avatar" aria-hidden="true">
          {nameInitials(avatarName)}
        </div>
        <div className="ac-text">
          <div className="ac-name">{me?.username ?? '—'}</div>
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

  // 焦点关在浮层里，关闭后还给打开它的那个按钮
  const trapRef = useFocusTrap<HTMLDivElement>(true)

  // Esc 键关闭弹框
  React.useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      // 面板内弹出的 radix 浮层（账户 / 规则 / 渠道对话框、删除确认框）开着时，
      // 这一下 Esc 是给它的。radix 只调 preventDefault 而不 stopPropagation，
      // 事件照样冒泡到这里——不判一下就会「取消一次删除，整个设置面板跟着关掉」。
      // 两个判据各管一头：defaultPrevented 认「那一层已经消费了这次按键」，
      // modalLayerOpen() 认「那一层还开着」。实测任一个单独都够用，
      // 留着两个是因为它们失效的方式不同（浮层不 preventDefault / 浮层不在 DOM 上留痕）。
      if (e.defaultPrevented || modalLayerOpen()) return
      onClose()
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
      <div
        ref={trapRef}
        className="settings-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={t('settings.title')}
        onMouseDown={(e) => e.stopPropagation()}
      >
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
            {section === 'notify' && (
              <>
                {/* 这台设备上的提醒排在外发渠道之前：多数人要的是「让这个浏览器
                    提醒我」，而不是先去配一个 webhook */}
                <BrowserNotifySection Row={Row} Toggle={Toggle} />
                {/* 对外访问地址紧挨着外发渠道：它只对外发的通知有意义
                    （站内通知点一下就跳，不需要绝对地址） */}
                <BaseUrlSection Row={Row} />
                <NotifyChannelsSection />
              </>
            )}
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

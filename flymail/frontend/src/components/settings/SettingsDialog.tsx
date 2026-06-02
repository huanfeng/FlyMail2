// 设置弹框组件（modal）
// 参考蓝本：.dev/mailmaster/src_extracted/06_87910dfb.js (SettingsScreen + THEMES_LIST)
// 原版设置是覆盖层弹框（.settings-backdrop > .settings-dialog），左侧分栏导航 + 右侧内容。
// 所有颜色严格使用 CSS 令牌，不写死任何颜色值。复用现有 Section 子组件数据逻辑。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { AccountDialog } from '@/components/mail/AccountDialog'
import { getTheme, applyTheme, TONES } from '@/lib/theme'
import { setListStyle } from '@/lib/list-prefs'
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
} from '@/lib/queries'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { Account, SyncPhase } from '@/lib/types'

// ── 常量 ─────────────────────────────────────────────────
const LOAD_REMOTE_IMAGES_KEY = 'flymail_load_remote_images'
const SYNC_DEPTH_MIN = 100
const SYNC_DEPTH_MAX = 5000
const POLL_INTERVAL_MIN = 30
const POLL_INTERVAL_MAX = 3600

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
type SettingSection = 'appearance' | 'accounts' | 'mail' | 'security'

// ── Props ─────────────────────────────────────────────────
interface SettingsDialogProps {
  /** 当前列表样式（Shell 管理），使改动立即对邮件列表生效 */
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
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

function AppearanceSection() {
  const { t, i18n } = useTranslation()
  const initial = getTheme()
  const [currentMode, setCurrentMode] = React.useState<ThemeMode>(initial.mode)
  const [currentTone, setCurrentTone] = React.useState<ToneId>(initial.tone)

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

  /** 切换语言 */
  function handleLang(lng: string) {
    void i18n.changeLanguage(lng)
    localStorage.setItem('flymail_lang', lng)
  }

  const currentLang = i18n.language.startsWith('zh') ? 'zh' : 'en'

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

      {/* 亮/暗模式切换 */}
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
      </div>

      {/* 语言切换 */}
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
// 子组件：邮件分区（同步 + 样式 + 远程图）
// ════════════════════════════════════════════════════════════

interface MailSectionProps {
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
}

function MailSection({ listStyle, onChangeListStyle }: MailSectionProps) {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()

  const [syncDepth, setSyncDepth] = React.useState<number>(settings?.sync_depth ?? 1000)
  const [pollInterval, setPollInterval] = React.useState<number>(settings?.sync_poll_interval ?? 180)
  const [depthError, setDepthError] = React.useState<string | null>(null)
  const [intervalError, setIntervalError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)

  const [loadRemoteImages, setLoadRemoteImages] = React.useState<boolean>(
    () => localStorage.getItem(LOAD_REMOTE_IMAGES_KEY) === 'true',
  )

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

  function handleRemoteImages(next: boolean) {
    setLoadRemoteImages(next)
    localStorage.setItem(LOAD_REMOTE_IMAGES_KEY, String(next))
  }

  function handleListStyle(style: ListStyle) {
    setListStyle(style)
    onChangeListStyle(style)
  }

  function handleSave() {
    setDepthError(null)
    setIntervalError(null)
    setSaved(false)

    if (syncDepth < SYNC_DEPTH_MIN || syncDepth > SYNC_DEPTH_MAX) {
      setDepthError(t('settings.mail.invalidDepth'))
      return
    }
    if (pollInterval < POLL_INTERVAL_MIN || pollInterval > POLL_INTERVAL_MAX) {
      setIntervalError(t('settings.mail.invalidInterval'))
      return
    }

    updateSettings.mutate(
      { sync_depth: String(syncDepth), sync_poll_interval: String(pollInterval) },
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

      {/* 列表样式 */}
      <Row label={t('settings.mail.listStyle')}>
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

      {/* 远程图片开关 */}
      <Row label={t('settings.mail.loadRemoteImages')}>
        <Toggle on={loadRemoteImages} onChange={handleRemoteImages} />
      </Row>

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

export function SettingsDialog({ listStyle, onChangeListStyle, onClose }: SettingsDialogProps) {
  const { t } = useTranslation()
  const [section, setSection] = React.useState<SettingSection>('appearance')

  // 分区导航配置
  const sections: { id: SettingSection; labelKey: string; icon: string }[] = [
    { id: 'appearance', labelKey: 'settings.page.sectAppearance', icon: 'sun' },
    { id: 'accounts',   labelKey: 'settings.navAccounts',         icon: 'inbox' },
    { id: 'mail',       labelKey: 'settings.navMail',             icon: 'send' },
    { id: 'security',   labelKey: 'settings.navSecurity',         icon: 'settings' },
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
            {section === 'appearance' && <AppearanceSection />}
            {section === 'accounts' && <AccountsSection />}
            {section === 'mail' && (
              <MailSection
                listStyle={listStyle}
                onChangeListStyle={onChangeListStyle}
              />
            )}
            {section === 'security' && <SecuritySection />}
          </div>
        </div>
      </div>
    </div>
  )
}

// 设置 → 外观与语言：主题色调、布局、亮暗、栏宽、列表密度，以及界面语言。
//
// 「通用」页此前只有语言一项，并进来：一个只有一行设置的页面，
// 在导航里占的位置比它的内容还多。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { getTheme, applyTheme, TONES } from '@/lib/theme'
import { setListStyle } from '@/lib/list-prefs'
import { LAYOUT_LIMITS, loadLayoutWidths, saveLayoutWidths } from '@/lib/layout-prefs'
import type { LayoutWidths } from '@/lib/layout-prefs'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import { Row, Toggle } from '../controls'

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
// 子组件：外观分区
// ════════════════════════════════════════════════════════════

export interface AppearanceSectionProps {
  listStyle: ListStyle
  onChangeListStyle: (style: ListStyle) => void
  alwaysShowSelect: boolean
  onChangeAlwaysShowSelect: (on: boolean) => void
  layoutMode: LayoutMode
  onChangeLayoutMode: (mode: LayoutMode) => void
}

export function AppearanceSection({
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

      <LanguageBlock />
    </>
  )
}

// ════════════════════════════════════════════════════════════
// 子组件：界面语言
// ════════════════════════════════════════════════════════════

function LanguageBlock() {
  const { t, i18n } = useTranslation()
  const currentLang = i18n.language.startsWith('zh') ? 'zh' : 'en'

  function handleLang(lng: string) {
    void i18n.changeLanguage(lng)
    localStorage.setItem('flymail_lang', lng)
  }

  return (
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
  )
}

// 设置弹框（modal）：左侧分组导航 + 右侧内容。
//
// 页面、分组、图标都来自 registry.tsx；这里只负责外壳——焦点、Esc、导航与窄屏适配。
// 所有颜色严格使用 CSS 令牌，不写死任何颜色值。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useFocusTrap } from '@/hooks/useFocusTrap'
import { Icon } from '@/components/ui/Icon'
import { modalLayerOpen } from '@/lib/overlay-layers'
import { resolveSettingsPage } from '@/lib/settings-nav'
import type { SettingsPageId } from '@/lib/settings-nav'
import { SETTINGS_GROUPS, SETTINGS_PAGES } from './registry'
import type { SettingsShellProps } from './registry'

// ThemeCard 早先定义在这个文件里，保留一个出口免得外部 import 断掉
export { ThemeCard } from './sections/AppearanceSection'

interface SettingsDialogProps extends SettingsShellProps {
  /** 打开时定位到哪一页；旧版页面 ID 也认（见 resolveSettingsPage） */
  initialSection?: SettingsPageId | string
  /** 关闭弹框的回调 */
  onClose: () => void
}

// ════════════════════════════════════════════════════════════
// 主组件：SettingsDialog（覆盖层弹框）
// ════════════════════════════════════════════════════════════

export function SettingsDialog({ initialSection, onClose, ...shell }: SettingsDialogProps) {
  const { t } = useTranslation()
  // 初值只在挂载时求一次：弹框是「打开时才挂载」的（见 Shell），每次打开都从这里开始
  const [section, setSection] = React.useState<SettingsPageId>(() => resolveSettingsPage(initialSection))

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

  const current = SETTINGS_PAGES.find((p) => p.id === section) ?? SETTINGS_PAGES[0]

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
        <nav className="sd-nav" aria-label={t('settings.title')}>
          <div className="sd-nav-head">{t('settings.title')}</div>
          {SETTINGS_GROUPS.map((g) => (
            <div key={g.id} className="sd-nav-group" role="group" aria-labelledby={`sd-group-${g.id}`}>
              <div className="sd-nav-group-label" id={`sd-group-${g.id}`}>{t(g.labelKey)}</div>
              {g.pages.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className={'sd-nav-item' + (section === p.id ? ' active' : '')}
                  aria-current={section === p.id ? 'page' : undefined}
                  onClick={() => setSection(p.id)}
                >
                  <Icon name={p.icon} size={14} />
                  <span>{t(p.labelKey)}</span>
                </button>
              ))}
            </div>
          ))}
        </nav>

        {/* 右侧内容区 */}
        <div className="sd-body">
          <div className="sd-body-head">
            <div className="sd-body-title">{t(current.labelKey)}</div>
          </div>
          {/* key 按页面换：切页时滚动位置与页内临时状态都从头开始 */}
          <div className="sd-body-scroll" key={current.id}>
            {current.render(shell)}
          </div>
        </div>
      </div>
    </div>
  )
}

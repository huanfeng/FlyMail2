import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'

/**
 * 移动端写邮件入口（≤768 才显示，桌面由 CSS 隐藏）。
 *
 * 侧栏那颗写邮件按钮在手机上藏在抽屉里，列表页原来没有任何写信入口。
 * 挂在 .list-stack 内：阅读面板激活时列表栏整体隐藏，它跟着消失，不需要额外状态。
 */
export function ComposeFab({ onCompose }: { onCompose: () => void }) {
  const { t } = useTranslation()
  return (
    <button type="button" className="compose-fab" onClick={onCompose} aria-label={t('sidebar.compose')}>
      <Icon name="compose" size={22} />
    </button>
  )
}

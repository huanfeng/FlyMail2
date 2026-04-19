import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: string
  subtitle?: string
  actions?: ReactNode
  /** 传递给最外层 div 的 className，默认 shrink-0 */
  className?: string
}

/**
 * 标准页面头部
 *
 * 使用方式:
 *
 * // 类型 A: 仅有 icon 操作按钮
 * <PageHeader
 *   title={t('dashboard.title')}
 *   actions={<Button variant="ghost" size="icon" onClick={refetch}><RefreshCw /></Button>}
 * />
 *
 * // 类型 B: 有副标题
 * <PageHeader
 *   title={t('policy.title')}
 *   subtitle={t('policy.subtitle')}
 *   actions={<Button variant="ghost" size="icon">...</Button>}
 * />
 *
 * // 类型 C: 扩展型（刷新 + 主操作按钮）
 * <PageHeader
 *   title={t('menu.accounts')}
 *   subtitle="管理邮箱账户..."
 *   className="px-6 py-4 border-b"
 *   actions={
 *     <>
 *       <Button variant="outline" size="sm">刷新</Button>
 *       <Button size="sm">新建</Button>
 *     </>
 *   }
 * />
 */
export function PageHeader({ title, subtitle, actions, className = '' }: PageHeaderProps) {
  return (
    <div className={`flex items-center justify-between shrink-0 ${className}`}>
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {subtitle && <p className="text-sm text-muted-foreground mt-0.5">{subtitle}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}

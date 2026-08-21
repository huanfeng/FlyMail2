// 右键菜单（radix ContextMenu 的主题化封装）。
// 用法：<CtxMenu trigger={<div>行内容</div>} items={[...]} />
// items 支持普通项、危险项（destructive）、分隔线与子菜单（children）。

import * as React from 'react'
import { ContextMenu } from 'radix-ui'
import { Icon, type IconName } from '@/components/ui/Icon'

export interface CtxMenuItem {
  /** 唯一 key（列表渲染用） */
  key: string
  label?: string
  icon?: IconName
  /** 危险操作（红色显示） */
  destructive?: boolean
  disabled?: boolean
  onSelect?: () => void
  /** 分隔线（忽略其余字段） */
  separator?: boolean
  /** 子菜单项（存在时忽略 onSelect） */
  children?: CtxMenuItem[]
}

export interface CtxMenuProps {
  /** 触发区域（右键该元素弹出菜单）。必须是可接收 ref 的单个元素。 */
  trigger: React.ReactElement
  items: CtxMenuItem[]
}

const contentClass = 'ctx-menu'

function renderItems(items: CtxMenuItem[]) {
  return items.map((it) => {
    if (it.separator) {
      return <ContextMenu.Separator key={it.key} className="ctx-sep" />
    }
    if (it.children && it.children.length > 0) {
      return (
        <ContextMenu.Sub key={it.key}>
          <ContextMenu.SubTrigger className="ctx-item" disabled={it.disabled}>
            {it.icon && <Icon name={it.icon} size={13} />}
            <span style={{ flex: 1 }}>{it.label}</span>
            <Icon name="chevron-right" size={12} />
          </ContextMenu.SubTrigger>
          <ContextMenu.Portal>
            <ContextMenu.SubContent className={contentClass} sideOffset={2} alignOffset={-4}>
              {renderItems(it.children)}
            </ContextMenu.SubContent>
          </ContextMenu.Portal>
        </ContextMenu.Sub>
      )
    }
    return (
      <ContextMenu.Item
        key={it.key}
        className={'ctx-item' + (it.destructive ? ' destructive' : '')}
        disabled={it.disabled}
        onSelect={it.onSelect}
      >
        {it.icon && <Icon name={it.icon} size={13} />}
        <span style={{ flex: 1 }}>{it.label}</span>
      </ContextMenu.Item>
    )
  })
}

export function CtxMenu({ trigger, items }: CtxMenuProps) {
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger asChild>{trigger}</ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className={contentClass}>
          {renderItems(items)}
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  )
}

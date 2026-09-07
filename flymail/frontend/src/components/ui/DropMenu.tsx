// 左键下拉菜单（radix DropdownMenu 的主题化封装）。
//
// 与 ContextMenu.tsx 是一对：菜单项模型（CtxMenuItem）与视觉（.ctx-menu / .ctx-item）
// 完全共用，只有触发方式不同——那边是右键，这里是左键点击。
// 用法：<DropMenu trigger={<button>⋯</button>} items={[...]} />

import * as React from 'react'
import { DropdownMenu } from 'radix-ui'
import { Icon } from '@/components/ui/Icon'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'

export interface DropMenuProps {
  /** 触发元素，必须能接收 ref（radix 用 asChild 挂事件） */
  trigger: React.ReactElement
  items: CtxMenuItem[]
  /** 对齐方式，默认右对齐（工具栏「更多」在右端，左对齐会溢出栏外） */
  align?: 'start' | 'center' | 'end'
}

function renderItems(items: CtxMenuItem[]) {
  return items.map((it) => {
    if (it.separator) {
      return <DropdownMenu.Separator key={it.key} className="ctx-sep" />
    }
    if (it.children && it.children.length > 0) {
      return (
        <DropdownMenu.Sub key={it.key}>
          <DropdownMenu.SubTrigger className="ctx-item" disabled={it.disabled}>
            {it.icon && <Icon name={it.icon} size={13} />}
            <span style={{ flex: 1 }}>{it.label}</span>
            <Icon name="chevron-right" size={12} />
          </DropdownMenu.SubTrigger>
          <DropdownMenu.Portal>
            <DropdownMenu.SubContent className="ctx-menu" sideOffset={2} alignOffset={-4}>
              {renderItems(it.children)}
            </DropdownMenu.SubContent>
          </DropdownMenu.Portal>
        </DropdownMenu.Sub>
      )
    }
    return (
      <DropdownMenu.Item
        key={it.key}
        className={'ctx-item' + (it.destructive ? ' destructive' : '')}
        disabled={it.disabled}
        onSelect={it.onSelect}
      >
        {it.icon && <Icon name={it.icon} size={13} />}
        <span style={{ flex: 1 }}>{it.label}</span>
      </DropdownMenu.Item>
    )
  })
}

export function DropMenu({ trigger, items, align = 'end' }: DropMenuProps) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="ctx-menu" align={align} sideOffset={4}>
          {renderItems(items)}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

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
  /**
   * 受控开合。**省略即非受控**（radix 自己管），绝大多数调用点都该省略。
   *
   * 需要它的只有一种情况：菜单开着时**由程序**让触发元素变得不可用或消失。
   * radix 自己覆盖了点外部 / Esc / 选中项这三条关闭路径，但覆盖不到
   * 「换了数据源」「退出了选择模式」这类外部状态变化——菜单会留在屏幕上，
   * 而它的 trigger 已经 disabled，关闭时焦点回不到 trigger，掉到 <body>。
   */
  open?: boolean
  onOpenChange?: (open: boolean) => void
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

export function DropMenu({ trigger, items, align = 'end', open, onOpenChange }: DropMenuProps) {
  return (
    // open 为 undefined 时 radix 退回非受控，与原行为逐字相同
    <DropdownMenu.Root open={open} onOpenChange={onOpenChange}>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="ctx-menu" align={align} sideOffset={4}>
          {renderItems(items)}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

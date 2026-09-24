// 设置页通用控件：设置行与开关。
//
// 从 SettingsDialog 里拆出来单独成文件：各分区文件都要用，而 BrowserNotifySection
// 这类早先独立出去的分区只能靠 props 把它们传进去——现在直接 import 即可。

import * as React from 'react'

// ════════════════════════════════════════════════════════════
// 子组件：Toggle 开关
// ════════════════════════════════════════════════════════════

interface ToggleProps {
  on: boolean
  onChange: (next: boolean) => void
  ariaLabel?: string
}

export function Toggle({ on, onChange, ariaLabel }: ToggleProps) {
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

export function Row({ label, help, children }: RowProps) {
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

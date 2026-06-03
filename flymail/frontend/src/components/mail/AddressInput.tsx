// 收件人输入框 + 自动补全
// 多地址以逗号/分号分隔；对"当前正在输入的最后一段 token"查询历史联系人并下拉补全。
// 仅替换当前 token，保留前面已输入的地址。

import { useEffect, useRef, useState } from 'react'
import { useContacts } from '@/lib/queries'
import type { Contact } from '@/lib/types'

interface AddressInputProps {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  disabled?: boolean
  autoFocus?: boolean
}

/** 取最后一个分隔符（, 或 ;）的位置；返回 -1 表示没有 */
function lastSepIndex(s: string): number {
  return Math.max(s.lastIndexOf(','), s.lastIndexOf(';'))
}

export function AddressInput({ value, onChange, placeholder, disabled, autoFocus }: AddressInputProps) {
  const [open, setOpen] = useState(false)
  const [highlight, setHighlight] = useState(0)
  // 防抖后的查询 token，避免每个按键都发请求
  const [queryToken, setQueryToken] = useState('')
  const rootRef = useRef<HTMLDivElement>(null)

  // 当前 token（最后一个分隔符之后的片段，去首尾空格）
  const sep = lastSepIndex(value)
  const rawToken = value.slice(sep + 1)
  const token = rawToken.trim()

  // 防抖：token 变化 200ms 后才更新查询词
  useEffect(() => {
    const id = setTimeout(() => setQueryToken(token), 200)
    return () => clearTimeout(id)
  }, [token])

  const { data: contacts = [] } = useContacts(queryToken, open && queryToken.length >= 1)

  // token 变化时重置高亮并（有内容时）展开
  useEffect(() => {
    setHighlight(0)
  }, [queryToken])

  // 点击组件外部关闭下拉
  useEffect(() => {
    function onDocDown(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onDocDown)
    return () => document.removeEventListener('mousedown', onDocDown)
  }, [])

  /** 用选中的联系人替换当前 token，保留前面的地址 */
  function pick(c: Contact) {
    const prefix = sep >= 0 ? value.slice(0, sep + 1) + ' ' : ''
    onChange(prefix + c.email + ', ')
    setOpen(false)
    setQueryToken('')
  }

  const showList = open && contacts.length > 0 && token.length >= 1

  function onKeyDown(e: React.KeyboardEvent) {
    if (!showList) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlight((h) => Math.min(contacts.length - 1, h + 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlight((h) => Math.max(0, h - 1))
    } else if (e.key === 'Enter') {
      // 有高亮候选时用回车补全（阻止表单/换行）
      e.preventDefault()
      const c = contacts[highlight]
      if (c) pick(c)
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <div ref={rootRef} style={{ position: 'relative', flex: 1, minWidth: 0 }}>
      <input
        value={value}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        disabled={disabled}
        autoFocus={autoFocus}
        autoComplete="off"
        style={{ width: '100%' }}
      />
      {showList && (
        <div
          style={{
            position: 'absolute', top: '100%', left: 0, right: 0, marginTop: 4, zIndex: 50,
            background: 'var(--surface)', border: '1px solid var(--rule)', borderRadius: 8,
            boxShadow: '0 8px 24px rgba(0,0,0,0.12)', padding: 4, maxHeight: 240, overflowY: 'auto',
          }}
        >
          {contacts.map((c, i) => (
            <button
              key={c.email}
              type="button"
              // 用 mousedown 避免 input blur 先于 click 触发
              onMouseDown={(e) => { e.preventDefault(); pick(c) }}
              onMouseEnter={() => setHighlight(i)}
              style={{
                display: 'flex', flexDirection: 'column', alignItems: 'flex-start',
                width: '100%', padding: '6px 10px', border: 'none', borderRadius: 6,
                background: i === highlight ? 'var(--bg-alt)' : 'transparent',
                cursor: 'pointer', textAlign: 'left',
              }}
            >
              {c.name && (
                <span style={{ fontSize: 13, color: 'var(--ink)' }}>{c.name}</span>
              )}
              <span style={{ fontSize: 12, color: 'var(--ink-3)', fontFamily: 'var(--font-mono)' }}>
                {c.email}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

import { useEffect } from 'react'
import type { MessageListItem } from '@/lib/types'

// ────────────────────────────────────────────────────────────────────────────
// 自定义事件名：用于跨组件通信（聚焦搜索框）
// ────────────────────────────────────────────────────────────────────────────

/** 快捷键 `/` 触发时广播此事件，MailList 内部监听后聚焦搜索框 */
export const FOCUS_SEARCH_EVENT = 'flymail:focus-search'

// ────────────────────────────────────────────────────────────────────────────
// 类型
// ────────────────────────────────────────────────────────────────────────────

interface KeyboardShortcutsOptions {
  /** 撰写新邮件 */
  onCompose: () => void
  /** 回复当前打开的邮件 */
  onReply: (() => void) | null
  /** 当前邮件列表（用于 j/k 上下导航） */
  messages: MessageListItem[]
  /** 当前选中的邮件 id */
  activeMessageId: number | null
  /** 选中邮件回调（用于 j/k 导航） */
  selectMessage: (id: number) => void
  /** 关闭 Compose 对话框 */
  onCloseCompose: () => void
  /** Compose 是否打开中（打开时屏蔽单键，但 Esc 仍生效） */
  composeOpen: boolean
}

// ────────────────────────────────────────────────────────────────────────────
// 判断焦点是否在输入型元素内（输入框/textarea/富文本编辑器）
// 单键快捷键在此情况下不触发，避免干扰打字
// ────────────────────────────────────────────────────────────────────────────

function isInInputField(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false
  const tag = target.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
  if ((target as HTMLElement).isContentEditable) return true
  return false
}

// ────────────────────────────────────────────────────────────────────────────
// Hook
// ────────────────────────────────────────────────────────────────────────────

/**
 * 绑定全局键盘快捷键（复刻 MailMaster 键位）：
 * - c / n   : 撰写新邮件
 * - /       : 聚焦列表搜索框
 * - r       : 回复当前邮件（仅有选中邮件时生效）
 * - j / k   : 列表下一封 / 上一封
 * - Esc     : 关闭 Compose / 取消选中
 *
 * 注意：输入框/textarea/contenteditable 聚焦时不触发单键。
 */
export function useKeyboardShortcuts({
  onCompose,
  onReply,
  messages,
  activeMessageId,
  selectMessage,
  onCloseCompose,
  composeOpen,
}: KeyboardShortcutsOptions): void {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Esc 优先处理（无论焦点位置）
      if (e.key === 'Escape') {
        if (composeOpen) {
          onCloseCompose()
        }
        return
      }

      // 在输入型元素内或 Compose 打开时，屏蔽单键快捷键
      if (isInInputField(e.target) || composeOpen) return

      // 忽略带修饰键的组合（让浏览器原生快捷键正常工作）
      if (e.metaKey || e.ctrlKey || e.altKey) return

      const key = e.key.toLowerCase()

      switch (key) {
        // c 或 n：撰写新邮件
        case 'c':
        case 'n': {
          e.preventDefault()
          onCompose()
          break
        }

        // /：聚焦列表搜索框（通过自定义事件通知 MailList）
        case '/': {
          e.preventDefault()
          window.dispatchEvent(new CustomEvent(FOCUS_SEARCH_EVENT))
          break
        }

        // r：回复当前邮件
        case 'r': {
          if (onReply != null) {
            e.preventDefault()
            onReply()
          }
          break
        }

        // j：列表下一封
        case 'j': {
          e.preventDefault()
          if (messages.length === 0) break
          const idx = messages.findIndex((m) => m.id === activeMessageId)
          // 未选中时选第一封；已选中则移到下一封（不超出末尾）
          const nextIdx = idx === -1 ? 0 : Math.min(messages.length - 1, idx + 1)
          const next = messages[nextIdx]
          if (next != null) selectMessage(next.id)
          break
        }

        // k：列表上一封
        case 'k': {
          e.preventDefault()
          if (messages.length === 0) break
          const idx = messages.findIndex((m) => m.id === activeMessageId)
          if (idx <= 0) break
          const prev = messages[idx - 1]
          if (prev != null) selectMessage(prev.id)
          break
        }

        default:
          break
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [
    onCompose,
    onReply,
    messages,
    activeMessageId,
    selectMessage,
    onCloseCompose,
    composeOpen,
  ])
}

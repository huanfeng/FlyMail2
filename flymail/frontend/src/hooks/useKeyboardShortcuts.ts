import { useEffect } from 'react'
import type { MessageListItem } from '@/lib/types'
import { KEY } from '@/lib/shortcuts'

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
  /** Esc 且 Compose / 帮助浮层均未打开时调用（清空选中邮件 / 关闭双栏浮动阅读） */
  onEscape?: () => void
  /** `?` 切换快捷键速查浮层 */
  onToggleHelp: () => void
  /** 关闭快捷键速查浮层（Esc 时优先于其它 Esc 行为） */
  onCloseHelp: () => void
  /** 速查浮层是否打开中（打开时屏蔽单键，Esc 优先关闭它） */
  helpOpen: boolean
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
 * - ?       : 切换快捷键速查浮层
 * - Esc     : 关闭速查浮层 / 关闭 Compose / 取消选中
 *
 * 键位目录的单一真相源见 lib/shortcuts.ts（KEY 常量 + getShortcutGroups）。
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
  onEscape,
  onToggleHelp,
  onCloseHelp,
  helpOpen,
}: KeyboardShortcutsOptions): void {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Esc 优先处理（无论焦点位置）：帮助浮层 > Compose > 通用返回
      if (e.key === KEY.escape) {
        if (helpOpen) {
          onCloseHelp()
        } else if (composeOpen) {
          onCloseCompose()
        } else {
          onEscape?.()
        }
        return
      }

      // ⌘K / Ctrl+K：聚焦搜索（组合键，无论焦点位置都生效）
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        window.dispatchEvent(new CustomEvent(FOCUS_SEARCH_EVENT))
        return
      }

      // ? : 切换快捷键速查浮层。在其它单键屏蔽之前处理（即使浮层已开也能再次按 ? 关闭），
      //     但输入框内不触发，避免打字时误开。
      if (e.key === KEY.help) {
        if (isInInputField(e.target)) return
        e.preventDefault()
        onToggleHelp()
        return
      }

      // 在输入型元素内 / Compose 打开 / 帮助浮层打开时，屏蔽其余单键快捷键
      if (isInInputField(e.target) || composeOpen || helpOpen) return

      // 忽略带修饰键的组合（让浏览器原生快捷键正常工作）
      if (e.metaKey || e.ctrlKey || e.altKey) return

      const key = e.key.toLowerCase()

      switch (key) {
        // c 或 n：撰写新邮件
        case KEY.composeC:
        case KEY.composeN: {
          e.preventDefault()
          onCompose()
          break
        }

        // /：聚焦列表搜索框（通过自定义事件通知 MailList）
        case KEY.focusSearch: {
          e.preventDefault()
          window.dispatchEvent(new CustomEvent(FOCUS_SEARCH_EVENT))
          break
        }

        // r：回复当前邮件
        case KEY.reply: {
          if (onReply != null) {
            e.preventDefault()
            onReply()
          }
          break
        }

        // j：列表下一封
        case KEY.next: {
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
        case KEY.prev: {
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
    onEscape,
    onToggleHelp,
    onCloseHelp,
    helpOpen,
  ])
}

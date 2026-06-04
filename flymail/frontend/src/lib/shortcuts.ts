// 键盘快捷键单一真相源
// ─────────────────────────────────────────────────────────────────────────────
// 此模块是全应用快捷键目录的唯一定义处，被三个消费者复用：
//   1. hooks/useKeyboardShortcuts.ts —— 实际按键绑定（用此处的 KEY 常量避免魔法字符串）
//   2. components/mail/ShortcutsCheatsheet.tsx —— `?` 触发的速查浮层
//   3. components/settings/SettingsDialog.tsx —— 设置内的键位表
// 描述文案统一走 i18n `shortcuts.*` 键，避免多处漂移。

import { searchShortcutHint } from '@/lib/platform'

// ── 原始按键常量（供 hook 匹配，避免魔法字符串）────────────────────────────────

/** 单键快捷键的 `event.key`（统一小写比较）。 */
export const KEY = {
  composeC: 'c',
  composeN: 'n',
  reply: 'r',
  focusSearch: '/',
  next: 'j',
  prev: 'k',
  help: '?',
  escape: 'Escape',
} as const

// ── 目录数据模型 ───────────────────────────────────────────────────────────────

/** 单条快捷键定义。`keys` 中每个元素渲染成一个独立 <kbd>。 */
export interface ShortcutItem {
  /** 稳定标识（React key / 测试断言用）。 */
  id: string
  /** 展示按键，每个元素一个 <kbd>；多元素表示"任一可用"。 */
  keys: string[]
  /** 描述文案的 i18n 键。 */
  descKey: string
}

/** 一组同类快捷键。 */
export interface ShortcutGroup {
  id: string
  /** 分组标题的 i18n 键。 */
  titleKey: string
  items: ShortcutItem[]
}

/**
 * 返回按分组组织的快捷键目录。
 *
 * 用函数而非常量：搜索快捷键的展示（⌘K / Ctrl K）依赖运行平台，
 * 需在调用时经 `searchShortcutHint()` 解析。
 */
export function getShortcutGroups(): ShortcutGroup[] {
  return [
    {
      id: 'nav',
      titleKey: 'shortcuts.groupNav',
      items: [
        { id: 'next-prev', keys: ['J', 'K'], descKey: 'shortcuts.nav' },
      ],
    },
    {
      id: 'actions',
      titleKey: 'shortcuts.groupActions',
      items: [
        { id: 'compose', keys: ['C', 'N'], descKey: 'shortcuts.compose' },
        { id: 'reply', keys: ['R'], descKey: 'shortcuts.reply' },
      ],
    },
    {
      id: 'search',
      titleKey: 'shortcuts.groupSearch',
      items: [
        { id: 'search', keys: ['/', searchShortcutHint()], descKey: 'shortcuts.search' },
      ],
    },
    {
      id: 'general',
      titleKey: 'shortcuts.groupGeneral',
      items: [
        { id: 'help', keys: ['?'], descKey: 'shortcuts.help' },
        { id: 'close', keys: ['Esc'], descKey: 'shortcuts.close' },
      ],
    },
  ]
}

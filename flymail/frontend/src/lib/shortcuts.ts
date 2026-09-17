// 键盘快捷键单一真相源
// ─────────────────────────────────────────────────────────────────────────────
// 此模块是全应用快捷键目录的唯一定义处，被三个消费者复用：
//   1. hooks/useKeyboardShortcuts.ts —— 实际按键绑定（用此处的 KEY 常量避免魔法字符串）
//   2. components/mail/ShortcutsCheatsheet.tsx —— `?` 触发的速查浮层
//   3. components/settings/SettingsDialog.tsx —— 设置内的键位表
// 描述文案统一走 i18n `shortcuts.*` 键，避免多处漂移。
//
// 键位取自主流邮件客户端的通用集（Gmail / Outlook 网页版基本一致），
// 使用者的肌肉记忆可以直接迁移过来，不需要重新学一套。

import { comboHint, searchShortcutHint } from '@/lib/platform'

// ── 原始按键常量（供 hook 匹配，避免魔法字符串）────────────────────────────────

/** 单键快捷键的 `event.key`（统一小写比较）。 */
export const KEY = {
  composeC: 'c',
  composeN: 'n',
  reply: 'r',
  replyAll: 'a',
  forward: 'f',
  focusSearch: '/',
  next: 'j',
  prev: 'k',
  archive: 'e',
  /** # 删除（Gmail 键位）；Delete 键同义，见 KEY.delete */
  deleteHash: '#',
  delete: 'Delete',
  star: 's',
  /** u 单独按 = 回到列表；Shift+U = 标为未读。同一个键靠修饰键区分 */
  back: 'u',
  /** 选中/取消选中当前行 */
  select: 'x',
  /** 跳转前缀：g 之后再按 i/s/t/d 落到对应位置 */
  go: 'g',
  help: '?',
  escape: 'Escape',
} as const

/** `g` 之后可接的目标键 → 跳转位置。 */
export const GO_TARGETS = {
  i: 'inbox',
  s: 'starred',
  t: 'sent',
  d: 'drafts',
} as const

export type GoTarget = (typeof GO_TARGETS)[keyof typeof GO_TARGETS]

/**
 * `g` 前缀的等待时长（毫秒）。
 *
 * 超过这个时间没等到第二个键就放弃，否则一个误按的 g 会把之后随便哪次
 * 按 i 都变成跳转。
 */
export const GO_TIMEOUT_MS = 1200

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
        { id: 'back', keys: ['U'], descKey: 'shortcuts.back' },
        { id: 'go-inbox', keys: ['G', 'I'], descKey: 'shortcuts.goInbox' },
        { id: 'go-starred', keys: ['G', 'S'], descKey: 'shortcuts.goStarred' },
        { id: 'go-sent', keys: ['G', 'T'], descKey: 'shortcuts.goSent' },
        { id: 'go-drafts', keys: ['G', 'D'], descKey: 'shortcuts.goDrafts' },
      ],
    },
    {
      id: 'actions',
      titleKey: 'shortcuts.groupActions',
      items: [
        { id: 'compose', keys: ['C', 'N'], descKey: 'shortcuts.compose' },
        { id: 'reply', keys: ['R'], descKey: 'shortcuts.reply' },
        { id: 'reply-all', keys: ['A'], descKey: 'shortcuts.replyAll' },
        { id: 'forward', keys: ['F'], descKey: 'shortcuts.forward' },
        { id: 'send', keys: [comboHint('Enter')], descKey: 'shortcuts.send' },
      ],
    },
    {
      id: 'organize',
      titleKey: 'shortcuts.groupOrganize',
      items: [
        { id: 'archive', keys: ['E'], descKey: 'shortcuts.archive' },
        { id: 'delete', keys: ['#', 'Del'], descKey: 'shortcuts.delete' },
        { id: 'star', keys: ['S'], descKey: 'shortcuts.star' },
        { id: 'unread', keys: ['Shift', 'U'], descKey: 'shortcuts.markUnread' },
        { id: 'undo', keys: [comboHint('Z')], descKey: 'shortcuts.undo' },
      ],
    },
    {
      id: 'select',
      titleKey: 'shortcuts.groupSelect',
      items: [
        { id: 'select-row', keys: ['X'], descKey: 'shortcuts.selectRow' },
        { id: 'extend', keys: ['Shift', 'J/K'], descKey: 'shortcuts.extendSelection' },
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

// ── 按钮上的快捷键提示 ────────────────────────────────────────────────────────

/**
 * 把一个 `event.key` 变成给人看的键名。
 *
 * 单字母统一大写：键盘上印的是大写，而 KEY 里存的是小写（匹配时用小写比较）。
 * 直接把 'k' 展示成小写会让人以为要按 Shift 之外的什么组合。
 */
export function keyLabel(key: string): string {
  switch (key) {
    case 'Delete':
      return 'Del'
    case 'Escape':
      return 'Esc'
    default:
      return key.length === 1 ? key.toUpperCase() : key
  }
}

/**
 * 给按钮的 tooltip 拼上快捷键，形如「下一封 (J)」。
 *
 * ── 为什么要做这件事 ─────────────────────────────────────────────────────────
 *
 * 快捷键目录只出现在 `?` 速查浮层和设置里的键位表——两个地方用户都得先知道
 * 「有快捷键这回事」才会去看。而真正的学习时机是他正在用鼠标点那个按钮的时候：
 * 把键位写在按钮的悬停提示上，用户点几次就自然记住了，不必专门去背一张表。
 *
 * 只加在 title 上、不进 aria-label：读屏用户由 aria-keyshortcuts 得到同样的信息，
 * 把键位念进名字里只会让每次聚焦都多读一串。
 */
export function withShortcut(label: string, key: string): string {
  return `${label} (${keyLabel(key)})`
}

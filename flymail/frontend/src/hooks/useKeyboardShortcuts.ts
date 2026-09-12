import { useEffect, useRef } from 'react'
import { GO_TARGETS, GO_TIMEOUT_MS, KEY, type GoTarget } from '@/lib/shortcuts'
import { modalLayerOpen } from '@/lib/overlay-layers'

// ────────────────────────────────────────────────────────────────────────────
// 自定义事件名：用于跨组件通信（聚焦搜索框）
// ────────────────────────────────────────────────────────────────────────────

/** 快捷键 `/` 触发时广播此事件，MailList 内部监听后聚焦搜索框 */
export const FOCUS_SEARCH_EVENT = 'flymail:focus-search'

/**
 * 请求关闭撰写器。Esc 广播此事件而不是直接关窗：
 * 写了一半的邮件要先问一句「保存草稿 / 丢弃 / 继续写」，
 * 而只有 ComposeDialog 自己知道有没有写过东西。
 */
export const COMPOSE_CLOSE_EVENT = 'flymail:compose-close'

// ────────────────────────────────────────────────────────────────────────────
// 类型
// ────────────────────────────────────────────────────────────────────────────

interface KeyboardShortcutsOptions {
  /** 撰写新邮件 */
  onCompose: () => void
  /** 回复当前打开的邮件 */
  onReply: (() => void) | null
  /** 全部回复 */
  onReplyAll: (() => void) | null
  /** 转发 */
  onForward: (() => void) | null
  /**
   * j/k 导航的条目 id 序列，顺序与列表一致。
   *
   * 用 id 序列而不是邮件数组：会话视图下一「条」是 thread_id 字符串，
   * 单封视图下是数字 id，导航逻辑对两者完全相同，没必要为此写两套。
   */
  navIds: (number | string)[]
  /** 当前选中的条目 id（无选中为 null） */
  activeNavId: number | string | null
  /** 选中条目回调（用于 j/k 导航） */
  onNavigate: (id: number | string) => void
  /** 归档当前条目；null = 当前无可归档目标（没有归档文件夹或已在归档里） */
  onArchive: (() => void) | null
  /** 删除当前条目 */
  onDelete: (() => void) | null
  /** 星标开关 */
  onToggleStar: (() => void) | null
  /** 标为未读 */
  onMarkUnread: (() => void) | null
  /** u：从阅读区回到列表 */
  onBack: () => void
  /** g + i/s/t/d：跳转到收件箱 / 星标 / 已发送 / 草稿 */
  onGo: (target: GoTarget) => void
  /** x：选中/取消选中当前行 */
  onToggleSelectCurrent: () => void
  /** Shift+J / Shift+K：把选择扩展到下一条 / 上一条 */
  onExtendSelection: (dir: 1 | -1) => void
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
  /**
   * 是否有其它浮层遮挡（设置 / 通知 / 账户对话框 / 移动端抽屉）。
   *
   * 为什么必须单列一项：屏蔽判据原本只有「焦点在输入框内」，而浮层里的焦点
   * 通常落在按钮上——不是输入框，于是单键照常穿透到背后的列表。设置面板开着时
   * 按 `#` 会删掉背后那封邮件，撤销条又在用户视线之外的屏幕底部，5 秒后静默提交。
   *
   * Esc 也据此让位：浮层自己负责关自己，这里不能再顺手把当前邮件也关掉。
   */
  overlayOpen: boolean
  /** Ctrl/⌘+Z：撤销刚才的删除/归档/移动。返回 false 表示已无可撤销项。 */
  onUndo: () => boolean
  /** 已无可撤销项时的反馈（撤销窗口已过或已被强制落地） */
  onUndoUnavailable: () => void
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
 * 绑定全局键盘快捷键。键位取自主流邮件客户端的通用集：
 * - c / n     : 撰写新邮件
 * - r / a / f : 回复 / 全部回复 / 转发
 * - e         : 归档          # 或 Del : 删除
 * - s         : 星标          Shift+U  : 标为未读
 * - j / k     : 列表下一条 / 上一条（会话视图下按会话切换）
 * - u         : 回到列表
 * - x         : 选中当前行    Shift+J/K : 扩展选择
 * - g i/s/t/d : 跳收件箱 / 星标 / 已发送 / 草稿
 * - /         : 聚焦搜索      ? : 速查浮层      Esc : 逐层返回
 *
 * 键位目录的单一真相源见 lib/shortcuts.ts（KEY 常量 + getShortcutGroups）。
 * 注意：输入框/textarea/contenteditable 聚焦时不触发单键。
 */
export function useKeyboardShortcuts(opts: KeyboardShortcutsOptions): void {
  // 每次渲染都把最新的回调塞进 ref，事件监听器只注册一次。
  //
  // 这里的选项对象每渲染都是新的（内联箭头函数、flatMap 出来的 navIds），
  // 若直接进依赖数组，等于每次渲染都摘掉再挂上一次全局监听器——按键在这个
  // 空窗里会丢。
  const ref = useRef(opts)
  // 在 effect 里赋值而不是渲染期间直接写：渲染必须是纯的，
  // React 19 的 lint 规则会明确拦下渲染期访问 ref。
  // 无依赖数组 = 每次渲染后都刷新，事件触发时读到的必然是最新那份。
  useEffect(() => {
    ref.current = opts
  })

  // `g` 前缀的等待状态：按下 g 后记一个时间戳，下一个键在窗口内到达才算组合。
  const goAt = useRef(0)

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const o = ref.current

      // Esc 优先处理（无论焦点位置）：帮助浮层 > Compose > 其它浮层 > 通用返回
      //
      // overlayOpen 时直接收手：设置/通知/账户浮层各自监听 Esc 关自己，
      // 这里若继续往下走会把背后正在看的那封邮件也一并关掉——同一次按键被消费两回。
      if (e.key === KEY.escape) {
        goAt.current = 0
        if (o.helpOpen) {
          o.onCloseHelp()
        } else if (o.composeOpen) {
          o.onCloseCompose()
        } else if (!o.overlayOpen && !modalLayerOpen()) {
          // modalLayerOpen()：确认框 / 账户对话框这类 radix 浮层自己会处理这一下 Esc。
          // 不判的话同一次按键被消费两回——取消一次删除，背后正在看的那封邮件跟着关掉。
          o.onEscape?.()
        }
        return
      }

      // ⌘K / Ctrl+K：聚焦搜索（组合键，无论焦点位置都生效）
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        window.dispatchEvent(new CustomEvent(FOCUS_SEARCH_EVENT))
        return
      }

      // ⌘Z / Ctrl+Z：撤销刚才的删除/归档/移动。
      //
      // 撤销条钉在屏幕底部且只存在 5 秒，键盘用户要 Tab 穿过整个虚拟列表才够得到，
      // 实际上等于没有。给它一个键位，撤销才对非鼠标用户真正可用。
      // 输入框内让位给浏览器原生的文本撤销。
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z' && !e.shiftKey) {
        if (isInInputField(e.target) || o.composeOpen) return
        e.preventDefault()
        if (!o.onUndo()) o.onUndoUnavailable()
        return
      }

      // ? : 切换快捷键速查浮层。在其它单键屏蔽之前处理（即使浮层已开也能再次按 ? 关闭），
      //     但输入框内、以及别的浮层开着时不触发。
      if (e.key === KEY.help) {
        if (isInInputField(e.target) || o.overlayOpen) return
        e.preventDefault()
        o.onToggleHelp()
        return
      }

      // 屏蔽其余单键快捷键：输入型元素内 / Compose / 速查浮层 / 任何其它浮层。
      // overlayOpen 漏掉过一次，后果是设置面板开着时按 # 删掉背后的邮件。
      //
      // modalLayerOpen() 补的是那几个布尔量覆盖不到的浮层：删除确认框从阅读区弹出时
      // 三个布尔量全是 false，于是「确认要删掉这一封吗」开着的时候按 # 会删掉**另一封**。
      if (
        isInInputField(e.target) ||
        o.composeOpen ||
        o.helpOpen ||
        o.overlayOpen ||
        modalLayerOpen()
      )
        return

      // 忽略带 Ctrl/Meta/Alt 的组合（让浏览器原生快捷键正常工作）。
      // Shift 不在此列：Shift+U / Shift+J / Shift+K 都是有效键位。
      if (e.metaKey || e.ctrlKey || e.altKey) return

      const key = e.key.toLowerCase()

      // ── g 前缀：g 之后的那个键决定跳去哪儿 ────────────────────────────
      if (goAt.current > 0) {
        const fresh = Date.now() - goAt.current < GO_TIMEOUT_MS
        goAt.current = 0
        const target = (GO_TARGETS as Record<string, GoTarget>)[key]
        if (fresh && target) {
          e.preventDefault()
          o.onGo(target)
          return
        }
        // 过期或不是合法目标键：落下去按普通单键处理
      }
      if (key === KEY.go) {
        e.preventDefault()
        goAt.current = Date.now()
        return
      }

      // ── Shift 组合 ────────────────────────────────────────────────────
      if (e.shiftKey) {
        switch (key) {
          case KEY.back: // Shift+U：标为未读
            if (o.onMarkUnread) {
              e.preventDefault()
              o.onMarkUnread()
            }
            return
          case KEY.next: // Shift+J：向下扩展选择
            e.preventDefault()
            o.onExtendSelection(1)
            return
          case KEY.prev: // Shift+K：向上扩展选择
            e.preventDefault()
            o.onExtendSelection(-1)
            return
          default:
            break
        }
        // 其余带 Shift 的键继续往下走（# 就是 Shift+3 打出来的）
      }

      switch (key) {
        // c 或 n：撰写新邮件
        case KEY.composeC:
        case KEY.composeN: {
          e.preventDefault()
          o.onCompose()
          break
        }

        // /：聚焦列表搜索框（通过自定义事件通知 MailList）
        case KEY.focusSearch: {
          e.preventDefault()
          window.dispatchEvent(new CustomEvent(FOCUS_SEARCH_EVENT))
          break
        }

        // r / a / f：回复 / 全部回复 / 转发
        case KEY.reply: {
          if (o.onReply != null) {
            e.preventDefault()
            o.onReply()
          }
          break
        }
        case KEY.replyAll: {
          if (o.onReplyAll != null) {
            e.preventDefault()
            o.onReplyAll()
          }
          break
        }
        case KEY.forward: {
          if (o.onForward != null) {
            e.preventDefault()
            o.onForward()
          }
          break
        }

        // e：归档
        case KEY.archive: {
          if (o.onArchive != null) {
            e.preventDefault()
            o.onArchive()
          }
          break
        }

        // # 或 Delete：删除（KEY.delete 是 'Delete'，这里比较的是小写化之后的值）
        case KEY.deleteHash:
        case KEY.delete.toLowerCase(): {
          if (o.onDelete != null) {
            e.preventDefault()
            o.onDelete()
          }
          break
        }

        // s：星标开关
        case KEY.star: {
          if (o.onToggleStar != null) {
            e.preventDefault()
            o.onToggleStar()
          }
          break
        }

        // u：回到列表
        case KEY.back: {
          e.preventDefault()
          o.onBack()
          break
        }

        // x：选中/取消选中当前行
        case KEY.select: {
          e.preventDefault()
          o.onToggleSelectCurrent()
          break
        }

        // j：列表下一条
        case KEY.next: {
          e.preventDefault()
          if (o.navIds.length === 0) break
          const idx = o.navIds.indexOf(o.activeNavId as number | string)
          // 未选中时选第一条；已选中则移到下一条（不超出末尾）
          const nextIdx = idx === -1 ? 0 : Math.min(o.navIds.length - 1, idx + 1)
          const next = o.navIds[nextIdx]
          if (next != null) o.onNavigate(next)
          break
        }

        // k：列表上一条
        case KEY.prev: {
          e.preventDefault()
          if (o.navIds.length === 0) break
          const idx = o.navIds.indexOf(o.activeNavId as number | string)
          if (idx <= 0) break
          const prev = o.navIds[idx - 1]
          if (prev != null) o.onNavigate(prev)
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
  }, [])
}

import { useEffect, useRef } from 'react'

/** 能接收键盘焦点的元素选择器（排除 disabled 与显式移出 Tab 序列的） */
const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

/**
 * 把键盘焦点关在一个浮层里。
 *
 * 自绘的 backdrop + div 只是视觉上盖住了背后的界面，对键盘毫无约束：
 * Tab 会径直走到遮罩背后去，用户在一个自己看不见的区域里操作控件，
 * 关闭后焦点落回 body，下一次 Tab 又从整页开头重来。
 * 设置面板有十几个分区上百个控件，没有这层约束等于纯键盘用户进不去也出不来。
 *
 * radix 的 Dialog 自带这套（仓库里 AccountDialog 就是），
 * 但把已有的三个浮层整体迁过去要动到它们的全部布局；
 * 这个 hook 只补焦点这一层，其余不碰。
 *
 * @param active 浮层是否打开中
 * @returns 挂到浮层根元素上的 ref
 */
export function useFocusTrap<T extends HTMLElement>(active: boolean) {
  const ref = useRef<T>(null)

  useEffect(() => {
    if (!active) return
    const root = ref.current
    if (root == null) return

    // 记住是谁打开的，关闭后要把焦点还回去——否则用户「回到」的是页面开头
    const opener = document.activeElement as HTMLElement | null

    function focusables(): HTMLElement[] {
      if (root == null) return []
      return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        // offsetParent 为 null 即不可见（display:none 或祖先隐藏）；
        // position:fixed 的元素 offsetParent 恒为 null，故补一个尺寸判断
        (el) => el.offsetParent !== null || el.getBoundingClientRect().width > 0,
      )
    }

    // 焦点移入浮层。没有可聚焦元素时退而聚焦容器本身，
    // 读屏才会宣布「进入了对话框」而不是继续念背后的列表。
    const first = focusables()[0]
    if (first) first.focus()
    else {
      root.setAttribute('tabindex', '-1')
      root.focus()
    }

    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== 'Tab') return
      const items = focusables()
      if (items.length === 0) {
        e.preventDefault()
        return
      }
      const firstEl = items[0]
      const lastEl = items[items.length - 1]
      const current = document.activeElement

      // 两端环绕。焦点已经在浮层外时（点了背后的元素）一并拉回来。
      if (root != null && !root.contains(current)) {
        e.preventDefault()
        firstEl.focus()
        return
      }
      if (e.shiftKey && current === firstEl) {
        e.preventDefault()
        lastEl.focus()
      } else if (!e.shiftKey && current === lastEl) {
        e.preventDefault()
        firstEl.focus()
      }
    }

    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      // 还焦点。元素可能已经随浮层一起没了，所以要判一下还在不在文档里。
      if (opener && document.contains(opener)) opener.focus()
    }
  }, [active])

  return ref
}

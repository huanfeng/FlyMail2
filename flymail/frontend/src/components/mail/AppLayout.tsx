import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  LAYOUT_EVENT,
  LAYOUT_LIMITS as LIMITS,
  clampWidth as clamp,
  loadLayoutWidths,
  saveLayoutWidths,
  type LayoutWidths as Widths,
} from '@/lib/layout-prefs'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  /**
   * 第三栏内容。邮件视图为 Reader；通知视图（特殊模式）由 Shell 换成
   * 通知屏，左侧 sidebar + list 仍可见。设置为浮层 modal，不经此处。
   */
  reader: ReactNode
}

type PaneKey = 'sidebar' | 'list'

/**
 * 三栏布局外壳，复刻 MailMaster .app/.col/.col-resize 结构。
 * - 宽度通过 CSS 变量 --sidebar-w / --list-w 注入 documentElement，
 *   与 index.css 中 .col.sidebar / .col.list 的 flex-basis 声明联动。
 * - 拖拽：pointer capture，拖拽时给手柄加 .dragging、给 body 加 .is-resizing。
 * - 宽度偏好与设置弹框滑块共用 layout-prefs；设置改动经 LAYOUT_EVENT 即时同步到此。
 */
export function AppLayout({ sidebar, list, reader }: AppLayoutProps) {
  const [w, setW] = useState<Widths>(loadLayoutWidths)
  // 使用 ref 在事件回调里读取最新值（避免闭包陷阱）
  const wRef = useRef(w)
  wRef.current = w

  // 同步 CSS 变量到 :root，驱动 .col.sidebar / .col.list 宽度
  useEffect(() => {
    document.documentElement.style.setProperty('--sidebar-w', `${w.sidebar}px`)
    document.documentElement.style.setProperty('--list-w', `${w.list}px`)
  }, [w])

  // 监听设置弹框滑块的宽度变更，即时同步（拖拽自身写入也会触发，setW 同值为 no-op）
  useEffect(() => {
    function onLayoutChange(e: Event) {
      const detail = (e as CustomEvent<Widths>).detail
      if (detail) setW(detail)
    }
    window.addEventListener(LAYOUT_EVENT, onLayoutChange)
    return () => window.removeEventListener(LAYOUT_EVENT, onLayoutChange)
  }, [])

  // 持久化到 localStorage（并广播，使设置滑块同步）
  function persistWidths(next: Widths) {
    saveLayoutWidths(next)
  }

  // 构造单个 col-resize 手柄的 pointer 事件处理器
  function makeResizeHandlers(key: PaneKey) {
    return {
      onPointerDown(e: React.PointerEvent<HTMLDivElement>) {
        e.preventDefault()
        const el = e.currentTarget
        el.setPointerCapture(e.pointerId)
        el.classList.add('dragging')
        document.body.classList.add('is-resizing')

        let lastX = e.clientX

        function onMove(ev: PointerEvent) {
          const dx = ev.clientX - lastX
          lastX = ev.clientX
          setW((prev) => {
            const next = clamp(prev[key] + dx, LIMITS[key].min, LIMITS[key].max)
            return { ...prev, [key]: next }
          })
        }

        function onUp(ev: PointerEvent) {
          el.releasePointerCapture(ev.pointerId)
          el.classList.remove('dragging')
          document.body.classList.remove('is-resizing')
          el.removeEventListener('pointermove', onMove)
          el.removeEventListener('pointerup', onUp)
          el.removeEventListener('pointercancel', onUp)
          // 拖拽结束后持久化
          persistWidths(wRef.current)
        }

        el.addEventListener('pointermove', onMove)
        el.addEventListener('pointerup', onUp)
        el.addEventListener('pointercancel', onUp)
      },
    }
  }

  return (
    // .app：全屏 flex 容器，class 与 index.css 中的 .app 对应
    <div className="app">
      {/* 侧栏：.col.sidebar，宽度由 CSS 变量 --sidebar-w 控制，常驻所有视图 */}
      <div className="col sidebar">
        {sidebar}
      </div>

      {/* 侧栏与列表之间的拖拽手柄 */}
      <div
        className="col-resize"
        role="separator"
        aria-orientation="vertical"
        aria-label="调整侧栏宽度"
        {...makeResizeHandlers('sidebar')}
      />

      {/* 列表栏：.col.list，宽度由 CSS 变量 --list-w 控制 */}
      <div className="col list">
        {list}
      </div>

      {/* 列表与阅读区之间的拖拽手柄 */}
      <div
        className="col-resize"
        role="separator"
        aria-orientation="vertical"
        aria-label="调整列表宽度"
        {...makeResizeHandlers('list')}
      />

      {/* 第三栏：.col.reader，占剩余空间（邮件视图=Reader，通知视图=通知屏）*/}
      <div className="col reader">
        {reader}
      </div>
    </div>
  )
}

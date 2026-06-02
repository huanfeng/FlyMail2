import { useEffect, useRef, useState, type ReactNode } from 'react'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
  /**
   * 可选：整页内容。传入时渲染 sidebar + fullpage 两栏布局，
   * 忽略 list / reader，用于设置页和通知页。
   */
  fullpage?: ReactNode
}

// localStorage 持久化键
const LS_KEY = 'flymail-layout-v1'

// 默认宽度（px），参考 MailMaster 比例
const DEFAULTS = { sidebar: 248, list: 380 }

// 各栏宽度约束
const LIMITS = {
  sidebar: { min: 180, max: 420 },
  list: { min: 300, max: 680 },
}

type PaneKey = 'sidebar' | 'list'
type Widths = { sidebar: number; list: number }

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v))
}

/** 从 localStorage 加载宽度，失败时返回默认值 */
function loadWidths(): Widths {
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (raw) {
      const p = JSON.parse(raw) as Partial<Widths>
      return {
        sidebar: clamp(p.sidebar ?? DEFAULTS.sidebar, LIMITS.sidebar.min, LIMITS.sidebar.max),
        list: clamp(p.list ?? DEFAULTS.list, LIMITS.list.min, LIMITS.list.max),
      }
    }
  } catch {
    /* ignore */
  }
  return { ...DEFAULTS }
}

/**
 * 三栏布局外壳，复刻 MailMaster .app/.col/.col-resize 结构。
 * - 宽度通过 CSS 变量 --sidebar-w / --list-w 注入 documentElement，
 *   与 index.css 中 .col.sidebar / .col.list 的 flex-basis 声明联动。
 * - 拖拽：pointer capture，拖拽时给手柄加 .dragging、给 body 加 .is-resizing。
 * - 刷新后宽度从 localStorage 恢复。
 */
export function AppLayout({ sidebar, list, reader, fullpage }: AppLayoutProps) {
  const [w, setW] = useState<Widths>(loadWidths)
  // 使用 ref 在事件回调里读取最新值（避免闭包陷阱）
  const wRef = useRef(w)
  wRef.current = w

  // 同步 CSS 变量到 :root，驱动 .col.sidebar / .col.list 宽度
  useEffect(() => {
    document.documentElement.style.setProperty('--sidebar-w', `${w.sidebar}px`)
    document.documentElement.style.setProperty('--list-w', `${w.list}px`)
  }, [w])

  // 持久化到 localStorage
  function persistWidths(next: Widths) {
    try {
      localStorage.setItem(LS_KEY, JSON.stringify(next))
    } catch {
      /* ignore */
    }
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

      {fullpage ? (
        // 整页分支（设置/通知视图）：侧栏 + 整页，无拖拽手柄、无 list/reader
        <div className="col reader" style={{ flex: 1, minWidth: 0 }}>
          {fullpage}
        </div>
      ) : (
        // 三栏分支（邮件视图）：侧栏调宽手柄 + list + list/reader 调宽手柄 + reader
        <>
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

          {/* 阅读区：.col.reader，占剩余空间 */}
          <div className="col reader">
            {reader}
          </div>
        </>
      )}
    </div>
  )
}

import { useCallback, useRef, useState, type ReactNode, type PointerEvent } from 'react'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
}

// 三栏宽度（px）持久化到 localStorage；阅读区占剩余空间。
const LS_KEY = 'flymail-layout-v1'
const DEFAULTS = { sidebar: 248, list: 380 } // 参考 MailMaster 比例
const LIMITS = {
  sidebar: { min: 180, max: 420 },
  list: { min: 300, max: 680 },
}

type PaneKey = 'sidebar' | 'list'
type Widths = { sidebar: number; list: number }

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v))
}

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

// 可拖拽的分隔条：1px 细线 + 更宽透明命中区，悬停/拖拽高亮强调色。
function Divider({ onResize, onCommit }: { onResize: (dx: number) => void; onCommit: () => void }) {
  const startX = useRef(0)
  const dragging = useRef(false)

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    dragging.current = true
    startX.current = e.clientX
    e.currentTarget.setPointerCapture(e.pointerId)
    document.body.style.cursor = 'col-resize'
  }
  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    if (!dragging.current) return
    onResize(e.clientX - startX.current)
    startX.current = e.clientX
  }
  const endDrag = (e: PointerEvent<HTMLDivElement>) => {
    if (!dragging.current) return
    dragging.current = false
    try {
      e.currentTarget.releasePointerCapture(e.pointerId)
    } catch {
      /* ignore */
    }
    document.body.style.cursor = ''
    onCommit()
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      className="group relative z-10 w-px shrink-0 cursor-col-resize self-stretch"
      style={{ background: 'var(--rule)' }}
    >
      {/* 加宽命中区域 */}
      <div className="absolute inset-y-0 -left-1 -right-1" />
      {/* 悬停/拖拽高亮 */}
      <div className="absolute inset-y-0 left-0 w-px opacity-0 transition-opacity group-hover:opacity-100"
        style={{ background: 'var(--accent-color)' }}
      />
    </div>
  )
}

// 三栏布局：侧栏 / 列表 可拖拽调宽并记忆，阅读区占剩余空间。
export function AppLayout({ sidebar, list, reader }: AppLayoutProps) {
  const [w, setW] = useState<Widths>(loadWidths)
  const wRef = useRef(w)
  wRef.current = w

  const resize = useCallback((key: PaneKey, dx: number) => {
    setW((prev) => {
      const next = clamp(prev[key] + dx, LIMITS[key].min, LIMITS[key].max)
      return { ...prev, [key]: next }
    })
  }, [])

  const commit = useCallback(() => {
    try {
      localStorage.setItem(LS_KEY, JSON.stringify(wRef.current))
    } catch {
      /* ignore */
    }
  }, [])

  return (
    <div className="flex h-screen w-screen overflow-hidden">
      <aside
        className="flex-shrink-0 overflow-y-auto"
        style={{ width: w.sidebar, background: 'var(--bg-alt)' }}
      >
        {sidebar}
      </aside>
      <Divider onResize={(dx) => resize('sidebar', dx)} onCommit={commit} />
      <section
        className="flex flex-shrink-0 flex-col overflow-hidden"
        style={{ width: w.list }}
      >
        {list}
      </section>
      <Divider onResize={(dx) => resize('list', dx)} onCommit={commit} />
      <main className="min-w-0 flex-1 overflow-y-auto">{reader}</main>
    </div>
  )
}

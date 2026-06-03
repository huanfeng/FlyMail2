import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LAYOUT_EVENT,
  LAYOUT_LIMITS as LIMITS,
  clampWidth as clamp,
  loadLayoutWidths,
  saveLayoutWidths,
  type LayoutWidths as Widths,
} from '@/lib/layout-prefs'
import { Icon } from '@/components/ui/Icon'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  /**
   * 第三栏内容。邮件视图为 Reader；通知视图（特殊模式）由 Shell 换成
   * 通知屏，左侧 sidebar + list 仍可见。设置为浮层 modal，不经此处。
   */
  reader: ReactNode
  /**
   * 移动端当前显示的主面板：'list'（列表）或 'reader'（阅读/通知）。
   * 桌面端忽略（三栏并排），仅窄屏单栏时据此切换。
   */
  mobilePane: 'list' | 'reader'
  /** 移动端侧栏抽屉是否打开（桌面端忽略，侧栏常驻）。 */
  drawerOpen: boolean
  onDrawerOpenChange: (open: boolean) => void
  /** 移动端从阅读面板返回列表（清空当前邮件/退出通知）。 */
  onMobileBack: () => void
}

type PaneKey = 'sidebar' | 'list'

/**
 * 三栏布局外壳，复刻 MailMaster .app/.col/.col-resize 结构。
 * - 宽度通过 CSS 变量 --sidebar-w / --list-w 注入 documentElement，
 *   与 index.css 中 .col.sidebar / .col.list 的 flex-basis 声明联动。
 * - 拖拽：pointer capture，拖拽时给手柄加 .dragging、给 body 加 .is-resizing。
 * - 宽度偏好与设置弹框滑块共用 layout-prefs；设置改动经 LAYOUT_EVENT 即时同步到此。
 */
export function AppLayout({
  sidebar,
  list,
  reader,
  mobilePane,
  drawerOpen,
  onDrawerOpenChange,
  onMobileBack,
}: AppLayoutProps) {
  const { t } = useTranslation()
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
    // .app：桌面三栏 flex；窄屏据 data-mobile-pane 单栏切换，drawer-open 控制侧栏抽屉
    <div className={'app' + (drawerOpen ? ' drawer-open' : '')} data-mobile-pane={mobilePane}>
      {/* 移动端顶栏：列表面板显示汉堡(开抽屉)，阅读面板显示返回。桌面端 CSS 隐藏。*/}
      <div className="mobile-bar">
        {mobilePane === 'reader' ? (
          <button
            type="button"
            className="icon-btn"
            onClick={onMobileBack}
            aria-label={t('notif.backToInbox')}
          >
            <span style={{ transform: 'scaleX(-1)', display: 'inline-flex' }}>
              <Icon name="chevron-right" size={18} />
            </span>
          </button>
        ) : (
          <button
            type="button"
            className="icon-btn"
            onClick={() => onDrawerOpenChange(true)}
            aria-label={t('app.name')}
          >
            <Icon name="more" size={18} />
          </button>
        )}
        <div className="brand-name" style={{ fontSize: 15 }}>{t('app.name')}</div>
      </div>

      {/* 侧栏：.col.sidebar，桌面常驻；窄屏为左侧抽屉 */}
      <div className="col sidebar">
        {sidebar}
      </div>

      {/* 抽屉遮罩（仅窄屏 + 抽屉打开时可见，点击关闭）*/}
      <div className="drawer-backdrop" onClick={() => onDrawerOpenChange(false)} />

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

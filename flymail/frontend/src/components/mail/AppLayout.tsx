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
  /** 移动端从阅读面板返回列表（清空当前邮件/退出通知）。也用于双栏模式关闭浮动阅读。 */
  onMobileBack: () => void
  /** 布局模式：three（三栏并排）/ two-slide（双栏 + 右侧浮动阅读）。移动端一律按三栏单栏处理。 */
  layoutMode: 'three' | 'two-slide'
}

type PaneKey = 'sidebar' | 'list'

/** 监听媒体查询是否匹配（用于桌面/移动布局切换）。 */
function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(
    () => typeof window !== 'undefined' && window.matchMedia(query).matches,
  )
  useEffect(() => {
    const m = window.matchMedia(query)
    const handler = () => setMatches(m.matches)
    handler()
    m.addEventListener('change', handler)
    return () => m.removeEventListener('change', handler)
  }, [query])
  return matches
}

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
  layoutMode,
}: AppLayoutProps) {
  const { t } = useTranslation()
  // 移动端(≤768)一律按三栏单栏处理，双栏浮动仅桌面生效
  const isMobile = useMediaQuery('(max-width: 768px)')
  const effLayout = isMobile ? 'three' : layoutMode
  const readerOpen = mobilePane === 'reader'
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
    <div
      className={'app' + (drawerOpen ? ' drawer-open' : '')}
      data-mobile-pane={mobilePane}
      data-layout={effLayout === 'two-slide' ? 'two-slide' : undefined}
    >
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

      {/* 列表栏：.col.list；双栏模式加 .list-wide 占满剩余宽度 */}
      <div className={'col list' + (effLayout === 'two-slide' ? ' list-wide' : '')}>
        {list}
      </div>

      {effLayout === 'three' ? (
        <>
          {/* 列表与阅读区之间的拖拽手柄 */}
          <div
            className="col-resize"
            role="separator"
            aria-orientation="vertical"
            aria-label="调整列表宽度"
            {...makeResizeHandlers('list')}
          />

          {/* 第三栏：.col.reader（邮件视图=Reader，通知视图=通知屏）*/}
          <div className="col reader">
            {reader}
          </div>
        </>
      ) : (
        // 双栏模式：阅读面板从右侧滑入浮层，不占列表空间，可关闭
        <div className={'reader-slide-wrap' + (readerOpen ? ' open' : '')}>
          <div className="reader-slide">
            {readerOpen && (
              <>
                {/* 关闭按钮（浮于阅读面板右上角）*/}
                <button
                  type="button"
                  className="icon-btn"
                  onClick={onMobileBack}
                  aria-label={t('reader.close')}
                  title={t('reader.close')}
                  style={{ position: 'absolute', top: 10, right: 12, zIndex: 5 }}
                >
                  <Icon name="close" size={16} />
                </button>
                {reader}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

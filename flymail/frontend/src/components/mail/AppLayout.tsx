import { useEffect, useState, type ReactNode } from 'react'
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
import { ResizeHandle } from '@/components/ui/ResizeHandle'

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

/** 阅读区（第三栏）拖拽保底宽度，与 index.css 中 .col.reader 的 min-width 保持一致。 */
const READER_MIN = 300

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

  // 同步 CSS 变量到 :root，驱动 .col.sidebar / .col.list / .reader-slide 宽度
  useEffect(() => {
    document.documentElement.style.setProperty('--sidebar-w', `${w.sidebar}px`)
    document.documentElement.style.setProperty('--list-w', `${w.list}px`)
    document.documentElement.style.setProperty('--slide-w', `${w.slide}px`)
    // ⚠ 这里**不写** --sender-col-w：那一项归 MailList 管（见它的 senderCol effect）。
    // 两边都写就是与 localStorage 那条同形的竞态——拖发件人列时 MailList 立刻把变量
    // 写成新值而它的防抖还挂着，这时去拖侧栏，本 effect 每帧触发、把变量写回手上那份
    // 旧的 w.senderCol，列宽在侧栏拖拽过程中一直弹回旧值，直到防抖广播出来才跳回去。
  }, [w])

  // 挂载时按存盘值写一次 --sender-col-w 兜底。
  // 不这么做的话，MailList 挂载之前 CSS 走 var(--sender-col-w, 150px) 的默认值，
  // 存了别的宽度的用户会看到列宽先窄后宽闪一下。只写这一次，之后交给 MailList。
  useEffect(() => {
    document.documentElement.style.setProperty(
      '--sender-col-w',
      `${loadLayoutWidths().senderCol}px`,
    )
  }, [])

  // 监听设置弹框滑块的宽度变更，即时同步（拖拽自身写入也会触发，setW 同值为 no-op）
  useEffect(() => {
    function onLayoutChange(e: Event) {
      const detail = (e as CustomEvent<Widths>).detail
      if (!detail) return
      // 必须按值比较后再决定要不要更新：落盘 effect 依赖 [w]，而 saveLayoutWidths
      // 会广播回这里，detail 每次都是新对象——直接 setW 就是
      // 「落盘 → 广播 → 新引用 → 再落盘」的自激循环。返回 prev 时 React 不重渲染。
      setW((prev) =>
        prev.sidebar === detail.sidebar &&
        prev.list === detail.list &&
        prev.slide === detail.slide &&
        prev.senderCol === detail.senderCol
          ? prev
          : detail,
      )
    }
    window.addEventListener(LAYOUT_EVENT, onLayoutChange)
    return () => window.removeEventListener(LAYOUT_EVENT, onLayoutChange)
  }, [])

  // 持久化到 localStorage（并广播，使设置滑块同步）。
  // 防抖挂在宽度上而不是「松手」那一刻：拖拽每帧写盘是浪费，键盘每按一次也一样，
  // 而挂在状态上让两条路共用同一个时机——不必为了「结束时读到最新值」
  // 维护一个在 render 期赋值的 ref（那正是 react-hooks/refs 拦的东西）。
  useEffect(() => {
    // 只写自己管的三项：senderCol 归 MailList，这里手上的那份可能比它的新改动旧
    const mine = { sidebar: w.sidebar, list: w.list, slide: w.slide }
    // pending 让 flush 名副其实：它的本意是「把**还没到期的**改动补上」。
    // 没有它就是「无条件再写一次」——而 visibilitychange 每次切标签页都触发
    //（哪怕一个字节都没改），加上 pagehide，一次页面隐藏会写 4 次盘、广播 4 次，
    // 把 LAYOUT_EVENT 变成「切标签页也会响」的事件。今天只有值比较的监听器在听，
    // 无害；哪天有谁订阅它做实事就会收到一堆莫名其妙的唤醒。
    let pending = true
    const id = setTimeout(() => {
      pending = false
      saveLayoutWidths(mine)
    }, 200)
    // 卸载/关窗时把还没到期的改动补上。防抖的 cleanup 只能 clearTimeout
    //（它每次变更都跑，在里面直接写就等于没有防抖），所以另挂一条 flush——
    // 桌面端关窗没有第二次机会，而「拖完就关」正是常见的收尾动作。
    //
    // 两个事件都挂：按 Page Lifecycle 的模型，页面被丢弃/终止前唯一可以指望的是
    // visibilitychange → hidden，pagehide 与 beforeunload 在多种终止路径上都可能不触发
    // ——而那恰恰包括 WebView2 关窗这种最需要它的场景。多写一次 localStorage 无害，
    // 漏写一次就是用户刚调的宽度白调了。
    const flush = () => {
      if (!pending) return
      pending = false
      saveLayoutWidths(mine)
    }
    const onHide = () => {
      if (document.visibilityState === 'hidden') flush()
    }
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', onHide)
    return () => {
      clearTimeout(id)
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', onHide)
    }
  }, [w])

  /**
   * 三栏形态下侧栏/列表的动态上限：不允许把阅读区挤到 READER_MIN 以下
   * （双栏模式第三栏是浮层，不受列宽挤压，无需此约束）。
   */
  function maxOf(key: PaneKey, prev: Widths): number {
    if (effLayout !== 'three') return LIMITS[key].max
    const other = key === 'list' ? prev.sidebar : prev.list
    return Math.min(LIMITS[key].max, window.innerWidth - other - READER_MIN)
  }

  /** 水平位移 → 侧栏/列表宽度。两者都左锚定，向右拖即变宽。 */
  function resizePane(key: PaneKey, dx: number) {
    setW((prev) => ({ ...prev, [key]: clamp(prev[key] + dx, LIMITS[key].min, maxOf(key, prev)) }))
  }

  function jumpPane(key: PaneKey, to: 'min' | 'max') {
    setW((prev) => ({ ...prev, [key]: to === 'min' ? LIMITS[key].min : maxOf(key, prev) }))
  }

  /** 浮动阅读面板右锚定：向左拖才是变宽，符号与另外两个相反。 */
  function resizeSlide(dx: number) {
    setW((prev) => ({ ...prev, slide: clamp(prev.slide - dx, LIMITS.slide.min, LIMITS.slide.max) }))
  }

  function jumpSlide(to: 'min' | 'max') {
    setW((prev) => ({ ...prev, slide: to === 'min' ? LIMITS.slide.min : LIMITS.slide.max }))
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
      <ResizeHandle
        className="col-resize"
        label={t('layout.resizeSidebar')}
        value={w.sidebar}
        min={LIMITS.sidebar.min}
        // 用动态上限而不是 LIMITS.max：三栏形态下真正的上限由窗口宽度收紧，
        // 而 Home/End 走的就是 maxOf——两处不一致的话，读屏被告知的最大值永远到不了
        max={maxOf('sidebar', w)}
        onDelta={(dx) => resizePane('sidebar', dx)}
        onJump={(to) => jumpPane('sidebar', to)}
      />

      {/* 列表栏：.col.list；双栏模式加 .list-wide 占满剩余宽度 */}
      <div className={'col list' + (effLayout === 'two-slide' ? ' list-wide' : '')}>
        {list}
      </div>

      {effLayout === 'three' ? (
        <>
          {/* 列表与阅读区之间的拖拽手柄 */}
          <ResizeHandle
            className="col-resize"
            label={t('layout.resizeList')}
            value={w.list}
            min={LIMITS.list.min}
            max={maxOf('list', w)}
            onDelta={(dx) => resizePane('list', dx)}
            onJump={(to) => jumpPane('list', to)}
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
                {/* 左缘拖拽手柄：调整浮动面板宽度。
                    这一个尤其需要键盘可用——另外三处在设置里还有滑块替代，
                    浮动面板的宽度只有这条路。 */}
                <ResizeHandle
                  className="slide-resize"
                  label={t('layout.resizeSlide')}
                  value={w.slide}
                  min={LIMITS.slide.min}
                  max={LIMITS.slide.max}
                  onDelta={resizeSlide}
                  onJump={jumpSlide}
                />
                {/* 关闭按钮（浮于阅读面板左上角，返回方向指向左侧列表，符合操作逻辑）*/}
                <button
                  type="button"
                  className="icon-btn reader-slide-close"
                  onClick={onMobileBack}
                  aria-label={t('reader.close')}
                  title={t('reader.close')}
                  style={{ position: 'absolute', top: 12, left: 12, zIndex: 5 }}
                >
                  <span style={{ transform: 'scaleX(-1)', display: 'inline-flex' }}>
                    <Icon name="chevron-right" size={18} />
                  </span>
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

import { useId } from 'react'
import { useTranslation } from 'react-i18next'

/**
 * 可拖拽、**也可用键盘操作**的分隔手柄。
 *
 * 在此之前，四个手柄（侧栏/列表/浮动面板/发件人列）都是 `role="separator"`
 * 加一个 `onPointerDown`：向读屏宣告了"这里有个可调节的分隔符"，却不可聚焦、
 * 没有 `aria-valuenow`、不接受方向键——用户被告知能调，实际调不动，
 * 比不加这个角色更糟。双栏模式下浮动面板的宽度更是只有拖拽一条路
 * （侧栏/列表/发件人列在设置里还有滑块替代）。
 *
 * 键盘操作遵循 ARIA 的 window splitter 惯例：方向键按步长调整，
 * Shift 加速，Home / End 跳到区间端点。
 *
 * 值的语义留给调用方：组件只报告**水平位移增量**，由调用方决定它如何映射到宽度
 * ——浮动面板右锚定，向左拖才是变宽，符号与另外三个相反。这样调用方各自的
 * 夹紧规则（比如三栏形态下按窗口宽度动态收紧上限）不必挤进这里。
 *
 * 落盘也不在这里：调用方对着宽度状态挂一个防抖 effect 即可，
 * 拖拽与键盘两条路自然共用同一个时机，也免得为了「松手时读到最新值」
 * 去维护一个在 render 期赋值的 ref。
 */

/** 方向键步长（px）。与网格对齐，按住 Shift 走 4 倍。 */
const STEP = 16
const STEP_FAST = 64

interface ResizeHandleProps {
  className: string
  /** 无障碍名称，必须是已翻译好的文本 */
  label: string
  /** 当前宽度，用于 aria-valuenow */
  value: number
  min: number
  max: number
  /** 水平位移增量：拖拽的 dx、方向键的 ±step。调用方自己夹紧并写入 */
  onDelta: (dx: number) => void
  /** Home / End：直接跳到区间端点 */
  onJump: (to: 'min' | 'max') => void
  /** 绝对定位等由调用方决定 */
  style?: React.CSSProperties
}

export function ResizeHandle({
  className,
  label,
  value,
  min,
  max,
  onDelta,
  onJump,
  style,
}: ResizeHandleProps) {
  const { t } = useTranslation()
  const hintId = useId()

  function onPointerDown(e: React.PointerEvent<HTMLDivElement>) {
    e.preventDefault()
    const el = e.currentTarget
    el.setPointerCapture(e.pointerId)
    el.classList.add('dragging')
    document.body.classList.add('is-resizing')

    let lastX = e.clientX

    function onMove(ev: PointerEvent) {
      const dx = ev.clientX - lastX
      lastX = ev.clientX
      onDelta(dx)
    }
    function onUp(ev: PointerEvent) {
      el.releasePointerCapture(ev.pointerId)
      el.classList.remove('dragging')
      document.body.classList.remove('is-resizing')
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointercancel', onUp)
    }
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointercancel', onUp)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    const step = e.shiftKey ? STEP_FAST : STEP
    switch (e.key) {
      case 'ArrowLeft':
        e.preventDefault()
        onDelta(-step)
        break
      case 'ArrowRight':
        e.preventDefault()
        onDelta(step)
        break
      case 'Home':
        e.preventDefault()
        onJump('min')
        break
      case 'End':
        e.preventDefault()
        onJump('max')
        break
      default:
        break
    }
  }

  return (
    <>
      <div
        className={className}
        role="separator"
        aria-orientation="vertical"
        aria-label={label}
        aria-valuenow={Math.round(value)}
        aria-valuemin={min}
        aria-valuemax={max}
        // 读屏默认把 valuenow 念成百分比，对宽度没有意义
        aria-valuetext={t('layout.widthPx', { px: Math.round(value) })}
        // 可聚焦不等于用户知道能按什么。sr-only 的说明不占布局（绝对定位）
        aria-describedby={hintId}
        tabIndex={0}
        style={style}
        onPointerDown={onPointerDown}
        onKeyDown={onKeyDown}
      />
      <span id={hintId} className="sr-only">
        {t('layout.resizeHint')}
      </span>
    </>
  )
}

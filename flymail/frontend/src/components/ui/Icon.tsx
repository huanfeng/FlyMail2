// 图标集 — stroke-based，16px viewBox，跟随父级 currentColor
// 移植自 .dev/mailmaster/src_extracted/04_52065f95.js

/** 所有支持的图标名称联合类型 */
export type IconName =
  | 'chevron-right'
  | 'chevron-down'
  | 'plus'
  | 'search'
  | 'inbox'
  | 'star'
  | 'star-fill'
  | 'send'
  | 'draft'
  | 'archive'
  | 'trash'
  | 'reply'
  | 'reply-all'
  | 'forward'
  | 'attach'
  | 'settings'
  | 'more'
  | 'close'
  | 'minus'
  | 'tag'
  | 'folder'
  | 'compose'
  | 'paperclip'
  | 'bell'
  | 'sun'
  | 'moon'
  | 'check'
  | 'circle-dot'
  | 'mail'
  | 'user'

interface IconProps {
  name: IconName
  size?: number
  stroke?: number
  className?: string
}

/**
 * 通用 SVG 图标组件
 * - viewBox 0 0 16 16，stroke=currentColor，颜色完全继承父级
 * - size: 像素尺寸（默认 16）
 * - stroke: 线宽（默认 1.6）
 */
export function Icon({ name, size = 16, stroke = 1.6, className }: IconProps) {
  // 公共 SVG 属性
  const p = {
    width: size,
    height: size,
    viewBox: '0 0 16 16',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: stroke,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    className,
  }

  switch (name) {
    case 'chevron-right':
      return <svg {...p}><path d="M6 3.5L10.5 8 6 12.5"/></svg>

    case 'chevron-down':
      return <svg {...p}><path d="M3.5 6L8 10.5 12.5 6"/></svg>

    case 'plus':
      return <svg {...p}><path d="M8 3v10M3 8h10"/></svg>

    case 'search':
      return <svg {...p}><circle cx="7" cy="7" r="4"/><path d="M10 10l3 3"/></svg>

    case 'inbox':
      return (
        <svg {...p}>
          <path d="M2 9l2-5h8l2 5v3.5a.5.5 0 01-.5.5h-11a.5.5 0 01-.5-.5V9z"/>
          <path d="M2 9h3.5l1 1.5h3l1-1.5H14"/>
        </svg>
      )

    case 'star':
      return <svg {...p}><path d="M8 2.5l1.7 3.5 3.8.5-2.8 2.7.7 3.8L8 11.2 4.6 13l.7-3.8L2.5 6.5l3.8-.5z"/></svg>

    case 'star-fill':
      return <svg {...p} fill="currentColor"><path d="M8 2.5l1.7 3.5 3.8.5-2.8 2.7.7 3.8L8 11.2 4.6 13l.7-3.8L2.5 6.5l3.8-.5z"/></svg>

    case 'send':
      return <svg {...p}><path d="M13.5 2.5L2 7l5 1.5L8.5 13l5-10.5zM7 8.5l3-3"/></svg>

    case 'draft':
      return <svg {...p}><path d="M4 2h5l3 3v9H4V2z"/><path d="M9 2v3h3"/></svg>

    case 'archive':
      return <svg {...p}><rect x="2" y="3" width="12" height="3" rx="0.5"/><path d="M3 6v7h10V6M6.5 9h3"/></svg>

    case 'trash':
      return <svg {...p}><path d="M2.5 4.5h11M6 4.5V3h4v1.5M4 4.5v9h8v-9M6.5 7v4M9.5 7v4"/></svg>

    case 'reply':
      return <svg {...p}><path d="M6.5 3L2 7.5 6.5 12M2 7.5h7a4 4 0 014 4V13"/></svg>

    case 'reply-all':
      return (
        <svg {...p}>
          <path d="M4.5 3L1 6.5 4.5 10M7.5 3L4 6.5 7.5 10M4 6.5h6.5a3 3 0 013 3V12"/>
        </svg>
      )

    case 'forward':
      return <svg {...p}><path d="M9.5 3L14 7.5 9.5 12M14 7.5H7a4 4 0 00-4 4V13"/></svg>

    case 'attach':
    case 'paperclip':
      return <svg {...p}><path d="M10.5 7l-4 4a2 2 0 11-3-3l5-5a3 3 0 014.2 4.2L7.5 12.5"/></svg>

    case 'settings':
      return (
        <svg {...p}>
          <circle cx="8" cy="8" r="2"/>
          <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.5 3.5l1.4 1.4M11.1 11.1l1.4 1.4M3.5 12.5l1.4-1.4M11.1 4.9l1.4-1.4"/>
        </svg>
      )

    case 'more':
      return (
        <svg {...p}>
          <circle cx="3.5" cy="8" r="0.8" fill="currentColor"/>
          <circle cx="8" cy="8" r="0.8" fill="currentColor"/>
          <circle cx="12.5" cy="8" r="0.8" fill="currentColor"/>
        </svg>
      )

    case 'close':
      return <svg {...p}><path d="M3.5 3.5l9 9M12.5 3.5l-9 9"/></svg>

    case 'minus':
      return <svg {...p}><path d="M3 8h10"/></svg>

    case 'tag':
      return (
        <svg {...p}>
          <path d="M2.5 7.5V2.5h5L13 8l-5 5-5.5-5.5z"/>
          <circle cx="5" cy="5" r="0.5" fill="currentColor"/>
        </svg>
      )

    case 'folder':
      return <svg {...p}><path d="M2 4.5a1 1 0 011-1h3l1.5 1.5h5a1 1 0 011 1V12a1 1 0 01-1 1H3a1 1 0 01-1-1V4.5z"/></svg>

    case 'compose':
      return <svg {...p}><path d="M10 2.5L13.5 6 7 12.5H3.5V9z"/><path d="M2 14.5h12"/></svg>

    case 'bell':
      return (
        <svg {...p}>
          <path d="M4 11V7a4 4 0 118 0v4l1 1.5H3L4 11z"/>
          <path d="M6.5 13a1.5 1.5 0 003 0"/>
        </svg>
      )

    case 'sun':
      return (
        <svg {...p}>
          <circle cx="8" cy="8" r="2.5"/>
          <path d="M8 1.5V3M8 13v1.5M1.5 8H3M13 8h1.5M3.5 3.5l1 1M11.5 11.5l1 1M3.5 12.5l1-1M11.5 4.5l1-1"/>
        </svg>
      )

    case 'moon':
      return <svg {...p}><path d="M13 9.5A5.5 5.5 0 116.5 3a4.5 4.5 0 006.5 6.5z"/></svg>

    case 'check':
      return <svg {...p}><path d="M3 8.5L6.5 12 13 4.5"/></svg>

    case 'circle-dot':
      return (
        <svg {...p}>
          <circle cx="8" cy="8" r="5.5"/>
          <circle cx="8" cy="8" r="1.6" fill="currentColor"/>
        </svg>
      )

    case 'mail':
      return (
        <svg {...p}>
          <rect x="2" y="4" width="12" height="9" rx="1"/>
          <path d="M2 5l6 4.5L14 5"/>
        </svg>
      )

    case 'user':
      return (
        <svg {...p}>
          <circle cx="8" cy="6" r="3"/>
          <path d="M2 14c0-3.3 2.7-6 6-6s6 2.7 6 6"/>
        </svg>
      )

    default:
      return null
  }
}

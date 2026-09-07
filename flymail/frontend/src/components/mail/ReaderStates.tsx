// 阅读区的三种非内容状态：空态 / 骨架屏 / 加载失败。
// 单封 Reader 与会话手风琴共用同一套，换视图形态时观感不跳。

import { useTranslation } from 'react-i18next'

/** 未选中任何邮件或会话时的欢迎面板 */
export function ReaderEmpty() {
  const { t } = useTranslation()
  return (
    <section className="col reader">
      <div className="reader-empty">
        <div className="empty-inner">
          <h3>{t('reader.welcome')}</h3>
          <p>{t('reader.welcomeHint')}</p>
          {/* 快捷键提示表 */}
          <div className="shortcuts">
            <kbd>J / K</kbd><span>{t('reader.scKbNav')}</span>
            <kbd>C</kbd><span>{t('reader.scKbCompose')}</span>
            <kbd>R</kbd><span>{t('reader.scKbReply')}</span>
            <kbd>S</kbd><span>{t('reader.scKbStar')}</span>
            <kbd>⌘K</kbd><span>{t('reader.scKbSearch')}</span>
          </div>
        </div>
      </div>
    </section>
  )
}

/**
 * 加载中的骨架占位。
 *
 * ⚠ 根节点必须与正常态一样是 .col.reader：它带着 flex:1 1 auto / min-width:300px，
 * 换成普通 div 会退回 flex:0 1 auto（宽度由内容决定），第三栏在
 * 「骨架 → 正文」之间先塌缩再弹回，表现为每次点开邮件整个布局抖一下。
 */
export function ReaderSkeleton() {
  return (
    <section className="col reader animate-pulse" style={{ background: 'var(--bg)' }}>
      {/* 工具条骨架 */}
      <div
        className="reader-toolbar"
        style={{ borderBottom: '1px solid var(--rule)', background: 'var(--surface)' }}
      >
        {[60, 60, 50, 70].map((w, i) => (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={i}
            className="h-7 rounded-md"
            style={{ width: w, background: 'var(--bg-sunk)' }}
          />
        ))}
      </div>
      {/* 正文骨架 */}
      <div className="reader-scroll">
        <div className="reader-inner">
          {/* 主题 */}
          <div className="h-8 rounded mb-5" style={{ width: '55%', background: 'var(--bg-sunk)' }} />
          {/* thread-head */}
          <div className="flex items-center gap-3 mb-5">
            <div className="h-10 w-10 rounded-lg flex-shrink-0" style={{ background: 'var(--bg-sunk)' }} />
            <div className="flex flex-col gap-2 flex-1">
              <div className="h-3.5 rounded" style={{ width: 140, background: 'var(--bg-sunk)' }} />
              <div className="h-3 rounded" style={{ width: 200, background: 'var(--bg-sunk)' }} />
            </div>
            <div className="h-3 rounded" style={{ width: 80, background: 'var(--bg-sunk)' }} />
          </div>
          {/* 正文行 */}
          {[90, 75, 88, 65, 80].map((w, i) => (
            <div
              // eslint-disable-next-line react/no-array-index-key
              key={i}
              className="h-3.5 rounded mb-3"
              style={{ width: `${w}%`, background: 'var(--bg-sunk)' }}
            />
          ))}
        </div>
      </div>
    </section>
  )
}

/** 加载失败面板 */
export function ReaderError({ error }: { error: unknown }) {
  const { t } = useTranslation()
  const msg = error instanceof Error ? error.message : String(error ?? '')
  return (
    <section className="col reader">
      <div className="reader-empty">
        <div className="empty-inner">
          <h3 style={{ color: 'var(--ink-2)' }}>{t('reader.loadError')}</h3>
          <p>{t('reader.loadErrorHint')}</p>
          {msg && (
            <p
              style={{
                marginTop: 8,
                fontFamily: 'var(--font-mono)',
                fontSize: 11,
                color: 'var(--ink-4)',
                wordBreak: 'break-all',
              }}
            >
              {msg}
            </p>
          )}
        </div>
      </div>
    </section>
  )
}

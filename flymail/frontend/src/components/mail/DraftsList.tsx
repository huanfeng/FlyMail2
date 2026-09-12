import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useDrafts, useDeleteDraft, useSendDraft } from '@/lib/queries'
import { errorText } from '@/lib/format'
import type { Draft } from '@/lib/types'

interface Props {
  accountId: number
  onOpenDraft: (d: Draft) => void
}

export function DraftsList({ accountId, onOpenDraft }: Props) {
  const { t } = useTranslation()
  const draftsQuery = useDrafts(accountId)
  const drafts = draftsQuery.data ?? []
  const deleteDraft = useDeleteDraft()
  const sendDraft = useSendDraft()

  // 错误分支必须排在空态之前：请求失败时 data 回落成空数组，
  // 与「一封草稿都没有」同形——把故障显示成一个空草稿箱。
  //
  // 判据是 isLoadingError（= isError && 没有数据）而不是 isError：
  // 发信/删草稿都会 invalidate ['drafts']，用 isError 的话一次后台重取失败
  // 就会把缓存里好端端的草稿整列换成错误面板。
  if (draftsQuery.isLoadingError) {
    const detail = errorText(draftsQuery.error)
    return (
      <div className="list-error">
        <div className="list-error-title">{t('list.loadErrorTitle')}</div>
        <div>{t('compose.draftsLoadErrorHint')}</div>
        {detail && <div className="list-error-detail">{detail}</div>}
        <button
          type="button"
          className="pill-btn"
          style={{ marginTop: 14 }}
          onClick={() => void draftsQuery.refetch()}
        >
          {t('app.retry')}
        </button>
      </div>
    )
  }

  if (drafts.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <span className="text-sm" style={{ color: 'var(--ink-3)' }}>
          {t('compose.noDrafts')}
        </span>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      {drafts.map((d) => {
        const subject = d.subject.trim() || t('list.noSubject')
        const toSummary = d.to.join(', ')
        return (
          <button
            key={d.id}
            type="button"
            onClick={() => onOpenDraft(d)}
            className="group flex w-full items-start gap-3 border-b px-4 py-3 text-left transition-colors hover:bg-[var(--bg-hover)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
            style={{ borderColor: 'var(--rule)' }}
          >
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium" style={{ color: 'var(--ink)' }}>
                {subject}
              </p>
              {toSummary && (
                <p className="truncate text-[12px]" style={{ color: 'var(--ink-3)' }}>
                  {toSummary}
                </p>
              )}
            </div>
            {/* 操作按钮：发送 + 删除。
                显隐交给 .draft-actions（CSS 里带指针类型守卫）而不是 Tailwind 的
                group-hover：触摸设备上 :hover 永不触发，而 opacity:0 的元素照常接收点击——
                那就是两个看不见的「立即发送」和「删除草稿」躺在每一行右侧。
                这与第一轮修掉的 .mi-star / .mi-del 是同一个缺陷，同一条规矩。 */}
            <div className="draft-actions">
              <button
                type="button"
                title={t('compose.sendDraft')}
                aria-label={t('compose.sendDraft')}
                onClick={(e) => {
                  e.stopPropagation()
                  sendDraft.mutate({ id: d.id, accountId })
                }}
                className="icon-btn"
              >
                <Icon name="send" size={13} />
              </button>
              <button
                type="button"
                title={t('compose.deleteDraft')}
                aria-label={t('compose.deleteDraft')}
                onClick={(e) => {
                  e.stopPropagation()
                  deleteDraft.mutate({ id: d.id, accountId })
                }}
                className="icon-btn"
              >
                <Icon name="trash" size={13} />
              </button>
            </div>
          </button>
        )
      })}
    </div>
  )
}

import { useTranslation } from 'react-i18next'
import { Send, Trash2 } from 'lucide-react'
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
            {/* 操作按钮：发送 + 删除 */}
            <div className="flex shrink-0 items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
              <button
                type="button"
                title={t('compose.sendDraft')}
                aria-label={t('compose.sendDraft')}
                onClick={(e) => {
                  e.stopPropagation()
                  sendDraft.mutate({ id: d.id, accountId })
                }}
                className="rounded p-1 hover:bg-[var(--bg-hover)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)' }}
              >
                <Send size={13} />
              </button>
              <button
                type="button"
                title={t('compose.deleteDraft')}
                aria-label={t('compose.deleteDraft')}
                onClick={(e) => {
                  e.stopPropagation()
                  deleteDraft.mutate({ id: d.id, accountId })
                }}
                className="rounded p-1 hover:bg-[var(--bg-hover)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)' }}
              >
                <Trash2 size={13} />
              </button>
            </div>
          </button>
        )
      })}
    </div>
  )
}

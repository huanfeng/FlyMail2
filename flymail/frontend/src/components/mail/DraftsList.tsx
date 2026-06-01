import { useTranslation } from 'react-i18next'
import { Send, Trash2 } from 'lucide-react'
import { useDrafts, useDeleteDraft, useSendDraft } from '@/lib/queries'
import type { Draft } from '@/lib/types'

interface Props {
  accountId: number
  onOpenDraft: (d: Draft) => void
}

export function DraftsList({ accountId, onOpenDraft }: Props) {
  const { t } = useTranslation()
  const { data: drafts = [] } = useDrafts(accountId)
  const deleteDraft = useDeleteDraft()
  const sendDraft = useSendDraft()

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
                title={t('account.delete')}
                aria-label={t('account.delete')}
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

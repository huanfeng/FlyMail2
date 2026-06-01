import { useTranslation } from 'react-i18next'
import { Paperclip } from 'lucide-react'
import type { Folder, MessageListItem } from '@/lib/types'

interface Props {
  folder: Folder | null
  messages: MessageListItem[]
  loading: boolean
  activeMessageId: number | null
  onSelectMessage: (id: number) => void
}

function initials(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getMonth() + 1}/${d.getDate()}`
}

export function MailList({ folder, messages, loading, activeMessageId, onSelectMessage }: Props) {
  const { t } = useTranslation()
  const title = folder
    ? folder.type === 'custom'
      ? folder.display_name
      : t(`folder.${folder.type}`)
    : ''
  return (
    <div className="flex h-full flex-col">
      <div className="px-5 py-3" style={{ borderBottom: '1px solid var(--rule)' }}>
        <div className="text-lg font-medium">{title}</div>
        {folder && folder.unread_count > 0 && (
          <div className="text-[11px]" style={{ color: 'var(--ink-3)' }}>
            {t('list.unreadCount', { count: folder.unread_count })}
          </div>
        )}
      </div>
      <div className="flex-1 overflow-y-auto">
        {loading && (
          <div className="px-5 py-8 text-center text-sm" style={{ color: 'var(--ink-3)' }}>
            …
          </div>
        )}
        {!loading && messages.length === 0 && (
          <div className="px-5 py-8 text-center text-sm" style={{ color: 'var(--ink-3)' }}>
            {t('list.empty')}
          </div>
        )}
        {messages.map((m) => (
          <button
            key={m.id}
            type="button"
            onClick={() => onSelectMessage(m.id)}
            className="grid w-full grid-cols-[32px_1fr] gap-3 px-5 py-3.5 text-left"
            style={{
              borderBottom: '1px solid var(--rule)',
              background: m.id === activeMessageId ? 'var(--accent-wash)' : 'transparent',
            }}
          >
            <div
              className="flex h-8 w-8 items-center justify-center rounded-md text-[12.5px] font-semibold text-white"
              style={{ background: 'var(--accent-color)' }}
            >
              {initials(m.from_name, m.from_addr)}
            </div>
            <div className="min-w-0">
              <div className="flex items-center justify-between gap-2">
                <span
                  className="truncate text-[13.5px]"
                  style={{
                    color: m.seen ? 'var(--ink-2)' : 'var(--ink)',
                    fontWeight: m.seen ? 400 : 600,
                  }}
                >
                  {m.from_name || m.from_addr}
                </span>
                <span
                  className="flex items-center gap-1 text-[10.5px]"
                  style={{ color: 'var(--ink-3)' }}
                >
                  {m.has_attachment && <Paperclip size={11} />}
                  {formatDate(m.date)}
                </span>
              </div>
              <div
                className="truncate text-[13px]"
                style={{
                  color: m.seen ? 'var(--ink-2)' : 'var(--ink)',
                  fontWeight: m.seen ? 400 : 600,
                }}
              >
                {m.subject || t('list.noSubject')}
              </div>
              {m.snippet && (
                <div className="truncate text-[12px]" style={{ color: 'var(--ink-3)' }}>
                  {m.snippet}
                </div>
              )}
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

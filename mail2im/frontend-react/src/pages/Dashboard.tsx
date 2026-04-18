import { useQueries } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { format } from 'date-fns'
import { RefreshCw, Mail, Users, Globe, Bell } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import api, { extractList } from '@/services/api'

interface LogEntry {
  id?: number
  ID?: number
  status: string
  priority: number
  action?: string
  channel_name?: string
  channel?: string
  channel_id?: number
  from?: string
  subject?: string
  received_at?: string
  forwarded_at?: string
  error?: string
  request?: string
  response?: string
  message_id?: string
}

interface EmailsResponse {
  total?: number
  data?: unknown[]
}

function StatCard({
  label,
  value,
  icon: Icon,
}: {
  label: string
  value: number | string
  icon: React.ElementType
}) {
  return (
    <div className="rounded-xl border bg-card p-5 flex items-start gap-4 shadow-xs">
      <div className="h-10 w-10 rounded-lg bg-primary/8 flex items-center justify-center shrink-0">
        <Icon className="h-5 w-5 text-primary" />
      </div>
      <div className="flex flex-col gap-0.5 min-w-0">
        <span className="text-sm text-muted-foreground">{label}</span>
        <span className="text-2xl font-semibold tabular-nums">{value}</span>
      </div>
    </div>
  )
}

function getStatusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'success':
      return 'default'
    case 'failed':
      return 'destructive'
    case 'received':
      return 'secondary'
    default:
      return 'outline'
  }
}

export function DashboardPage() {
  const { t } = useTranslation()

  const results = useQueries({
    queries: [
      {
        queryKey: ['emails-count'],
        queryFn: async () => {
          const res = await api.get<EmailsResponse | unknown[]>('/emails', { params: { page: 1, pageSize: 1 } })
          const data = res.data as EmailsResponse & unknown[]
          return (data as EmailsResponse).total ?? (Array.isArray(data) ? data.length : 0)
        },
      },
      {
        queryKey: ['accounts'],
        queryFn: async () => {
          const res = await api.get<unknown[]>('/accounts')
          return Array.isArray(res.data) ? res.data.length : 0
        },
      },
      {
        queryKey: ['proxies'],
        queryFn: async () => {
          const res = await api.get<unknown[]>('/proxies')
          return Array.isArray(res.data) ? res.data.length : 0
        },
      },
      {
        queryKey: ['channels'],
        queryFn: async () => {
          const res = await api.get<unknown[]>('/channels')
          return Array.isArray(res.data) ? res.data.length : 0
        },
      },
      {
        queryKey: ['logs-recent'],
        queryFn: async () => {
          const res = await api.get<LogEntry[]>('/logs')
          return extractList(res.data).slice(0, 5)
        },
      },
    ],
  })

  const [emailsQ, accountsQ, proxiesQ, channelsQ, logsQ] = results

  const isLoading = results.some((r) => r.isLoading)

  const refetchAll = () => {
    results.forEach((r) => r.refetch())
  }

  const stats = [
    { label: t('dashboard.emails'), value: emailsQ.data ?? '-', icon: Mail },
    { label: t('dashboard.accounts'), value: accountsQ.data ?? '-', icon: Users },
    { label: t('dashboard.proxies'), value: proxiesQ.data ?? '-', icon: Globe },
    { label: t('dashboard.channels'), value: channelsQ.data ?? '-', icon: Bell },
  ]

  const recentLogs: LogEntry[] = logsQ.data ?? []

  return (
    <div className="flex flex-col gap-6 max-w-5xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t('dashboard.title')}</h1>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={refetchAll}
          disabled={isLoading}
          aria-label={t('common.refresh')}
        >
          <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
        </Button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {stats.map((s) => (
          <StatCard key={s.label} label={s.label} value={s.value} icon={s.icon} />
        ))}
      </div>

      {/* Recent Logs */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
          {t('dashboard.recent_logs')}
        </h2>

        {logsQ.isLoading ? (
          <p className="text-sm text-muted-foreground">{t('dashboard.loading')}</p>
        ) : recentLogs.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('dashboard.no_logs')}</p>
        ) : (
          <div className="flex flex-col gap-2">
            {recentLogs.map((log, i) => {
              const dateStr = log.received_at ?? log.forwarded_at
              return (
                <div
                  key={log.id ?? log.ID ?? i}
                  className="rounded-xl border bg-card px-4 py-3 flex flex-col gap-1"
                >
                  <div className="flex items-center gap-2">
                    <Badge variant={getStatusVariant(log.status)}>{log.status}</Badge>
                    {dateStr && (
                      <span className="text-xs text-muted-foreground">
                        {format(new Date(dateStr), 'yyyy-MM-dd HH:mm:ss')}
                      </span>
                    )}
                  </div>
                  <p className="text-sm font-medium truncate">{log.subject || '-'}</p>
                  <p className="text-xs text-muted-foreground truncate">{log.from || ''}</p>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

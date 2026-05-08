import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { RefreshCw, X, Activity } from 'lucide-react'

import http from '@/services/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { PageHeader } from '@/components/PageHeader'

// ─── Types ────────────────────────────────────────────────────────────────────

interface WorkerLog {
  time: string
  level: string
  state: string
  message: string
}

interface Worker {
  account_id: number
  email: string
  state: string
  logs: WorkerLog[]
}

interface Stats {
  total: number
  workers: Worker[]
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getWorkerCardClass(state: string): string {
  switch (state) {
    case 'idle':
      return 'bg-green-100 border-green-500 text-green-900 dark:bg-green-950 dark:border-green-700 dark:text-green-200'
    case 'polling':
      return 'bg-blue-100 border-blue-500 text-blue-900 dark:bg-blue-950 dark:border-blue-700 dark:text-blue-200'
    case 'error':
      return 'bg-red-100 border-red-500 text-red-900 dark:bg-red-950 dark:border-red-700 dark:text-red-200'
    case 'connecting':
      return 'bg-yellow-100 border-yellow-500 text-yellow-900 dark:bg-yellow-950 dark:border-yellow-700 dark:text-yellow-200'
    default:
      return 'bg-muted border-border text-muted-foreground'
  }
}

function getLevelVariant(level: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (level) {
    case 'error':
      return 'destructive'
    case 'warn':
      return 'outline'
    case 'debug':
      return 'secondary'
    default:
      return 'default'
  }
}

// ─── DebugPage ────────────────────────────────────────────────────────────────

export function DebugPage() {
  const { t } = useTranslation()
  const [selectedWorker, setSelectedWorker] = useState<Worker | null>(null)

  const { data, isLoading, refetch, isFetching } = useQuery<Stats>({
    queryKey: ['debug-stats'],
    queryFn: async () => {
      const res = await http.get('/debug/stats')
      return res.data ?? { total: 0, workers: [] }
    },
    refetchInterval: 2000,
    select: (data) => {
      // Keep selected worker in sync with latest data
      if (selectedWorker) {
        const updated = data.workers.find(
          (w) => w.account_id === selectedWorker.account_id,
        )
        if (updated) {
          setSelectedWorker(updated)
        }
      }
      return data
    },
  })

  const stats = data ?? { total: 0, workers: [] }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <PageHeader
        title={t('menu.debug')}
        subtitle={t('debug.subtitle')}
        className="px-4 md:px-6 py-4 border-b"
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isLoading || isFetching}>
            <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
          </Button>
        }
      />

      {/* Content */}
      <div className="flex-1 overflow-auto px-4 md:px-6 py-6 space-y-6">
        {/* Summary card */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="rounded-xl border bg-card p-5 flex items-start gap-4 shadow-xs">
            <div className="h-10 w-10 rounded-lg bg-primary/8 flex items-center justify-center shrink-0">
              <Activity className="h-5 w-5 text-primary" />
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="text-sm text-muted-foreground">{t('debug.total_workers')}</span>
              <span className="text-2xl font-semibold tabular-nums">{stats.total}</span>
            </div>
          </div>
        </div>

        {/* Worker grid */}
        <div className="grid grid-cols-2 md:grid-cols-5 lg:grid-cols-8 gap-2">
          {stats.workers.length === 0 ? (
            <div className="col-span-full text-center py-12 text-muted-foreground border rounded-xl">
              {t('debug.no_workers')}
            </div>
          ) : (
            stats.workers.map((worker) => (
              <button
                key={worker.account_id}
                type="button"
                onClick={() => setSelectedWorker(worker)}
                className={`aspect-square rounded-lg flex items-center justify-center cursor-pointer border-2 transition-colors hover:opacity-80 ${getWorkerCardClass(worker.state)} ${
                  selectedWorker?.account_id === worker.account_id
                    ? 'ring-2 ring-primary ring-offset-2'
                    : ''
                }`}
              >
                <div className="text-center overflow-hidden w-full px-1">
                  <div className="font-bold text-sm">#{worker.account_id}</div>
                  <div className="text-xs truncate">{worker.email}</div>
                  <div className="text-[10px] uppercase mt-0.5 opacity-80">{worker.state}</div>
                </div>
              </button>
            ))
          )}
        </div>

        {/* Log panel */}
        {selectedWorker && (
          <div className="rounded-xl bg-gray-900 text-green-400 p-4 shadow-lg h-96 overflow-y-auto font-mono text-sm">
            <div className="flex justify-between items-center border-b border-gray-700 pb-2 mb-3">
              <span className="font-bold text-green-300">
                {t('debug.logs_for', { email: selectedWorker.email })}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-gray-400 hover:text-white hover:bg-gray-700"
                onClick={() => setSelectedWorker(null)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
            {selectedWorker.logs && selectedWorker.logs.length > 0 ? (
              selectedWorker.logs.map((log, index) => (
                <div
                  key={index}
                  className="flex items-start gap-2 text-sm py-0.5 whitespace-pre-wrap"
                >
                  <span className="text-gray-500 shrink-0 tabular-nums">
                    {new Date(log.time).toLocaleTimeString()}
                  </span>
                  <Badge
                    variant={getLevelVariant(log.level)}
                    className="shrink-0 text-[10px] px-1 py-0"
                  >
                    {log.level}
                  </Badge>
                  <span className="break-all">{log.message}</span>
                </div>
              ))
            ) : (
              <div className="text-gray-500 text-sm">暂无日志</div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

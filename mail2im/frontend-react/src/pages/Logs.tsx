import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  flexRender,
  type ColumnDef,
} from '@tanstack/react-table'
import { format } from 'date-fns'
import { RefreshCw, Trash2, Eye, ChevronLeft, ChevronRight } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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

function getPriorityVariant(priority: number): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (priority >= 3) return 'destructive'
  if (priority === 2) return 'default'
  if (priority === 1) return 'secondary'
  return 'outline'
}

function getPriorityLabel(priority: number, t: (key: string) => string): string {
  switch (priority) {
    case 3: return t('common.priority_critical')
    case 2: return t('common.priority_high')
    case 1: return t('common.priority_normal')
    default: return t('common.priority_low')
  }
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return '-'
  try {
    return format(new Date(dateStr), 'yyyy-MM-dd HH:mm:ss')
  } catch {
    return dateStr
  }
}

function getLogId(log: LogEntry): number | undefined {
  return log.id ?? log.ID
}

// Detail Dialog
function LogDetailDialog({
  log,
  open,
  onOpenChange,
}: {
  log: LogEntry | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  if (!log) return null

  const rows: { label: string; content: React.ReactNode }[] = [
    { label: t('logs.action'), content: log.action || '-' },
    {
      label: t('logs.status'),
      content: <Badge variant={getStatusVariant(log.status)}>{log.status}</Badge>,
    },
    {
      label: t('logs.priority'),
      content: (
        <Badge variant={getPriorityVariant(log.priority)}>
          {getPriorityLabel(log.priority, t)}
        </Badge>
      ),
    },
    {
      label: t('logs.channel'),
      content: (
        <span>
          {log.channel_name || log.channel || '-'}
          {log.channel_id ? (
            <span className="ml-1 text-muted-foreground text-xs">(#{log.channel_id})</span>
          ) : null}
        </span>
      ),
    },
    { label: t('logs.from'), content: log.from || '-' },
    { label: t('logs.subject'), content: log.subject || '-' },
    { label: t('logs.message_id'), content: log.message_id || '-' },
    {
      label: t('logs.time'),
      content: formatDate(log.received_at ?? log.forwarded_at),
    },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('logs.details')}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3 text-sm">
          {rows.map((row) => (
            <div key={row.label} className="flex items-start gap-3">
              <span className="min-w-24 text-muted-foreground font-medium shrink-0">
                {row.label}
              </span>
              <span className="break-all">{row.content}</span>
            </div>
          ))}
          {log.request && (
            <div className="flex flex-col gap-1">
              <span className="text-muted-foreground font-medium">{t('logs.request')}</span>
              <pre className="bg-muted rounded-lg p-3 text-xs whitespace-pre-wrap break-all font-mono overflow-auto max-h-40">
                {log.request}
              </pre>
            </div>
          )}
          {log.response && (
            <div className="flex flex-col gap-1">
              <span className="text-muted-foreground font-medium">{t('logs.response')}</span>
              <pre className="bg-muted rounded-lg p-3 text-xs whitespace-pre-wrap break-all font-mono overflow-auto max-h-40">
                {log.response}
              </pre>
            </div>
          )}
          {log.error && (
            <div className="flex flex-col gap-1">
              <span className="text-destructive font-medium">{t('logs.error')}</span>
              <pre className="bg-destructive/10 text-destructive rounded-lg p-3 text-xs whitespace-pre-wrap break-all font-mono overflow-auto max-h-40">
                {log.error}
              </pre>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

export function LogsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [detailLog, setDetailLog] = useState<LogEntry | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<LogEntry | null>(null)
  const [showClearDialog, setShowClearDialog] = useState(false)

  const { data: logs = [], isLoading, refetch } = useQuery<LogEntry[]>({
    queryKey: ['logs'],
    queryFn: async () => {
      const res = await api.get<LogEntry[]>('/logs')
      return extractList(res.data)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (log: LogEntry) => {
      const id = getLogId(log)
      await api.delete(`/logs/${id}`)
    },
    onSuccess: () => {
      toast.success(t('logs.delete_success'))
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['logs-recent'] })
    },
    onError: () => {
      toast.error(t('logs.delete_error'))
    },
  })

  const clearMutation = useMutation({
    mutationFn: async () => {
      await api.delete('/logs')
    },
    onSuccess: () => {
      toast.success(t('logs.clear_success'))
      setShowClearDialog(false)
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['logs-recent'] })
    },
    onError: () => {
      toast.error(t('logs.clear_error'))
    },
  })

  const columns: ColumnDef<LogEntry>[] = [
    {
      accessorKey: 'status',
      header: t('logs.status'),
      cell: ({ row }) => (
        <Badge variant={getStatusVariant(row.original.status)}>
          {row.original.status}
        </Badge>
      ),
    },
    {
      accessorKey: 'priority',
      header: t('logs.priority'),
      cell: ({ row }) => (
        <Badge variant={getPriorityVariant(row.original.priority)}>
          {getPriorityLabel(row.original.priority, t)}
        </Badge>
      ),
    },
    {
      id: 'channel',
      header: t('logs.channel'),
      cell: ({ row }) => (
        <span>{row.original.channel_name || row.original.channel || '-'}</span>
      ),
    },
    {
      accessorKey: 'from',
      header: t('logs.from'),
      cell: ({ row }) => (
        <span className="max-w-40 truncate block">{row.original.from || '-'}</span>
      ),
    },
    {
      accessorKey: 'subject',
      header: t('logs.subject'),
      cell: ({ row }) => (
        <span className="max-w-48 truncate block">{row.original.subject || '-'}</span>
      ),
    },
    {
      id: 'time',
      header: t('logs.time'),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {formatDate(row.original.received_at ?? row.original.forwarded_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => (
        <div className="flex items-center gap-1 justify-end">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={() => setDetailLog(row.original)}
            aria-label={t('logs.details')}
          >
            <Eye className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-destructive hover:text-destructive"
            onClick={() => setDeleteTarget(row.original)}
            aria-label={t('logs.delete')}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      ),
    },
  ]

  const table = useReactTable({
    data: logs,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize: 20 } },
  })

  const { pageIndex, pageSize } = table.getState().pagination
  const totalRows = logs.length
  const from = totalRows === 0 ? 0 : pageIndex * pageSize + 1
  const to = Math.min((pageIndex + 1) * pageSize, totalRows)

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">{t('logs.title')}</h1>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => refetch()}
            disabled={isLoading}
            aria-label={t('common.refresh')}
          >
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="text-destructive hover:text-destructive"
            onClick={() => setShowClearDialog(true)}
            aria-label={t('logs.clear')}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Table */}
      <div className="rounded-xl border bg-card overflow-hidden flex flex-col flex-1 min-h-0">
        <div className="overflow-auto flex-1">
          <Table>
            <TableHeader>
              {table.getHeaderGroups().map((hg) => (
                <TableRow key={hg.id}>
                  {hg.headers.map((header) => (
                    <TableHead key={header.id}>
                      {header.isPlaceholder
                        ? null
                        : flexRender(header.column.columnDef.header, header.getContext())}
                    </TableHead>
                  ))}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={columns.length} className="text-center py-10 text-muted-foreground text-sm">
                    {t('dashboard.loading')}
                  </TableCell>
                </TableRow>
              ) : table.getRowModel().rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={columns.length} className="text-center py-10 text-muted-foreground text-sm">
                    {t('dashboard.no_logs')}
                  </TableCell>
                </TableRow>
              ) : (
                table.getRowModel().rows.map((row) => (
                  <TableRow
                    key={row.id}
                    className="cursor-pointer"
                    onClick={() => setDetailLog(row.original)}
                  >
                    {row.getVisibleCells().map((cell) => (
                      <TableCell
                        key={cell.id}
                        onClick={
                          cell.column.id === 'actions'
                            ? (e) => e.stopPropagation()
                            : undefined
                        }
                      >
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {/* Pagination */}
        <div className="flex items-center justify-between px-4 py-3 border-t text-sm text-muted-foreground">
          <span>
            {totalRows === 0
              ? '0'
              : `${from}–${to} / ${totalRows}`}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="px-2 text-xs">
              {pageIndex + 1} / {table.getPageCount() || 1}
            </span>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Detail Dialog */}
      <LogDetailDialog
        log={detailLog}
        open={detailLog !== null}
        onOpenChange={(open) => { if (!open) setDetailLog(null) }}
      />

      {/* Delete single log */}
      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('logs.delete_confirm')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.no')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
              disabled={deleteMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Clear all logs */}
      <AlertDialog open={showClearDialog} onOpenChange={setShowClearDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('logs.clear_confirm')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.no')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => clearMutation.mutate()}
              disabled={clearMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

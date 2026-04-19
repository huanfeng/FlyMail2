import { useState, useCallback } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
} from '@tanstack/react-table'
import { toast } from 'sonner'
import { Trash2, RefreshCw, ExternalLink, ChevronLeft, ChevronRight, ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react'
import api from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
import { Badge } from '@/components/ui/badge'
import { PageHeader } from '@/components/PageHeader'

type Email = {
  id: number
  subject: string
  from: string
  to: string
  mailbox: string
  mailbox_path?: string
  mail_type: string
  received_at: string
  is_read: boolean
}

type EmailsResponse = {
  data: Email[]
  total: number
  page: number
  page_size: number
}

const PAGE_SIZE = 20

function formatDate(dateStr: string) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return isNaN(d.getTime()) ? '-' : d.toLocaleString()
}

function mailTypeBadgeVariant(type: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch ((type || '').toLowerCase()) {
    case 'important':
      return 'default'
    case 'spam':
    case 'trash':
      return 'destructive'
    case 'promotion':
    case 'social':
      return 'outline'
    default:
      return 'secondary'
  }
}

const columnHelper = createColumnHelper<Email>()

export function EmailsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [sorting, setSorting] = useState<SortingState>([{ id: 'received_at', desc: true }])

  const [deleteTarget, setDeleteTarget] = useState<Email | null>(null)
  const [showDeleteAll, setShowDeleteAll] = useState(false)

  const sortBy = sorting[0]?.id ?? 'received_at'
  const sortOrder = sorting[0]?.desc === false ? 'asc' : 'desc'

  const { data, isLoading, isFetching } = useQuery<EmailsResponse>({
    queryKey: ['emails', page, PAGE_SIZE, search, sortBy, sortOrder],
    queryFn: async () => {
      const res = await api.get('/emails', {
        params: { page, page_size: PAGE_SIZE, search, sort_by: sortBy, sort_order: sortOrder },
      })
      return res.data as EmailsResponse
    },
    placeholderData: (prev) => prev,
  })

  const emails = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.delete(`/emails/${id}`),
    onSuccess: () => {
      toast.success(t('emails_view.delete_success'))
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['emails'] })
    },
    onError: () => toast.error(t('emails_view.delete_error')),
  })

  const deleteAllMutation = useMutation({
    mutationFn: () => api.delete('/emails'),
    onSuccess: () => {
      toast.success(t('emails_view.delete_all_success'))
      setShowDeleteAll(false)
      setPage(1)
      queryClient.invalidateQueries({ queryKey: ['emails'] })
    },
    onError: () => toast.error(t('emails_view.delete_all_error')),
  })

  const handleSearch = useCallback(() => {
    setSearch(searchInput)
    setPage(1)
  }, [searchInput])

  const handleSortToggle = useCallback(
    (colId: string) => {
      setSorting((prev) => {
        if (prev[0]?.id === colId) {
          return [{ id: colId, desc: !prev[0].desc }]
        }
        return [{ id: colId, desc: true }]
      })
      setPage(1)
    },
    []
  )

  const columns = [
    columnHelper.accessor('subject', {
      header: t('common.subject'),
      cell: (info) => (
        <span
          className={`cursor-pointer hover:text-primary hover:underline transition-colors ${
            !info.row.original.is_read ? 'font-semibold' : 'font-normal'
          }`}
          onClick={() => navigate(`/emails/${info.row.original.id}`)}
        >
          {info.getValue() || '-'}
        </span>
      ),
    }),
    columnHelper.accessor('from', {
      header: t('common.from'),
      cell: (info) => (
        <span className="text-muted-foreground truncate max-w-[160px] block" title={info.getValue()}>
          {info.getValue() || '-'}
        </span>
      ),
    }),
    columnHelper.accessor('mailbox', {
      header: t('emails_view.mailbox'),
      cell: (info) => (
        <span className="text-muted-foreground">
          {info.getValue() || info.row.original.mailbox_path || '-'}
        </span>
      ),
    }),
    columnHelper.accessor('mail_type', {
      header: t('emails_view.mail_type'),
      cell: (info) => {
        const type = info.getValue() || ''
        const label = t(`emails_view.type_${type.toLowerCase()}`, {
          defaultValue: t('emails_view.type_normal'),
        })
        return <Badge variant={mailTypeBadgeVariant(type)}>{label}</Badge>
      },
    }),
    columnHelper.accessor('received_at', {
      header: t('common.received_at'),
      cell: (info) => (
        <span className="text-muted-foreground text-sm whitespace-nowrap">
          {formatDate(info.getValue())}
        </span>
      ),
    }),
    columnHelper.display({
      id: 'actions',
      cell: (info) => (
        <div className="flex items-center gap-1 justify-end">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(`/emails/${info.row.original.id}`)}
            title={t('common.view')}
          >
            <ExternalLink className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => setDeleteTarget(info.row.original)}
            title={t('emails_view.delete_confirm')}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      ),
    }),
  ]

  const table = useReactTable({
    data: emails,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    manualPagination: true,
    pageCount: totalPages,
  })

  const sortableColumns = ['subject', 'from', 'mailbox', 'mail_type', 'received_at']

  return (
    <div className="flex flex-col h-full gap-4">
      <PageHeader
        title={t('common.emails')}
        subtitle={total > 0 ? `${total} ${t('common.emails')}` : undefined}
        actions={
          <>
            <Input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              placeholder={t('common.search')}
              className="w-56"
            />
            <Button variant="outline" size="sm" onClick={handleSearch}>
              {t('common.search')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => queryClient.invalidateQueries({ queryKey: ['emails'] })}
              title={t('common.refresh')}
            >
              <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={() => setShowDeleteAll(true)}
              disabled={total === 0 || isLoading}
              title={t('emails_view.delete_all')}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </>
        }
      />

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id} className="bg-muted/40 hover:bg-muted/40">
                {headerGroup.headers.map((header) => {
                  const isSortable = sortableColumns.includes(header.id)
                  const currentSort = sorting[0]
                  const isActiveSorted = currentSort?.id === header.id
                  return (
                    <TableHead
                      key={header.id}
                      className={`font-medium text-foreground ${isSortable ? 'cursor-pointer select-none' : ''}`}
                      onClick={isSortable ? () => handleSortToggle(header.id) : undefined}
                    >
                      <div className="flex items-center gap-1">
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {isSortable && (
                          <span className="text-muted-foreground">
                            {isActiveSorted ? (
                              currentSort.desc ? (
                                <ArrowDown className="h-3 w-3" />
                              ) : (
                                <ArrowUp className="h-3 w-3" />
                              )
                            ) : (
                              <ArrowUpDown className="h-3 w-3 opacity-40" />
                            )}
                          </span>
                        )}
                      </div>
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="text-center py-16 text-muted-foreground">
                  <RefreshCw className="h-5 w-5 animate-spin mx-auto mb-2" />
                  <span className="text-sm">{t('dashboard.loading')}</span>
                </TableCell>
              </TableRow>
            ) : emails.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="text-center py-16 text-muted-foreground text-sm">
                  {t('common.no_emails')}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="cursor-pointer hover:bg-muted/30 transition-colors"
                  onClick={() => navigate(`/emails/${row.original.id}`)}
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
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          {total > 0
            ? `${(page - 1) * PAGE_SIZE + 1}–${Math.min(page * PAGE_SIZE, total)} / ${total}`
            : '0'}
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1 || isLoading}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="px-2 font-medium text-foreground">
            {page} / {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages || isLoading}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Delete single email dialog */}
      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('emails_view.delete_confirm')}
              <span className="block mt-1 text-xs">{t('emails_view.delete_notice')}</span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.no')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
              disabled={deleteMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete all emails dialog */}
      <AlertDialog open={showDeleteAll} onOpenChange={setShowDeleteAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('emails_view.delete_all_confirm')}
              <span className="block mt-1 text-xs">{t('emails_view.delete_notice')}</span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.no')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteAllMutation.mutate()}
              disabled={deleteAllMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

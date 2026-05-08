import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, RefreshCw, Pencil, Trash2, TriangleAlert } from 'lucide-react'

import http, { extractList } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
  DialogFooter,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PageHeader } from '@/components/PageHeader'

// ─── Types ────────────────────────────────────────────────────────────────────

interface Proxy {
  ID: number
  name: string
  type: string
  host: string
  port: number
  username?: string
  password?: string
}

interface ProxyForm {
  id: number | null
  name: string
  type: string
  host: string
  port: number | ''
  username: string
  password: string
}

// ─── Constants ────────────────────────────────────────────────────────────────

const PROXY_TYPES = [
  { label: 'SOCKS5', value: 'socks5' },
  { label: 'HTTP', value: 'http' },
]

const DEFAULT_FORM: ProxyForm = {
  id: null,
  name: '',
  type: 'socks5',
  host: '',
  port: '',
  username: '',
  password: '',
}

// ─── ProxyDialog ──────────────────────────────────────────────────────────────

interface ProxyDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: ProxyForm
  setForm: React.Dispatch<React.SetStateAction<ProxyForm>>
  onSave: () => void
  isSaving: boolean
  submitted: boolean
}

function ProxyDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  onSave,
  isSaving,
  submitted,
}: ProxyDialogProps) {
  const { t } = useTranslation()

  const FormRow = ({ label, children }: { label: string; children: React.ReactNode }) => (
    <div className="grid grid-cols-[120px_1fr] items-center gap-3">
      <Label className="text-sm text-muted-foreground text-right">{label}</Label>
      <div>{children}</div>
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('proxies.details') : t('common.new') + ' ' + t('menu.proxies')}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <FormRow label={t('proxies.name')}>
            <div>
              <Input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                autoFocus
                className={submitted && !form.name ? 'border-destructive' : ''}
              />
              {submitted && !form.name && (
                <p className="text-xs text-destructive mt-1">{t('common.required')}</p>
              )}
            </div>
          </FormRow>

          <FormRow label={t('proxies.type')}>
            <Select
              value={form.type}
              onValueChange={(v) => setForm((f) => ({ ...f, type: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROXY_TYPES.map((pt) => (
                  <SelectItem key={pt.value} value={pt.value}>
                    {pt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>

          <FormRow label={t('proxies.host')}>
            <div>
              <Input
                value={form.host}
                onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                className={submitted && !form.host ? 'border-destructive' : ''}
              />
              {submitted && !form.host && (
                <p className="text-xs text-destructive mt-1">{t('common.required')}</p>
              )}
            </div>
          </FormRow>

          <FormRow label={t('proxies.port')}>
            <div>
              <Input
                type="number"
                value={form.port}
                onChange={(e) =>
                  setForm((f) => ({ ...f, port: e.target.value ? parseInt(e.target.value) : '' }))
                }
                className={submitted && !form.port ? 'border-destructive' : ''}
              />
              {submitted && !form.port && (
                <p className="text-xs text-destructive mt-1">{t('common.required')}</p>
              )}
            </div>
          </FormRow>

          <FormRow label={t('proxies.username')}>
            <Input
              value={form.username}
              onChange={(e) => setForm((f) => ({ ...f, username: e.target.value }))}
            />
          </FormRow>

          <FormRow label={t('proxies.password')}>
            <Input
              type="password"
              value={form.password}
              onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
            />
          </FormRow>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="button" onClick={onSave} disabled={isSaving}>
            {isSaving ? '保存中...' : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ─── ProxiesPage ──────────────────────────────────────────────────────────────

export function ProxiesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Proxy | null>(null)
  const [isEdit, setIsEdit] = useState(false)
  const [form, setForm] = useState<ProxyForm>(DEFAULT_FORM)
  const [submitted, setSubmitted] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  // ── Queries ──
  const { data: proxies = [], isLoading, refetch } = useQuery<Proxy[]>({
    queryKey: ['proxies'],
    queryFn: async () => {
      const res = await http.get('/proxies')
      return extractList(res.data)
    },
  })

  // ── Delete mutation ──
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/proxies/${id}`)
    },
    onSuccess: () => {
      toast.success(t('proxies_view.delete_success'))
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['proxies'] })
    },
    onError: () => {
      toast.error(t('proxies_view.delete_error'))
    },
  })

  // ── Handlers ──
  const openCreate = () => {
    setIsEdit(false)
    setSubmitted(false)
    setForm(DEFAULT_FORM)
    setDialogOpen(true)
  }

  const openEdit = (proxy: Proxy) => {
    setIsEdit(true)
    setSubmitted(false)
    setForm({
      id: proxy.ID,
      name: proxy.name,
      type: proxy.type,
      host: proxy.host,
      port: proxy.port,
      username: proxy.username ?? '',
      password: '',
    })
    setDialogOpen(true)
  }

  const handleSave = async () => {
    setSubmitted(true)
    if (!form.name || !form.host || !form.port) return

    setIsSaving(true)
    try {
      const payload = {
        name: form.name,
        type: form.type,
        host: form.host,
        port: Number(form.port),
        username: form.username,
        password: form.password,
      }
      if (isEdit && form.id) {
        await http.put(`/proxies/${form.id}`, payload)
      } else {
        await http.post('/proxies', payload)
      }
      toast.success(t('proxies_view.save_success'))
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['proxies'] })
    } catch {
      toast.error(t('proxies_view.save_error'))
    } finally {
      setIsSaving(false)
    }
  }

  // ─── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title={t('menu.proxies')}
        subtitle={t('proxies.subtitle')}
        className="px-4 md:px-6 py-4 border-b"
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetch()}
              disabled={isLoading}
            >
              <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
            </Button>
            <Button size="sm" onClick={openCreate}>
              <Plus className="h-4 w-4 mr-1.5" />
              {t('common.new')}
            </Button>
          </>
        }
      />

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 md:px-6 py-4">
        <div className="rounded-lg border border-border overflow-hidden overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead>{t('proxies.name')}</TableHead>
              <TableHead>{t('proxies.type')}</TableHead>
              <TableHead>{t('proxies.host')}</TableHead>
              <TableHead>{t('proxies.port')}</TableHead>
              <TableHead>{t('proxies.username')}</TableHead>
              <TableHead className="w-24 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground py-16">
                  {t('dashboard.loading')}
                </TableCell>
              </TableRow>
            ) : proxies.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground py-16">
                  暂无代理
                </TableCell>
              </TableRow>
            ) : (
              proxies.map((proxy) => (
                <TableRow key={proxy.ID}>
                  <TableCell className="font-medium">{proxy.name}</TableCell>
                  <TableCell className="text-sm text-muted-foreground uppercase">
                    {proxy.type}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{proxy.host}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{proxy.port}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {proxy.username || '-'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => openEdit(proxy)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(proxy)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        </div>
      </div>

      {/* Proxy Dialog */}
      <ProxyDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        isEdit={isEdit}
        form={form}
        setForm={setForm}
        onSave={handleSave}
        isSaving={isSaving}
        submitted={submitted}
      />

      {/* Delete Confirm */}
      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-destructive/10">
              <TriangleAlert className="h-6 w-6 text-destructive" />
            </div>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget && t('common.delete_confirm', { name: deleteTarget.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.ID)}
              disabled={deleteMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

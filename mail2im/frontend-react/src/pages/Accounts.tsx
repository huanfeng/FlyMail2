import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  Plus,
  RefreshCw,
  Pencil,
  Trash2,
  PlugZap,
  FolderSync,
  TriangleAlert,
} from 'lucide-react'

import http, { extractList } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
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
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

// ─── Types ────────────────────────────────────────────────────────────────────

interface Account {
  ID: number
  display_name: string
  email: string
  login: string
  provider: string
  imap_server: string
  imap_port: number
  ssl_mode: string
  proxy_id: number | null
  use_idle: boolean
  poll_interval_day: number
  poll_interval_night: number
  timezone: string
  enabled: boolean
  status: string
  last_sync_at: string | null
}

interface Provider {
  id: string
  name: string
  servers?: Record<string, { host: string; port: number }>
}

interface Proxy {
  ID: number
  name: string
}

interface Mailbox {
  ID: number
  name: string
  path: string
  watch_mode: string
  type: string
  watch_status: string
}

interface AccountForm {
  id: number | null
  display_name: string
  email: string
  login: string
  password: string
  provider: string
  imap_server: string
  imap_port: number
  ssl_mode: string
  proxy_id: number | null
  use_idle: boolean
  poll_interval_day: number
  poll_interval_night: number
  timezone: string
  enabled: boolean
}

interface ConnectionStatus {
  ok: boolean
  message: string
  details: string[]
}

// ─── Constants ─────────────────────────────────────────────────────────────────

const SSL_MODES = [
  { label: 'SSL', value: 'ssl' },
  { label: 'STARTTLS', value: 'starttls' },
  { label: '不加密', value: 'none' },
]

const WATCH_MODES = [
  { label: 'IDLE (实时推送)', value: 'idle' },
  { label: 'Poll (定期轮询)', value: 'poll' },
  { label: 'None (忽略)', value: 'none' },
]

const FOLDER_TYPES = [
  { label: '收件箱', value: 'primary' },
  { label: '账单 / 票据', value: 'bill' },
  { label: '系统通知', value: 'notification' },
  { label: '推广', value: 'promotion' },
  { label: '社交', value: 'social' },
  { label: '垃圾', value: 'spam' },
  { label: '回收站', value: 'trash' },
  { label: '已发送', value: 'sent' },
  { label: '草稿', value: 'draft' },
  { label: '未知类型', value: 'unknown' },
]

const DEFAULT_FORM: AccountForm = {
  id: null,
  display_name: '',
  email: '',
  login: '',
  password: '',
  provider: '',
  imap_server: '',
  imap_port: 993,
  ssl_mode: 'ssl',
  proxy_id: null,
  use_idle: true,
  poll_interval_day: 60,
  poll_interval_night: 300,
  timezone: 'UTC',
  enabled: true,
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getStatusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'Active':
      return 'default'
    case 'AuthFailed':
      return 'destructive'
    case 'NetworkError':
      return 'outline'
    default:
      return 'secondary'
  }
}

function getStatusLabel(t: (k: string) => string, status: string): string {
  switch (status) {
    case 'Active':
      return t('accounts.status_active')
    case 'AuthFailed':
      return t('accounts.status_auth_failed')
    case 'NetworkError':
      return t('accounts.status_network_error')
    default:
      return t('accounts.status_unknown')
  }
}

// ─── AccountDialog ────────────────────────────────────────────────────────────

interface AccountDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: AccountForm
  setForm: React.Dispatch<React.SetStateAction<AccountForm>>
  providers: Provider[]
  proxies: Proxy[]
  mailboxes: Mailbox[]
  mailboxesLoading: boolean
  onSave: () => void
  onTest: () => void
  onSyncMailboxes: () => void
  onUpdateMailbox: (mb: Mailbox) => void
  isSaving: boolean
  isTesting: boolean
  connectionStatus: ConnectionStatus | null
  changePassword: boolean
  setChangePassword: (v: boolean) => void
}

function AccountDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  providers,
  proxies,
  mailboxes,
  mailboxesLoading,
  onSave,
  onTest,
  onSyncMailboxes,
  onUpdateMailbox,
  isSaving,
  isTesting,
  connectionStatus,
  changePassword,
  setChangePassword,
}: AccountDialogProps) {
  const { t } = useTranslation()

  const applyProviderDefaults = (providerId: string, sslMode: string) => {
    const provider = providers.find((p) => p.id === providerId)
    if (!provider) return
    const server = provider.servers?.[sslMode]
    setForm((f) => ({
      ...f,
      provider: providerId,
      imap_server: server?.host ?? f.imap_server,
      imap_port: server?.port ?? f.imap_port,
    }))
  }

  const handleProviderChange = (value: string) => {
    applyProviderDefaults(value, form.ssl_mode)
  }

  const handleSslModeChange = (value: string) => {
    setForm((f) => ({ ...f, ssl_mode: value }))
    applyProviderDefaults(form.provider, value)
  }

  // ── Section label component ──
  const SectionLabel = ({ children }: { children: React.ReactNode }) => (
    <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3 mt-1">
      {children}
    </h4>
  )

  // ── Form row component ──
  const FormRow = ({
    label,
    children,
  }: {
    label: string
    children: React.ReactNode
  }) => (
    <div className="grid grid-cols-[140px_1fr] items-center gap-3">
      <Label className="text-sm text-muted-foreground text-right">{label}</Label>
      <div className="flex items-center">{children}</div>
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('accounts.edit') : t('accounts.add')}
          </DialogTitle>
        </DialogHeader>

        <Tabs defaultValue="basic" className="flex-1 overflow-hidden flex flex-col">
          <TabsList className="w-fit">
            <TabsTrigger value="basic">{t('accounts.basic_tab')}</TabsTrigger>
            <TabsTrigger value="folders" disabled={!isEdit}>
              {t('accounts.folders_tab')}
            </TabsTrigger>
          </TabsList>

          {/* ── Basic Tab ── */}
          <TabsContent value="basic" className="flex-1 overflow-y-auto pr-1">
            <div className="space-y-5 py-2">
              {/* Section: Basic */}
              <div className="space-y-3">
                <SectionLabel>{t('accounts.section_basic')}</SectionLabel>
                <FormRow label={t('accounts.display_name')}>
                  <Input
                    className="w-full"
                    value={form.display_name}
                    onChange={(e) => setForm((f) => ({ ...f, display_name: e.target.value }))}
                  />
                </FormRow>
                <FormRow label={t('accounts.email')}>
                  <Input
                    className="w-full"
                    value={form.email}
                    onChange={(e) => setForm((f) => ({ ...f, email: e.target.value }))}
                  />
                </FormRow>
                <FormRow label={t('accounts.login')}>
                  <Input
                    className="w-full"
                    placeholder={t('accounts.login_placeholder')}
                    value={form.login}
                    onChange={(e) => setForm((f) => ({ ...f, login: e.target.value }))}
                  />
                </FormRow>
                <FormRow label={t('accounts.password')}>
                  {!isEdit || changePassword ? (
                    <Input
                      className="w-full"
                      type="password"
                      value={form.password}
                      onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
                    />
                  ) : (
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-muted-foreground">
                        {t('accounts.password_hidden')}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setChangePassword(true)}
                      >
                        {t('accounts.password_change')}
                      </Button>
                    </div>
                  )}
                </FormRow>
                <FormRow label={t('accounts.enabled')}>
                  <Switch
                    checked={form.enabled}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                  />
                </FormRow>
              </div>

              {/* Section: Server */}
              <div className="space-y-3">
                <SectionLabel>{t('accounts.section_server')}</SectionLabel>
                <FormRow label={t('accounts.provider')}>
                  <Select value={form.provider} onValueChange={handleProviderChange}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {providers.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormRow>
                <FormRow label={t('accounts.ssl_mode')}>
                  <Select value={form.ssl_mode} onValueChange={handleSslModeChange}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SSL_MODES.map((m) => (
                        <SelectItem key={m.value} value={m.value}>
                          {m.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormRow>
                <FormRow label={t('accounts.imap_server')}>
                  <Input
                    className="w-full"
                    value={form.imap_server}
                    onChange={(e) => setForm((f) => ({ ...f, imap_server: e.target.value }))}
                  />
                </FormRow>
                <FormRow label={t('accounts.port')}>
                  <Input
                    className="w-full"
                    type="number"
                    value={form.imap_port}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, imap_port: parseInt(e.target.value) || 993 }))
                    }
                  />
                </FormRow>
              </div>

              {/* Section: Policy */}
              <div className="space-y-3">
                <SectionLabel>{t('accounts.section_policy')}</SectionLabel>
                <FormRow label={t('accounts.proxy')}>
                  <Select
                    value={form.proxy_id != null ? String(form.proxy_id) : '__none__'}
                    onValueChange={(v) =>
                      setForm((f) => ({ ...f, proxy_id: v === '__none__' ? null : parseInt(v) }))
                    }
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder={t('accounts.no_proxy')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__none__">{t('accounts.no_proxy')}</SelectItem>
                      {proxies.map((p) => (
                        <SelectItem key={p.ID} value={String(p.ID)}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormRow>
                <FormRow label={t('accounts.use_idle')}>
                  <Switch
                    checked={form.use_idle}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, use_idle: v }))}
                  />
                </FormRow>
                <FormRow label={t('accounts.poll_day')}>
                  <Input
                    className="w-full"
                    type="number"
                    value={form.poll_interval_day}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        poll_interval_day: parseInt(e.target.value) || 60,
                      }))
                    }
                  />
                </FormRow>
                <FormRow label={t('accounts.poll_night')}>
                  <Input
                    className="w-full"
                    type="number"
                    value={form.poll_interval_night}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        poll_interval_night: parseInt(e.target.value) || 300,
                      }))
                    }
                  />
                </FormRow>
              </div>

              {/* Connection status */}
              {connectionStatus && (
                <div
                  className={`rounded-md border px-4 py-3 text-sm space-y-1 ${
                    connectionStatus.ok
                      ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-300'
                      : 'border-destructive/30 bg-destructive/10 text-destructive'
                  }`}
                >
                  <p className="font-medium">{connectionStatus.message}</p>
                  {connectionStatus.details.length > 0 && (
                    <ul className="list-disc ml-4 space-y-0.5 text-xs opacity-80">
                      {connectionStatus.details.map((d, i) => (
                        <li key={i}>{d}</li>
                      ))}
                    </ul>
                  )}
                </div>
              )}
            </div>
          </TabsContent>

          {/* ── Folders Tab ── */}
          <TabsContent value="folders" className="flex-1 overflow-hidden flex flex-col">
            <div className="flex items-center justify-between mb-3">
              <p className="text-sm text-muted-foreground">{t('accounts.folders_desc')}</p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onSyncMailboxes}
                disabled={mailboxesLoading}
              >
                <FolderSync className="h-4 w-4 mr-1.5" />
                {t('accounts.sync_folders')}
              </Button>
            </div>
            <div className="flex-1 overflow-y-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('accounts.folder_name')}</TableHead>
                    <TableHead className="w-44">{t('accounts.watch_mode')}</TableHead>
                    <TableHead className="w-36">{t('accounts.folder_type')}</TableHead>
                    <TableHead className="w-24">{t('accounts.status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mailboxesLoading ? (
                    <TableRow>
                      <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                        加载中...
                      </TableCell>
                    </TableRow>
                  ) : mailboxes.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                        暂无文件夹，请先同步
                      </TableCell>
                    </TableRow>
                  ) : (
                    mailboxes.map((mb) => (
                      <TableRow key={mb.ID}>
                        <TableCell>
                          <div className="flex flex-col">
                            <span className="font-medium text-sm">{mb.name}</span>
                            <span className="text-xs text-muted-foreground">{mb.path}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Select
                            value={mb.watch_mode}
                            onValueChange={(v) => onUpdateMailbox({ ...mb, watch_mode: v })}
                          >
                            <SelectTrigger size="sm" className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {WATCH_MODES.map((m) => (
                                <SelectItem key={m.value} value={m.value}>
                                  {m.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </TableCell>
                        <TableCell>
                          <Select
                            value={mb.type}
                            onValueChange={(v) => onUpdateMailbox({ ...mb, type: v })}
                          >
                            <SelectTrigger size="sm" className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {FOLDER_TYPES.map((ft) => (
                                <SelectItem key={ft.value} value={ft.value}>
                                  {ft.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={mb.watch_status === 'verified' ? 'default' : 'secondary'}
                          >
                            {mb.watch_status}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </TabsContent>
        </Tabs>

        <DialogFooter className="border-t pt-4 mt-2">
          <Button
            type="button"
            variant="outline"
            onClick={onTest}
            disabled={isTesting}
            className="mr-auto"
          >
            <PlugZap className="h-4 w-4 mr-1.5" />
            {isTesting ? t('accounts.testing') : t('accounts.test')}
          </Button>
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

// ─── AccountsPage ─────────────────────────────────────────────────────────────

export function AccountsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // ── State ──
  const [dialogOpen, setDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [isEdit, setIsEdit] = useState(false)
  const [form, setForm] = useState<AccountForm>(DEFAULT_FORM)
  const [changePassword, setChangePassword] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: number; email: string } | null>(null)
  const [mailboxes, setMailboxes] = useState<Mailbox[]>([])
  const [mailboxesLoading, setMailboxesLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isTesting, setIsTesting] = useState(false)
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus | null>(null)

  // ── Queries ──
  const { data: accounts = [], isLoading: accountsLoading, refetch: refetchAccounts } = useQuery<Account[]>({
    queryKey: ['accounts'],
    queryFn: async () => {
      const res = await http.get('/accounts')
      return extractList<Account>(res.data)
    },
  })

  const { data: providers = [] } = useQuery<Provider[]>({
    queryKey: ['providers'],
    queryFn: async () => {
      const res = await http.get('/providers')
      return res.data?.providers ?? []
    },
  })

  const { data: proxies = [] } = useQuery<Proxy[]>({
    queryKey: ['proxies'],
    queryFn: async () => {
      const res = await http.get('/proxies')
      return extractList(res.data)
    },
  })

  // ── Toggle enabled mutation ──
  const toggleMutation = useMutation({
    mutationFn: async ({ account, enabled }: { account: Account; enabled: boolean }) => {
      await http.put(`/accounts/${account.ID}`, {
        email: account.email,
        display_name: account.display_name,
        login: account.login || account.email,
        password: '',
        provider: account.provider,
        imap_server: account.imap_server,
        imap_port: account.imap_port,
        ssl_mode: account.ssl_mode,
        proxy_id: account.proxy_id,
        use_idle: account.use_idle,
        poll_interval_day: account.poll_interval_day,
        poll_interval_night: account.poll_interval_night,
        timezone: account.timezone,
        enabled,
      })
    },
    onSuccess: () => {
      toast.success(t('accounts.update_success'))
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
    },
    onError: () => {
      toast.error(t('accounts.update_error'))
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
    },
  })

  // ── Delete mutation ──
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/accounts/${id}`)
    },
    onSuccess: () => {
      toast.success(t('accounts.delete_success'))
      setDeleteDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
    },
    onError: () => {
      toast.error(t('accounts.delete_error'))
    },
  })

  // ── Handlers ──
  const openCreate = () => {
    setIsEdit(false)
    setChangePassword(false)
    setConnectionStatus(null)
    setMailboxes([])
    const defaultProvider = providers[0]?.id ?? ''
    setForm({ ...DEFAULT_FORM, provider: defaultProvider })
    setDialogOpen(true)
  }

  const openEdit = async (acc: Account) => {
    setIsEdit(true)
    setChangePassword(false)
    setConnectionStatus(null)
    setMailboxes([])
    try {
      const res = await http.get(`/accounts/${acc.ID}`)
      const data = res.data
      setForm({
        id: data.ID,
        display_name: data.display_name ?? '',
        email: data.email ?? '',
        login: data.login ?? '',
        password: '',
        provider: data.provider ?? '',
        imap_server: data.imap_server ?? '',
        imap_port: data.imap_port ?? 993,
        ssl_mode: data.ssl_mode ?? (data.use_ssl ? 'ssl' : 'none'),
        proxy_id: data.proxy_id ?? null,
        use_idle: data.use_idle ?? true,
        poll_interval_day: data.poll_interval_day ?? 60,
        poll_interval_night: data.poll_interval_night ?? 300,
        timezone: data.timezone ?? 'UTC',
        enabled: typeof data.enabled === 'boolean' ? data.enabled : true,
      })
      setDialogOpen(true)
      fetchMailboxes(data.ID)
    } catch {
      toast.error(t('accounts.load_error'))
    }
  }

  const fetchMailboxes = async (accountId: number) => {
    setMailboxesLoading(true)
    try {
      const res = await http.get(`/accounts/${accountId}/mailboxes`)
      setMailboxes(extractList(res.data))
    } catch {
      toast.error(t('accounts.load_folders_error'))
    } finally {
      setMailboxesLoading(false)
    }
  }

  const handleSyncMailboxes = async () => {
    if (!form.id) return
    setMailboxesLoading(true)
    try {
      const res = await http.post(`/accounts/${form.id}/mailboxes/sync`)
      setMailboxes(extractList(res.data))
      toast.success(t('accounts.sync_folders_success'))
    } catch {
      toast.error(t('accounts.sync_folders_error'))
    } finally {
      setMailboxesLoading(false)
    }
  }

  const handleUpdateMailbox = async (mb: Mailbox) => {
    // Optimistically update local state
    setMailboxes((prev) => prev.map((m) => (m.ID === mb.ID ? mb : m)))
    try {
      await http.put(`/mailboxes/${mb.ID}`, {
        watch_mode: mb.watch_mode,
        type: mb.type,
      })
      toast.success(t('accounts.update_folder_success'))
    } catch {
      toast.error(t('accounts.update_folder_error'))
      // Reload mailboxes on error
      if (form.id) fetchMailboxes(form.id)
    }
  }

  const handleTest = async () => {
    if (!form.password && (!isEdit || changePassword)) {
      toast.warning(t('accounts.password_required'))
      return
    }
    if (isEdit && !changePassword) {
      toast.warning(t('accounts.password_change_hint'))
      return
    }
    setIsTesting(true)
    setConnectionStatus({ ok: false, message: t('accounts.testing'), details: [] })
    try {
      const res = await http.post('/accounts/test', { ...form })
      const data = res.data ?? {}
      const details: string[] = []
      if (data.security) details.push(t('accounts.test_security', { mode: data.security }))
      if (typeof data.latency_ms !== 'undefined')
        details.push(t('accounts.test_latency', { ms: data.latency_ms }))
      if (typeof data.supports_idle !== 'undefined')
        details.push(
          t('accounts.test_idle', { status: data.supports_idle ? t('common.yes') : t('common.no') })
        )
      if (Array.isArray(data.capabilities) && data.capabilities.length)
        details.push(t('accounts.test_capabilities', { caps: data.capabilities.join(', ') }))

      setConnectionStatus({
        ok: true,
        message: data.message || t('accounts.test_success'),
        details,
      })
      toast.success(data.message || t('accounts.test_success'))
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      const msg = error.response?.data?.error || 'Failed'
      setConnectionStatus({ ok: false, message: msg, details: [] })
      toast.error(msg)
    } finally {
      setIsTesting(false)
    }
  }

  const handleSave = async () => {
    if (!isEdit && !form.password) {
      toast.warning(t('accounts.password_required'))
      return
    }
    if (isEdit && changePassword && !form.password) {
      toast.warning(t('accounts.password_required'))
      return
    }

    setIsSaving(true)
    try {
      const payload = {
        ...form,
        login: form.login || form.email,
        password: isEdit && !changePassword ? '' : form.password,
      }

      if (isEdit && form.id) {
        await http.put(`/accounts/${form.id}`, payload)
        toast.success(t('accounts.update_success'))
      } else {
        await http.post('/accounts', payload)
        toast.success(t('accounts.create_success'))
      }
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      toast.error(error.response?.data?.error || t('accounts.create_error'))
    } finally {
      setIsSaving(false)
    }
  }

  const handleDeleteConfirm = (acc: Account) => {
    setDeleteTarget({ id: acc.ID, email: acc.email })
    setDeleteDialogOpen(true)
  }

  const handleDelete = () => {
    if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
  }

  // ─── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full">
      {/* Page header */}
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        <div>
          <h1 className="text-xl font-semibold">{t('menu.accounts')}</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            管理邮箱账户的连接与监听配置
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetchAccounts()}
            disabled={accountsLoading}
          >
            <RefreshCw className={`h-4 w-4 ${accountsLoading ? 'animate-spin' : ''}`} />
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus className="h-4 w-4 mr-1.5" />
            {t('accounts.add')}
          </Button>
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('accounts.display_name')}</TableHead>
              <TableHead>{t('accounts.email')}</TableHead>
              <TableHead>{t('accounts.provider')}</TableHead>
              <TableHead>{t('accounts.imap_server')}</TableHead>
              <TableHead className="w-24">{t('accounts.enabled')}</TableHead>
              <TableHead className="w-28">{t('accounts.status')}</TableHead>
              <TableHead>{t('accounts.last_sync')}</TableHead>
              <TableHead className="w-24 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {accountsLoading ? (
              <TableRow>
                <TableCell colSpan={8} className="text-center text-muted-foreground py-16">
                  加载中...
                </TableCell>
              </TableRow>
            ) : accounts.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="text-center text-muted-foreground py-16">
                  暂无账户，点击右上角新建账户
                </TableCell>
              </TableRow>
            ) : (
              accounts.map((acc) => (
                <TableRow key={acc.ID}>
                  <TableCell className="font-medium">
                    {acc.display_name || <span className="text-muted-foreground">-</span>}
                  </TableCell>
                  <TableCell>{acc.email}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{acc.provider}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{acc.imap_server}</TableCell>
                  <TableCell>
                    <Switch
                      checked={acc.enabled !== false}
                      onCheckedChange={(v) => toggleMutation.mutate({ account: acc, enabled: v })}
                    />
                  </TableCell>
                  <TableCell>
                    <Badge variant={getStatusVariant(acc.status)}>
                      {getStatusLabel(t, acc.status)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {acc.last_sync_at
                      ? new Date(acc.last_sync_at).toLocaleString()
                      : '-'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => openEdit(acc)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => handleDeleteConfirm(acc)}
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

      {/* Account Dialog */}
      <AccountDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        isEdit={isEdit}
        form={form}
        setForm={setForm}
        providers={providers}
        proxies={proxies}
        mailboxes={mailboxes}
        mailboxesLoading={mailboxesLoading}
        onSave={handleSave}
        onTest={handleTest}
        onSyncMailboxes={handleSyncMailboxes}
        onUpdateMailbox={handleUpdateMailbox}
        isSaving={isSaving}
        isTesting={isTesting}
        connectionStatus={connectionStatus}
        changePassword={changePassword}
        setChangePassword={setChangePassword}
      />

      {/* Delete Confirm Dialog */}
      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-destructive/10">
              <TriangleAlert className="h-6 w-6 text-destructive" />
            </div>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('common.delete_account_confirm')}
              {deleteTarget && (
                <span className="block mt-1 font-medium text-foreground">
                  {deleteTarget.email}
                </span>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDelete}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

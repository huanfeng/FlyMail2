import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, RefreshCw, Pencil, Trash2, Send, TriangleAlert } from 'lucide-react'

import http, { extractList } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { PageHeader } from '@/components/PageHeader'
import { Textarea } from '@/components/ui/textarea'
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

// ─── Types ────────────────────────────────────────────────────────────────────

interface Channel {
  ID: number
  id?: number
  name: string
  Name?: string
  type: string
  Type?: string
  status: string
  Status?: string
  min_priority: number
  MinPriority?: number
  quiet_mode: string
  QuietMode?: string
  quiet_enable: boolean
  QuietEnable?: boolean
  quiet_start: string
  QuietStart?: string
  quiet_end: string
  QuietEnd?: string
  subscribed_types: string
  SubscribedTypes?: string
  template: string
  Template?: string
  template_id: number | null
  TemplateID?: number | null
  config: string | Record<string, unknown>
  Config?: string | Record<string, unknown>
}

interface Template {
  id: number
  name: string
}

interface TelegramConfig {
  token: string
  chat_id: string
}

interface ChannelForm {
  id: number | null
  name: string
  type: string
  status: string
  min_priority: number
  quiet_mode: string
  quiet_enable: boolean
  quiet_start: string
  quiet_end: string
  subscribed_types: string[]
  template: string
  template_id: number | null
}

// ─── Constants ────────────────────────────────────────────────────────────────

const CHANNEL_TYPES = [{ label: 'Telegram', value: 'telegram' }]

const SUBSCRIPTION_TYPES = [
  'primary',
  'bill',
  'notification',
  'promotion',
  'social',
  'spam',
  'trash',
  'sent',
  'draft',
  'unknown',
]

const PRIORITIES = [
  { label: 'priority_low', value: 0 },
  { label: 'priority_normal', value: 10 },
  { label: 'priority_high', value: 20 },
  { label: 'priority_critical', value: 30 },
]

const QUIET_MODES = [
  { key: 'quiet_global', value: 'global' },
  { key: 'quiet_override', value: 'override' },
  { key: 'quiet_off', value: 'off' },
]

const DEFAULT_FORM: ChannelForm = {
  id: null,
  name: '',
  type: 'telegram',
  status: 'enabled',
  min_priority: 10,
  quiet_mode: 'global',
  quiet_enable: false,
  quiet_start: '',
  quiet_end: '',
  subscribed_types: [],
  template: '',
  template_id: null,
}

const DEFAULT_TELEGRAM: TelegramConfig = { token: '', chat_id: '' }

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getChannelId(ch: Channel): number {
  return ch.ID ?? ch.id ?? 0
}

function getChannelName(ch: Channel): string {
  return ch.name || ch.Name || ''
}

function getChannelType(ch: Channel): string {
  return ch.type || ch.Type || ''
}

function getChannelStatus(ch: Channel): string {
  return ch.status || ch.Status || 'disabled'
}

function parseTelegramConfig(config: string | Record<string, unknown>): TelegramConfig {
  try {
    if (typeof config === 'string') {
      return JSON.parse(config) as TelegramConfig
    }
    return config as unknown as TelegramConfig
  } catch {
    return { token: '', chat_id: '' }
  }
}

function parseSubscribedTypes(raw: string | undefined): string[] {
  if (!raw) return []
  try {
    return JSON.parse(raw) as string[]
  } catch {
    return []
  }
}

// ─── ChannelDialog ────────────────────────────────────────────────────────────

interface ChannelDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: ChannelForm
  setForm: React.Dispatch<React.SetStateAction<ChannelForm>>
  telegramConfig: TelegramConfig
  setTelegramConfig: React.Dispatch<React.SetStateAction<TelegramConfig>>
  templates: Template[]
  onSave: () => void
  onTest: (eventType: string) => void
  isSaving: boolean
  isTesting: boolean
}

function ChannelDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  telegramConfig,
  setTelegramConfig,
  templates,
  onSave,
  onTest,
  isSaving,
  isTesting,
}: ChannelDialogProps) {
  const { t } = useTranslation()
  const [testEventType, setTestEventType] = useState('system')

  const SectionLabel = ({ children }: { children: React.ReactNode }) => (
    <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3 mt-1">
      {children}
    </h4>
  )

  const FormRow = ({ label, children }: { label: string; children: React.ReactNode }) => (
    <div className="grid grid-cols-[140px_1fr] items-start gap-3">
      <Label className="text-sm text-muted-foreground text-right pt-2">{label}</Label>
      <div>{children}</div>
    </div>
  )

  const toggleSubscriptionType = (type: string) => {
    setForm((f) => {
      const current = f.subscribed_types
      return {
        ...f,
        subscribed_types: current.includes(type)
          ? current.filter((t) => t !== type)
          : [...current, type],
      }
    })
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>{isEdit ? t('channels.edit') : t('channels.add')}</DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto pr-1 space-y-6 py-2">
          {/* Basic Info */}
          <div className="space-y-3">
            <SectionLabel>{t('channels.section_basic')}</SectionLabel>
            <FormRow label={t('channels.status')}>
              <div className="flex items-center gap-3 pt-1">
                <Switch
                  checked={form.status === 'enabled'}
                  onCheckedChange={(v) =>
                    setForm((f) => ({ ...f, status: v ? 'enabled' : 'disabled' }))
                  }
                />
                <span className="text-sm text-muted-foreground">
                  {form.status === 'enabled'
                    ? t('channels.status_enabled')
                    : t('channels.status_disabled')}
                </span>
              </div>
            </FormRow>
            <FormRow label={t('channels.name')}>
              <Input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                autoFocus
              />
            </FormRow>
            <FormRow label={t('channels.type')}>
              <Select
                value={form.type}
                onValueChange={(v) => setForm((f) => ({ ...f, type: v }))}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHANNEL_TYPES.map((ct) => (
                    <SelectItem key={ct.value} value={ct.value}>
                      {ct.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormRow>
          </div>

          {/* Config */}
          <div className="space-y-3">
            <SectionLabel>{t('channels.section_config')}</SectionLabel>
            {form.type === 'telegram' && (
              <>
                <FormRow label="Bot Token">
                  <Input
                    value={telegramConfig.token}
                    onChange={(e) =>
                      setTelegramConfig((c) => ({ ...c, token: e.target.value }))
                    }
                  />
                </FormRow>
                <FormRow label="Chat ID">
                  <Input
                    value={telegramConfig.chat_id}
                    onChange={(e) =>
                      setTelegramConfig((c) => ({ ...c, chat_id: e.target.value }))
                    }
                  />
                </FormRow>
              </>
            )}
          </div>

          {/* Subscription Rules */}
          <div className="space-y-3">
            <SectionLabel>{t('channels.section_sub')}</SectionLabel>
            <FormRow label={t('channels.priority')}>
              <Select
                value={String(form.min_priority)}
                onValueChange={(v) =>
                  setForm((f) => ({ ...f, min_priority: parseInt(v) }))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PRIORITIES.map((p) => (
                    <SelectItem key={p.value} value={String(p.value)}>
                      {t(`common.${p.label}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormRow>

            <FormRow label={t('channels.quiet_mode')}>
              <div className="space-y-2">
                <Select
                  value={form.quiet_mode}
                  onValueChange={(v) => setForm((f) => ({ ...f, quiet_mode: v }))}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {QUIET_MODES.map((qm) => (
                      <SelectItem key={qm.value} value={qm.value}>
                        {t(`channels.${qm.key}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {form.quiet_mode === 'override' && (
                  <div className="space-y-2 pl-1">
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={form.quiet_enable}
                        onCheckedChange={(v) =>
                          setForm((f) => ({ ...f, quiet_enable: v }))
                        }
                      />
                      <span className="text-sm">{t('settings.quiet_enable')}</span>
                    </div>
                    <div className="flex gap-2">
                      <Input
                        type="time"
                        value={form.quiet_start}
                        disabled={!form.quiet_enable}
                        onChange={(e) =>
                          setForm((f) => ({ ...f, quiet_start: e.target.value }))
                        }
                      />
                      <Input
                        type="time"
                        value={form.quiet_end}
                        disabled={!form.quiet_enable}
                        onChange={(e) =>
                          setForm((f) => ({ ...f, quiet_end: e.target.value }))
                        }
                      />
                    </div>
                  </div>
                )}
              </div>
            </FormRow>

            <FormRow label={t('channels.subscription')}>
              <div className="space-y-2">
                <div className="flex flex-wrap gap-2">
                  {SUBSCRIPTION_TYPES.map((type) => (
                    <button
                      key={type}
                      type="button"
                      onClick={() => toggleSubscriptionType(type)}
                      className={`px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors ${
                        form.subscribed_types.includes(type)
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'bg-muted text-muted-foreground border-border hover:border-primary/50'
                      }`}
                    >
                      {t(`emails_view.type_${type}`)}
                    </button>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">{t('channels.subscription_hint')}</p>
              </div>
            </FormRow>

            <FormRow label={t('channels.template')}>
              <div className="space-y-2">
                <Select
                  value={form.template_id != null ? String(form.template_id) : '__custom__'}
                  onValueChange={(v) =>
                    setForm((f) => ({
                      ...f,
                      template_id: v === '__custom__' ? null : parseInt(v),
                    }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder={t('channels.select_template')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__custom__">{t('channels.template_hint_custom')}</SelectItem>
                    {templates.map((tmpl) => (
                      <SelectItem key={tmpl.id} value={String(tmpl.id)}>
                        {tmpl.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t('channels.template_hint')}</p>
                {form.template_id == null && (
                  <>
                    <Textarea
                      rows={3}
                      value={form.template}
                      onChange={(e) => setForm((f) => ({ ...f, template: e.target.value }))}
                      placeholder={t('channels.template_hint_custom')}
                    />
                    <p className="text-xs text-muted-foreground">
                      {t('channels.template_vars')}: {'{{.Subject}}'}, {'{{.From}}'},{' '}
                      {'{{.Link}}'}, {'{{.Type}}'}
                    </p>
                  </>
                )}
              </div>
            </FormRow>
          </div>
        </div>

        <DialogFooter className="border-t pt-4 mt-2">
          <div className="flex items-center gap-2 mr-auto">
            <Select value={testEventType} onValueChange={setTestEventType}>
              <SelectTrigger className="w-40 h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">{t('channels.test_type_system')}</SelectItem>
                <SelectItem value="email">{t('channels.test_type_email')}</SelectItem>
              </SelectContent>
            </Select>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onTest(testEventType)}
              disabled={isTesting}
            >
              <Send className="h-3.5 w-3.5 mr-1.5" />
              {t('channels.test_send')}
            </Button>
          </div>
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

// ─── ChannelsPage ─────────────────────────────────────────────────────────────

export function ChannelsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Channel | null>(null)
  const [isEdit, setIsEdit] = useState(false)
  const [form, setForm] = useState<ChannelForm>(DEFAULT_FORM)
  const [telegramConfig, setTelegramConfig] = useState<TelegramConfig>(DEFAULT_TELEGRAM)
  const [isSaving, setIsSaving] = useState(false)
  const [isTesting, setIsTesting] = useState(false)

  // ── Queries ──
  const { data: channels = [], isLoading, refetch } = useQuery<Channel[]>({
    queryKey: ['channels'],
    queryFn: async () => {
      const res = await http.get('/channels')
      return extractList(res.data)
    },
  })

  const { data: templates = [] } = useQuery<Template[]>({
    queryKey: ['templates'],
    queryFn: async () => {
      const res = await http.get('/templates')
      return extractList(res.data)
    },
  })

  // ── Delete mutation ──
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/channels/${id}`)
    },
    onSuccess: () => {
      toast.success(t('channels.delete_success'))
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: () => {
      toast.error(t('channels.delete_error'))
    },
  })

  // ── Handlers ──
  const openCreate = () => {
    setIsEdit(false)
    setForm(DEFAULT_FORM)
    setTelegramConfig(DEFAULT_TELEGRAM)
    setDialogOpen(true)
  }

  const openEdit = (ch: Channel) => {
    setIsEdit(true)
    const subs = parseSubscribedTypes(ch.subscribed_types || ch.SubscribedTypes)
    const templateId = ch.template_id ?? ch.TemplateID ?? null
    setForm({
      id: getChannelId(ch),
      name: getChannelName(ch),
      type: getChannelType(ch),
      status: getChannelStatus(ch),
      min_priority: ch.min_priority ?? ch.MinPriority ?? 10,
      quiet_mode: ch.quiet_mode || ch.QuietMode || 'global',
      quiet_enable: !!(ch.quiet_enable ?? ch.QuietEnable),
      quiet_start: ch.quiet_start || ch.QuietStart || '',
      quiet_end: ch.quiet_end || ch.QuietEnd || '',
      subscribed_types: subs,
      template: ch.template || ch.Template || '',
      template_id: templateId,
    })
    const rawConfig = ch.config || ch.Config
    if (rawConfig && getChannelType(ch) === 'telegram') {
      setTelegramConfig(parseTelegramConfig(rawConfig as string))
    } else {
      setTelegramConfig(DEFAULT_TELEGRAM)
    }
    setDialogOpen(true)
  }

  const handleSave = async () => {
    if (!form.name) {
      toast.warning(t('channels.name_required'))
      return
    }
    setIsSaving(true)
    try {
      const configStr =
        form.type === 'telegram' ? JSON.stringify(telegramConfig) : '{}'
      const payload = {
        ...form,
        config: configStr,
        subscribed_types: JSON.stringify(form.subscribed_types),
        template_id: form.template_id,
        template: form.template_id ? '' : form.template,
      }
      if (isEdit && form.id) {
        await http.put(`/channels/${form.id}`, payload)
      } else {
        await http.post('/channels', payload)
      }
      toast.success(t('channels.save_success'))
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    } catch {
      toast.error(t('channels.save_error'))
    } finally {
      setIsSaving(false)
    }
  }

  const handleTest = async (eventType: string) => {
    setIsTesting(true)
    try {
      const configStr =
        form.type === 'telegram' ? JSON.stringify(telegramConfig) : '{}'
      await http.post('/channels/test', {
        type: form.type,
        config: configStr,
        event_type: eventType,
      })
      toast.success(t('channels.test_success'))
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      const detail = error.response?.data?.error
        ? `${t('channels.test_error')}: ${error.response.data.error}`
        : t('channels.test_error')
      toast.error(detail)
    } finally {
      setIsTesting(false)
    }
  }

  const getStatusBadge = (status: string) => (
    <Badge variant={status === 'enabled' ? 'default' : 'secondary'}>
      {status === 'enabled' ? t('channels.status_enabled') : t('channels.status_disabled')}
    </Badge>
  )

  const getPriorityLabel = (val: number) => {
    const p = PRIORITIES.find((p) => p.value === val)
    return p ? t(`common.${p.label}`) : String(val)
  }

  // ─── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <PageHeader
        title={t('channels.title')}
        className="px-6 py-4 border-b"
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
              {t('channels.add')}
            </Button>
          </>
        }
      />

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('channels.name')}</TableHead>
              <TableHead>{t('channels.type')}</TableHead>
              <TableHead>{t('channels.priority')}</TableHead>
              <TableHead>{t('channels.quiet_mode')}</TableHead>
              <TableHead>{t('channels.status')}</TableHead>
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
            ) : channels.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground py-16">
                  {t('channels.no_channels')}
                </TableCell>
              </TableRow>
            ) : (
              channels.map((ch) => {
                const mode = (ch.quiet_mode || ch.QuietMode || 'global').toLowerCase()
                let quietSummary = t('channels.quiet_global')
                if (mode === 'off') quietSummary = t('channels.quiet_off')
                else if (mode === 'override') {
                  const enabled = !!(ch.quiet_enable ?? ch.QuietEnable)
                  if (!enabled) quietSummary = t('channels.quiet_off')
                  else {
                    const start = ch.quiet_start || ch.QuietStart || '--'
                    const end = ch.quiet_end || ch.QuietEnd || '--'
                    quietSummary = `${start} - ${end}`
                  }
                }

                return (
                  <TableRow key={getChannelId(ch)}>
                    <TableCell className="font-medium">{getChannelName(ch)}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{getChannelType(ch)}</Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {getPriorityLabel(ch.min_priority ?? ch.MinPriority ?? 10)}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {quietSummary}
                    </TableCell>
                    <TableCell>{getStatusBadge(getChannelStatus(ch))}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => openEdit(ch)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => setDeleteTarget(ch)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* Channel Dialog */}
      <ChannelDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        isEdit={isEdit}
        form={form}
        setForm={setForm}
        telegramConfig={telegramConfig}
        setTelegramConfig={setTelegramConfig}
        templates={templates}
        onSave={handleSave}
        onTest={handleTest}
        isSaving={isSaving}
        isTesting={isTesting}
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
              {deleteTarget && t('common.delete_confirm', { name: getChannelName(deleteTarget) })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteTarget && deleteMutation.mutate(getChannelId(deleteTarget))}
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

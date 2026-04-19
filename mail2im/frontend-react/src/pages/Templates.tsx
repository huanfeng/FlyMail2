import { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, RefreshCw, Pencil, Trash2, TriangleAlert, FileText, Loader2 } from 'lucide-react'

import http, { extractList } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
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

interface Template {
  id?: number
  name: string
  content: string
  channel_type: string
  is_default: boolean
  description: string
}

interface TemplateVar {
  name: string
  description: string
  example: string
}

interface TemplateForm {
  id: number | null
  name: string
  content: string
  channel_type: string
  is_default: boolean
  description: string
}

// ─── Constants ─────────────────────────────────────────────────────────────────

const CHANNEL_TYPES = [
  { label: 'All', value: 'all' },
  { label: 'Telegram', value: 'telegram' },
  { label: 'Discord', value: 'discord' },
]

const DEFAULT_FORM: TemplateForm = {
  id: null,
  name: '',
  content: '',
  channel_type: 'all',
  is_default: false,
  description: '',
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getChannelLabel(type: string): string {
  return CHANNEL_TYPES.find((c) => c.value === type)?.label ?? type
}

function getChannelVariant(type: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (type) {
    case 'telegram':
      return 'default'
    case 'discord':
      return 'secondary'
    default:
      return 'outline'
  }
}

// ─── TemplateEditorDialog ─────────────────────────────────────────────────────

interface TemplateEditorDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: TemplateForm
  setForm: React.Dispatch<React.SetStateAction<TemplateForm>>
  variables: TemplateVar[]
  defaultTemplates: Template[]
  onSave: () => void
  isSaving: boolean
}

function TemplateEditorDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  variables,
  defaultTemplates,
  onSave,
  isSaving,
}: TemplateEditorDialogProps) {
  const { t } = useTranslation()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [preview, setPreview] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const previewTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const fetchPreview = useCallback(async (content: string, channelType: string) => {
    if (!content) {
      setPreview('')
      return
    }
    setPreviewLoading(true)
    try {
      const res = await http.post('/templates/preview', { content, channel_type: channelType })
      setPreview(res.data?.preview ?? '')
    } catch {
      setPreview(t('templates.preview_error'))
    } finally {
      setPreviewLoading(false)
    }
  }, [t])

  const debouncedPreview = useCallback(
    (content: string, channelType: string) => {
      if (previewTimerRef.current) clearTimeout(previewTimerRef.current)
      previewTimerRef.current = setTimeout(() => {
        fetchPreview(content, channelType)
      }, 500)
    },
    [fetchPreview],
  )

  // Trigger preview whenever content or channel_type changes
  useEffect(() => {
    if (open) {
      debouncedPreview(form.content, form.channel_type)
    }
    return () => {
      if (previewTimerRef.current) clearTimeout(previewTimerRef.current)
    }
  }, [form.content, form.channel_type, open, debouncedPreview])

  // Reset preview when dialog opens/closes
  useEffect(() => {
    if (!open) setPreview('')
  }, [open])

  const insertVariable = (varName: string) => {
    const textarea = textareaRef.current
    const insertion = `{{.${varName}}}`
    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const value = form.content
      const newContent = value.substring(0, start) + insertion + value.substring(end)
      setForm((f) => ({ ...f, content: newContent }))
      setTimeout(() => {
        textarea.focus()
        textarea.selectionStart = textarea.selectionEnd = start + insertion.length
      }, 0)
    } else {
      setForm((f) => ({ ...f, content: f.content + insertion }))
    }
  }

  const loadDefaultTemplate = () => {
    const match =
      defaultTemplates.find((dt) => dt.channel_type === form.channel_type) ??
      defaultTemplates.find((dt) => dt.channel_type === 'all')
    if (match) {
      setForm((f) => ({ ...f, content: match.content }))
    } else {
      toast.warning('未找到对应默认模板')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('templates.edit') : t('templates.add')}
          </DialogTitle>
        </DialogHeader>

        {/* Top row: name, channel type, description */}
        <div className="flex gap-3 flex-wrap">
          <div className="flex-1 min-w-40 space-y-1">
            <Label className="text-sm text-muted-foreground">{t('templates.name')}</Label>
            <Input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="模板名称"
            />
          </div>
          <div className="w-40 space-y-1">
            <Label className="text-sm text-muted-foreground">{t('templates.channel_type')}</Label>
            <Select
              value={form.channel_type}
              onValueChange={(v) => setForm((f) => ({ ...f, channel_type: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHANNEL_TYPES.map((c) => (
                  <SelectItem key={c.value} value={c.value}>
                    {c.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex-1 min-w-40 space-y-1">
            <Label className="text-sm text-muted-foreground">{t('templates.description')}</Label>
            <Input
              value={form.description}
              onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
              placeholder={t('templates.description_hint')}
            />
          </div>
        </div>

        {/* Split pane: editor + preview */}
        <div className="flex gap-4 flex-1 overflow-hidden min-h-0">
          {/* Left: editor */}
          <div className="flex-1 flex flex-col overflow-hidden">
            <div className="flex items-center justify-between mb-2">
              <Label className="text-sm font-medium">{t('templates.content')}</Label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={loadDefaultTemplate}
                className="h-7 text-xs"
              >
                <FileText className="h-3.5 w-3.5 mr-1" />
                {t('templates.load_default')}
              </Button>
            </div>
            <Textarea
              ref={textareaRef}
              value={form.content}
              onChange={(e) => setForm((f) => ({ ...f, content: e.target.value }))}
              placeholder={t('templates.content_placeholder')}
              className="flex-1 resize-none font-mono text-sm min-h-0"
              style={{ height: '100%' }}
            />
            {/* Variable tags */}
            {variables.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-2">
                <span className="text-xs text-muted-foreground self-center mr-1">
                  {t('templates.variables')}:
                </span>
                {variables.map((v) => (
                  <button
                    key={v.name}
                    type="button"
                    title={`${v.description} — ${v.example}`}
                    onClick={() => insertVariable(v.name)}
                    className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-muted hover:bg-muted/80 text-muted-foreground hover:text-foreground transition-colors font-mono"
                  >
                    {v.name}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Right: preview */}
          <div className="flex-1 flex flex-col overflow-hidden">
            <div className="flex items-center justify-between mb-2">
              <Label className="text-sm font-medium">{t('templates.preview')}</Label>
              {previewLoading && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
            </div>
            <div className="flex-1 rounded-lg border bg-muted/30 p-3 overflow-auto min-h-0">
              <pre className="text-sm whitespace-pre-wrap break-words font-sans leading-relaxed m-0 text-foreground">
                {preview || (
                  <span className="text-muted-foreground">{t('templates.preview_empty')}</span>
                )}
              </pre>
            </div>
          </div>
        </div>

        <DialogFooter className="border-t pt-4 mt-2 shrink-0">
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onSave} disabled={isSaving}>
            {isSaving ? '保存中...' : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ─── TemplatesPage ────────────────────────────────────────────────────────────

export function TemplatesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [isEdit, setIsEdit] = useState(false)
  const [form, setForm] = useState<TemplateForm>(DEFAULT_FORM)
  const [isSaving, setIsSaving] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Template | null>(null)

  // ── Queries ──
  const { data: templates = [], isLoading, refetch } = useQuery<Template[]>({
    queryKey: ['templates'],
    queryFn: async () => {
      const res = await http.get('/templates')
      return extractList(res.data)
    },
  })

  const { data: variables = [] } = useQuery<TemplateVar[]>({
    queryKey: ['templates-variables'],
    queryFn: async () => {
      const res = await http.get('/templates/variables')
      return extractList(res.data)
    },
  })

  const { data: defaultTemplates = [] } = useQuery<Template[]>({
    queryKey: ['templates-defaults'],
    queryFn: async () => {
      const res = await http.get('/templates/defaults')
      return extractList(res.data)
    },
  })

  // ── Delete mutation ──
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/templates/${id}`)
    },
    onSuccess: () => {
      toast.success(t('templates.delete_success'))
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['templates'] })
    },
    onError: () => {
      toast.error(t('templates.delete_error'))
    },
  })

  // ── Handlers ──
  const openCreate = () => {
    setForm({ ...DEFAULT_FORM })
    setIsEdit(false)
    setDialogOpen(true)
  }

  const openEdit = (tmpl: Template) => {
    setForm({
      id: tmpl.id ?? null,
      name: tmpl.name,
      content: tmpl.content,
      channel_type: tmpl.channel_type,
      is_default: tmpl.is_default,
      description: tmpl.description,
    })
    setIsEdit(true)
    setDialogOpen(true)
  }

  const handleSave = async () => {
    if (!form.name || !form.content) {
      toast.warning('请填写模板名称和内容')
      return
    }
    setIsSaving(true)
    try {
      if (isEdit && form.id) {
        await http.put(`/templates/${form.id}`, form)
        toast.success(t('templates.update_success'))
      } else {
        await http.post('/templates', form)
        toast.success(t('templates.create_success'))
      }
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['templates'] })
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      toast.error(error.response?.data?.error || t('templates.save_error'))
    } finally {
      setIsSaving(false)
    }
  }

  // ─── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full">
      {/* Page header */}
      <PageHeader
        title={t('templates.title')}
        subtitle="管理消息推送模板，支持变量替换与实时预览"
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
              {t('templates.add')}
            </Button>
          </>
        }
      />

      {/* Table */}
      <div className="flex-1 overflow-auto px-6 py-4">
        <Table>
          <TableHeader>
            <TableRow>
                <TableHead>{t('templates.name')}</TableHead>
                <TableHead className="w-32">{t('templates.channel_type')}</TableHead>
                <TableHead>{t('templates.description')}</TableHead>
                <TableHead>{t('templates.content')}</TableHead>
                <TableHead className="w-24 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center py-10 text-muted-foreground text-sm">
                    加载中...
                  </TableCell>
                </TableRow>
              ) : templates.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center py-10 text-muted-foreground text-sm">
                    暂无模板，点击右上角新建模板
                  </TableCell>
                </TableRow>
              ) : (
                templates.map((tmpl, idx) => (
                  <TableRow key={tmpl.id ?? idx}>
                    <TableCell className="font-medium">{tmpl.name}</TableCell>
                    <TableCell>
                      <Badge variant={getChannelVariant(tmpl.channel_type)}>
                        {getChannelLabel(tmpl.channel_type)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground max-w-48 truncate">
                      {tmpl.description || '-'}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground max-w-64 truncate font-mono">
                      {tmpl.content}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => openEdit(tmpl)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => setDeleteTarget(tmpl)}
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

      {/* Template Editor Dialog */}
      <TemplateEditorDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        isEdit={isEdit}
        form={form}
        setForm={setForm}
        variables={variables}
        defaultTemplates={defaultTemplates}
        onSave={handleSave}
        isSaving={isSaving}
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
              确定要删除模板
              {deleteTarget && (
                <span className="font-medium text-foreground"> {deleteTarget.name}</span>
              )}
              吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteTarget?.id !== undefined && deleteMutation.mutate(deleteTarget.id)}
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

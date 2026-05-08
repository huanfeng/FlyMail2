import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, RefreshCw, Pencil, Trash2, TriangleAlert } from 'lucide-react'

import http, { extractList } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { PageHeader } from '@/components/PageHeader'
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

interface MailType {
  id: number
  key: string
  name: string
  priority: number
  color: string
  is_system: boolean
}

interface FolderRule {
  id: number
  name: string
  folder_pattern: string
  mail_type: string
  priority: number
}

interface ContentRule {
  id: number
  name: string
  field: string
  pattern: string
  target_type: string
  order: number
  enabled: boolean
}

interface MailTypeForm {
  id: number | null
  key: string
  name: string
  priority: number
  color: string
}

interface FolderRuleForm {
  id: number | null
  name: string
  folder_pattern: string
  mail_type: string
  priority: number
}

interface ContentRuleForm {
  id: number | null
  name: string
  field: string
  pattern: string
  target_type: string
  order: number
  enabled: boolean
}

// ─── Constants ─────────────────────────────────────────────────────────────────

const PRIORITIES = [
  { label: '低', value: 0 },
  { label: '普通', value: 10 },
  { label: '高', value: 20 },
  { label: '紧急', value: 30 },
]

const COLORS = [
  { label: '默认', value: 'default' },
  { label: '次要', value: 'secondary' },
  { label: '危险', value: 'destructive' },
  { label: '轮廓', value: 'outline' },
]

const DEFAULT_MAIL_TYPE_FORM: MailTypeForm = {
  id: null,
  key: '',
  name: '',
  priority: 10,
  color: 'default',
}

const DEFAULT_FOLDER_RULE_FORM: FolderRuleForm = {
  id: null,
  name: '',
  folder_pattern: '',
  mail_type: '',
  priority: 10,
}

const CONTENT_RULE_FIELDS = [
  { label: 'content_rule_field_from', value: 'from' },
  { label: 'content_rule_field_subject', value: 'subject' },
  { label: 'content_rule_field_to', value: 'to' },
  { label: 'content_rule_field_body', value: 'body' },
]

const DEFAULT_CONTENT_RULE_FORM: ContentRuleForm = {
  id: null,
  name: '',
  field: 'subject',
  pattern: '',
  target_type: '',
  order: 10,
  enabled: true,
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getPriorityLabel(value: number): string {
  return PRIORITIES.find((p) => p.value === value)?.label ?? String(value)
}

// ─── MailTypeDialog ───────────────────────────────────────────────────────────

interface MailTypeDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: MailTypeForm
  setForm: React.Dispatch<React.SetStateAction<MailTypeForm>>
  onSave: () => void
  isSaving: boolean
}

function MailTypeDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  onSave,
  isSaving,
}: MailTypeDialogProps) {
  const { t } = useTranslation()

  const FormRow = ({ label, children }: { label: string; children: React.ReactNode }) => (
    <div className="grid grid-cols-[120px_1fr] items-center gap-3">
      <Label className="text-sm text-muted-foreground text-right">{label}</Label>
      <div>{children}</div>
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('classification.edit_mail_type') : t('classification.add_mail_type')}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <FormRow label={t('classification.mail_type_key')}>
            <Input
              value={form.key}
              onChange={(e) => setForm((f) => ({ ...f, key: e.target.value }))}
              disabled={form.id !== null && isEdit}
              placeholder="例如: bill, notification"
            />
          </FormRow>
          <FormRow label={t('classification.mail_type_name')}>
            <Input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="显示名称"
            />
          </FormRow>
          <FormRow label={t('classification.mail_type_priority')}>
            <Select
              value={String(form.priority)}
              onValueChange={(v) => setForm((f) => ({ ...f, priority: parseInt(v) }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PRIORITIES.map((p) => (
                  <SelectItem key={p.value} value={String(p.value)}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>
          <FormRow label={t('classification.mail_type_color')}>
            <Select
              value={form.color}
              onValueChange={(v) => setForm((f) => ({ ...f, color: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {COLORS.map((c) => (
                  <SelectItem key={c.value} value={c.value}>
                    <Badge variant={c.value as 'default' | 'secondary' | 'destructive' | 'outline'}>
                      {c.label}
                    </Badge>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>
        </div>
        <DialogFooter>
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

// ─── FolderRuleDialog ─────────────────────────────────────────────────────────

interface FolderRuleDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: FolderRuleForm
  setForm: React.Dispatch<React.SetStateAction<FolderRuleForm>>
  mailTypes: MailType[]
  onSave: () => void
  isSaving: boolean
}

function FolderRuleDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  mailTypes,
  onSave,
  isSaving,
}: FolderRuleDialogProps) {
  const { t } = useTranslation()

  const FormRow = ({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) => (
    <div className="grid grid-cols-[120px_1fr] items-start gap-3">
      <Label className="text-sm text-muted-foreground text-right pt-2">{label}</Label>
      <div className="space-y-1">
        {children}
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </div>
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('classification.edit_folder_rule') : t('classification.add_folder_rule')}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <FormRow label={t('classification.rule_name')}>
            <Input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="规则名称"
            />
          </FormRow>
          <FormRow
            label={t('classification.rule_folder_pattern')}
            hint="支持通配符，如 INBOX.* 匹配所有子文件夹"
          >
            <Input
              value={form.folder_pattern}
              onChange={(e) => setForm((f) => ({ ...f, folder_pattern: e.target.value }))}
              placeholder="INBOX.*"
            />
          </FormRow>
          <FormRow label={t('classification.rule_mail_type')}>
            <Select
              value={form.mail_type}
              onValueChange={(v) => setForm((f) => ({ ...f, mail_type: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="选择邮件类型" />
              </SelectTrigger>
              <SelectContent>
                {mailTypes.map((mt) => (
                  <SelectItem key={mt.key} value={mt.key}>
                    {mt.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>
          <FormRow
            label={t('classification.rule_priority')}
            hint="数字越大优先级越高"
          >
            <Input
              type="number"
              value={form.priority}
              onChange={(e) => setForm((f) => ({ ...f, priority: parseInt(e.target.value) || 0 }))}
            />
          </FormRow>
        </div>
        <DialogFooter>
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

// ─── ContentRuleDialog ────────────────────────────────────────────────────────

interface ContentRuleDialogProps {
  open: boolean
  onClose: () => void
  isEdit: boolean
  form: ContentRuleForm
  setForm: React.Dispatch<React.SetStateAction<ContentRuleForm>>
  mailTypes: MailType[]
  onSave: () => void
  isSaving: boolean
}

function ContentRuleDialog({
  open,
  onClose,
  isEdit,
  form,
  setForm,
  mailTypes,
  onSave,
  isSaving,
}: ContentRuleDialogProps) {
  const { t } = useTranslation()

  const FormRow = ({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) => (
    <div className="grid grid-cols-[120px_1fr] items-start gap-3">
      <Label className="text-sm text-muted-foreground text-right pt-2">{label}</Label>
      <div className="space-y-1">
        {children}
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </div>
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('classification.edit_content_rule') : t('classification.add_content_rule')}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <FormRow label={t('classification.content_rule_name')}>
            <Input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="规则名称"
            />
          </FormRow>
          <FormRow label={t('classification.content_rule_field')}>
            <Select
              value={form.field}
              onValueChange={(v) => setForm((f) => ({ ...f, field: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CONTENT_RULE_FIELDS.map((f) => (
                  <SelectItem key={f.value} value={f.value}>
                    {t(`classification.${f.label}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>
          <FormRow
            label={t('classification.content_rule_pattern')}
            hint={t('classification.content_rule_pattern_hint')}
          >
            <Input
              value={form.pattern}
              onChange={(e) => setForm((f) => ({ ...f, pattern: e.target.value }))}
              placeholder="(?i)验证码|verification"
              className="font-mono text-sm"
            />
          </FormRow>
          <FormRow label={t('classification.content_rule_target_type')}>
            <Select
              value={form.target_type}
              onValueChange={(v) => setForm((f) => ({ ...f, target_type: v }))}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="选择邮件类型" />
              </SelectTrigger>
              <SelectContent>
                {mailTypes.map((mt) => (
                  <SelectItem key={mt.key} value={mt.key}>
                    {mt.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormRow>
          <FormRow
            label={t('classification.content_rule_order')}
            hint={t('classification.content_rule_order_hint')}
          >
            <Input
              type="number"
              value={form.order}
              onChange={(e) => setForm((f) => ({ ...f, order: parseInt(e.target.value) || 0 }))}
            />
          </FormRow>
          <FormRow label={t('classification.content_rule_enabled')}>
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
              className="h-4 w-4 mt-2"
            />
          </FormRow>
        </div>
        <DialogFooter>
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

// ─── ClassificationPage ───────────────────────────────────────────────────────

export function ClassificationPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<'mailTypes' | 'folderRules' | 'contentRules'>('mailTypes')

  // ── Mail Type state ──
  const [mailTypeDialogOpen, setMailTypeDialogOpen] = useState(false)
  const [isEditMailType, setIsEditMailType] = useState(false)
  const [mailTypeForm, setMailTypeForm] = useState<MailTypeForm>(DEFAULT_MAIL_TYPE_FORM)
  const [isSavingMailType, setIsSavingMailType] = useState(false)
  const [deleteMailTypeTarget, setDeleteMailTypeTarget] = useState<MailType | null>(null)

  // ── Folder Rule state ──
  const [folderRuleDialogOpen, setFolderRuleDialogOpen] = useState(false)
  const [isEditFolderRule, setIsEditFolderRule] = useState(false)
  const [folderRuleForm, setFolderRuleForm] = useState<FolderRuleForm>(DEFAULT_FOLDER_RULE_FORM)
  const [isSavingFolderRule, setIsSavingFolderRule] = useState(false)
  const [deleteFolderRuleTarget, setDeleteFolderRuleTarget] = useState<FolderRule | null>(null)

  // ── Content Rule state ──
  const [contentRuleDialogOpen, setContentRuleDialogOpen] = useState(false)
  const [isEditContentRule, setIsEditContentRule] = useState(false)
  const [contentRuleForm, setContentRuleForm] = useState<ContentRuleForm>(DEFAULT_CONTENT_RULE_FORM)
  const [isSavingContentRule, setIsSavingContentRule] = useState(false)
  const [deleteContentRuleTarget, setDeleteContentRuleTarget] = useState<ContentRule | null>(null)

  // ── Queries ──
  const {
    data: mailTypes = [],
    isLoading: mailTypesLoading,
    refetch: refetchMailTypes,
  } = useQuery<MailType[]>({
    queryKey: ['mailtypes'],
    queryFn: async () => {
      const res = await http.get('/mailtypes')
      return extractList(res.data)
    },
  })

  const {
    data: folderRules = [],
    isLoading: folderRulesLoading,
    refetch: refetchFolderRules,
  } = useQuery<FolderRule[]>({
    queryKey: ['rules'],
    queryFn: async () => {
      const res = await http.get('/rules')
      return extractList(res.data)
    },
  })

  const {
    data: contentRules = [],
    isLoading: contentRulesLoading,
    refetch: refetchContentRules,
  } = useQuery<ContentRule[]>({
    queryKey: ['content-rules'],
    queryFn: async () => {
      const res = await http.get('/content-rules')
      return extractList(res.data)
    },
  })

  // ── Delete mutations ──
  const deleteMailTypeMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/mailtypes/${id}`)
    },
    onSuccess: () => {
      toast.success(t('classification.mail_type_delete_success'))
      setDeleteMailTypeTarget(null)
      queryClient.invalidateQueries({ queryKey: ['mailtypes'] })
    },
    onError: () => {
      toast.error(t('classification.mail_type_delete_error'))
    },
  })

  const deleteFolderRuleMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/rules/${id}`)
    },
    onSuccess: () => {
      toast.success(t('classification.folder_rule_delete_success'))
      setDeleteFolderRuleTarget(null)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
    onError: () => {
      toast.error(t('classification.folder_rule_delete_error'))
    },
  })

  const deleteContentRuleMutation = useMutation({
    mutationFn: async (id: number) => {
      await http.delete(`/content-rules/${id}`)
    },
    onSuccess: () => {
      toast.success(t('classification.content_rule_delete_success'))
      setDeleteContentRuleTarget(null)
      queryClient.invalidateQueries({ queryKey: ['content-rules'] })
    },
    onError: () => {
      toast.error(t('classification.content_rule_delete_error'))
    },
  })

  // ── Mail Type handlers ──
  const openCreateMailType = () => {
    setMailTypeForm({ ...DEFAULT_MAIL_TYPE_FORM })
    setIsEditMailType(false)
    setMailTypeDialogOpen(true)
  }

  const openEditMailType = (mt: MailType) => {
    setMailTypeForm({
      id: mt.id,
      key: mt.key,
      name: mt.name,
      priority: mt.priority,
      color: mt.color || 'default',
    })
    setIsEditMailType(true)
    setMailTypeDialogOpen(true)
  }

  const handleSaveMailType = async () => {
    if (!mailTypeForm.key || !mailTypeForm.name) {
      toast.warning('请填写 Key 和名称')
      return
    }
    setIsSavingMailType(true)
    try {
      if (isEditMailType && mailTypeForm.id) {
        await http.put(`/mailtypes/${mailTypeForm.id}`, mailTypeForm)
        toast.success(t('classification.mail_type_update_success'))
      } else {
        await http.post('/mailtypes', mailTypeForm)
        toast.success(t('classification.mail_type_create_success'))
      }
      setMailTypeDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['mailtypes'] })
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      toast.error(error.response?.data?.error || t('classification.mail_type_save_error'))
    } finally {
      setIsSavingMailType(false)
    }
  }

  // ── Folder Rule handlers ──
  const openCreateFolderRule = () => {
    setFolderRuleForm({ ...DEFAULT_FOLDER_RULE_FORM })
    setIsEditFolderRule(false)
    setFolderRuleDialogOpen(true)
  }

  const openEditFolderRule = (rule: FolderRule) => {
    setFolderRuleForm({
      id: rule.id,
      name: rule.name,
      folder_pattern: rule.folder_pattern,
      mail_type: rule.mail_type,
      priority: rule.priority,
    })
    setIsEditFolderRule(true)
    setFolderRuleDialogOpen(true)
  }

  const handleSaveFolderRule = async () => {
    if (!folderRuleForm.folder_pattern || !folderRuleForm.mail_type) {
      toast.warning('请填写文件夹匹配规则和邮件类型')
      return
    }
    setIsSavingFolderRule(true)
    try {
      if (isEditFolderRule && folderRuleForm.id) {
        await http.put(`/rules/${folderRuleForm.id}`, folderRuleForm)
        toast.success(t('classification.folder_rule_update_success'))
      } else {
        await http.post('/rules', folderRuleForm)
        toast.success(t('classification.folder_rule_create_success'))
      }
      setFolderRuleDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      toast.error(error.response?.data?.error || t('classification.folder_rule_save_error'))
    } finally {
      setIsSavingFolderRule(false)
    }
  }

  // ── Content Rule handlers ──
  const openCreateContentRule = () => {
    setContentRuleForm({ ...DEFAULT_CONTENT_RULE_FORM })
    setIsEditContentRule(false)
    setContentRuleDialogOpen(true)
  }

  const openEditContentRule = (rule: ContentRule) => {
    setContentRuleForm({
      id: rule.id,
      name: rule.name,
      field: rule.field,
      pattern: rule.pattern,
      target_type: rule.target_type,
      order: rule.order,
      enabled: rule.enabled,
    })
    setIsEditContentRule(true)
    setContentRuleDialogOpen(true)
  }

  const handleSaveContentRule = async () => {
    if (!contentRuleForm.pattern || !contentRuleForm.target_type) {
      toast.warning('请填写匹配正则和目标类型')
      return
    }
    setIsSavingContentRule(true)
    try {
      if (isEditContentRule && contentRuleForm.id) {
        await http.put(`/content-rules/${contentRuleForm.id}`, contentRuleForm)
        toast.success(t('classification.content_rule_update_success'))
      } else {
        await http.post('/content-rules', contentRuleForm)
        toast.success(t('classification.content_rule_create_success'))
      }
      setContentRuleDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['content-rules'] })
    } catch (err: unknown) {
      const error = err as { response?: { data?: { error?: string } } }
      toast.error(error.response?.data?.error || t('classification.content_rule_save_error'))
    } finally {
      setIsSavingContentRule(false)
    }
  }

  const isLoading = mailTypesLoading || folderRulesLoading || contentRulesLoading

  // ─── Render ───────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full">
      {/* Page header */}
      <PageHeader
        title={t('classification.title')}
        subtitle="管理邮件分类规则与文件夹匹配策略"
        className="px-4 md:px-6 py-4 border-b"
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => { refetchMailTypes(); refetchFolderRules(); refetchContentRules() }}
            disabled={isLoading}
          >
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          </Button>
        }
      />

      {/* Tabs */}
      <div className="flex-1 overflow-hidden flex flex-col px-4 md:px-6 py-4">
        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'mailTypes' | 'folderRules' | 'contentRules')} className="flex-1 flex flex-col overflow-hidden">
          <div className="flex items-center justify-between mb-4">
            <TabsList>
              <TabsTrigger value="mailTypes">{t('classification.mail_types_tab')}</TabsTrigger>
              <TabsTrigger value="folderRules">{t('classification.folder_rules_tab')}</TabsTrigger>
              <TabsTrigger value="contentRules">{t('classification.content_rules_tab')}</TabsTrigger>
            </TabsList>
            {activeTab === 'mailTypes' ? (
              <Button size="sm" onClick={openCreateMailType}>
                <Plus className="h-4 w-4 mr-1.5" />
                {t('classification.add_mail_type')}
              </Button>
            ) : activeTab === 'folderRules' ? (
              <Button size="sm" onClick={openCreateFolderRule}>
                <Plus className="h-4 w-4 mr-1.5" />
                {t('classification.add_folder_rule')}
              </Button>
            ) : (
              <Button size="sm" onClick={openCreateContentRule}>
                <Plus className="h-4 w-4 mr-1.5" />
                {t('classification.add_content_rule')}
              </Button>
            )}
          </div>

          {/* ── Mail Types Tab ── */}
          <TabsContent value="mailTypes" className="flex-1 flex flex-col overflow-hidden mt-0">
            <div className="overflow-auto flex-1 rounded-lg border border-border overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/40 hover:bg-muted/40">
                    <TableHead>{t('classification.mail_type_key')}</TableHead>
                    <TableHead>{t('classification.mail_type_name')}</TableHead>
                    <TableHead>{t('classification.mail_type_priority')}</TableHead>
                    <TableHead>{t('classification.mail_type_color')}</TableHead>
                    <TableHead>{t('classification.mail_type_is_system')}</TableHead>
                    <TableHead className="w-24 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mailTypesLoading ? (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center py-10 text-muted-foreground text-sm">
                        加载中...
                      </TableCell>
                    </TableRow>
                  ) : mailTypes.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center py-10 text-muted-foreground text-sm">
                        暂无邮件类型
                      </TableCell>
                    </TableRow>
                  ) : (
                    mailTypes.map((mt) => (
                      <TableRow key={mt.id}>
                        <TableCell className="font-mono text-sm">{mt.key}</TableCell>
                        <TableCell className="font-medium">{mt.name}</TableCell>
                        <TableCell>
                          <Badge variant="secondary">{getPriorityLabel(mt.priority)}</Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant={mt.color as 'default' | 'secondary' | 'destructive' | 'outline' || 'default'}>
                            {mt.color || 'default'}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {mt.is_system ? (
                            <Badge variant="outline">系统</Badge>
                          ) : (
                            <span className="text-muted-foreground text-sm">-</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => openEditMailType(mt)}
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-destructive hover:text-destructive"
                              onClick={() => setDeleteMailTypeTarget(mt)}
                              disabled={mt.is_system}
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
          </TabsContent>

          {/* ── Folder Rules Tab ── */}
          <TabsContent value="folderRules" className="flex-1 flex flex-col overflow-hidden mt-0">
            <div className="overflow-auto flex-1 rounded-lg border border-border overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/40 hover:bg-muted/40">
                    <TableHead>{t('classification.rule_name')}</TableHead>
                    <TableHead>{t('classification.rule_folder_pattern')}</TableHead>
                    <TableHead>{t('classification.rule_mail_type')}</TableHead>
                    <TableHead>{t('classification.rule_priority')}</TableHead>
                    <TableHead className="w-24 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {folderRulesLoading ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center py-10 text-muted-foreground text-sm">
                        加载中...
                      </TableCell>
                    </TableRow>
                  ) : folderRules.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center py-10 text-muted-foreground text-sm">
                        暂无文件夹规则
                      </TableCell>
                    </TableRow>
                  ) : (
                    folderRules.map((rule) => {
                      const mailTypeName = mailTypes.find((mt) => mt.key === rule.mail_type)?.name ?? rule.mail_type
                      return (
                        <TableRow key={rule.id}>
                          <TableCell className="font-medium">{rule.name}</TableCell>
                          <TableCell className="font-mono text-sm">{rule.folder_pattern}</TableCell>
                          <TableCell>
                            <Badge variant="secondary">{mailTypeName}</Badge>
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">{rule.priority}</TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditFolderRule(rule)}
                              >
                                <Pencil className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => setDeleteFolderRuleTarget(rule)}
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
          </TabsContent>

          {/* ── Content Rules Tab ── */}
          <TabsContent value="contentRules" className="flex-1 flex flex-col overflow-hidden mt-0">
            <div className="overflow-auto flex-1 rounded-lg border border-border overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/40 hover:bg-muted/40">
                    <TableHead>{t('classification.content_rule_name')}</TableHead>
                    <TableHead>{t('classification.content_rule_field')}</TableHead>
                    <TableHead>{t('classification.content_rule_pattern')}</TableHead>
                    <TableHead>{t('classification.content_rule_target_type')}</TableHead>
                    <TableHead>{t('classification.content_rule_order')}</TableHead>
                    <TableHead>{t('classification.content_rule_enabled')}</TableHead>
                    <TableHead className="w-24 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {contentRulesLoading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-center py-10 text-muted-foreground text-sm">
                        加载中...
                      </TableCell>
                    </TableRow>
                  ) : contentRules.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-center py-10 text-muted-foreground text-sm">
                        暂无内容规则
                      </TableCell>
                    </TableRow>
                  ) : (
                    contentRules.map((rule) => {
                      const mailTypeName = mailTypes.find((mt) => mt.key === rule.target_type)?.name ?? rule.target_type
                      const fieldLabel = CONTENT_RULE_FIELDS.find((f) => f.value === rule.field)
                      return (
                        <TableRow key={rule.id} className={!rule.enabled ? 'opacity-50' : ''}>
                          <TableCell className="font-medium">{rule.name}</TableCell>
                          <TableCell>
                            <Badge variant="outline">
                              {fieldLabel ? t(`classification.${fieldLabel.label}`) : rule.field}
                            </Badge>
                          </TableCell>
                          <TableCell className="font-mono text-sm max-w-[200px] truncate">{rule.pattern}</TableCell>
                          <TableCell>
                            <Badge variant="secondary">{mailTypeName}</Badge>
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">{rule.order}</TableCell>
                          <TableCell>
                            {rule.enabled ? (
                              <Badge variant="default" className="text-xs">启用</Badge>
                            ) : (
                              <Badge variant="secondary" className="text-xs">禁用</Badge>
                            )}
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditContentRule(rule)}
                              >
                                <Pencil className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => setDeleteContentRuleTarget(rule)}
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
          </TabsContent>
        </Tabs>
      </div>

      {/* Mail Type Dialog */}
      <MailTypeDialog
        open={mailTypeDialogOpen}
        onClose={() => setMailTypeDialogOpen(false)}
        isEdit={isEditMailType}
        form={mailTypeForm}
        setForm={setMailTypeForm}
        onSave={handleSaveMailType}
        isSaving={isSavingMailType}
      />

      {/* Folder Rule Dialog */}
      <FolderRuleDialog
        open={folderRuleDialogOpen}
        onClose={() => setFolderRuleDialogOpen(false)}
        isEdit={isEditFolderRule}
        form={folderRuleForm}
        setForm={setFolderRuleForm}
        mailTypes={mailTypes}
        onSave={handleSaveFolderRule}
        isSaving={isSavingFolderRule}
      />

      {/* Delete Mail Type Confirm */}
      <AlertDialog
        open={deleteMailTypeTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteMailTypeTarget(null) }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-destructive/10">
              <TriangleAlert className="h-6 w-6 text-destructive" />
            </div>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除邮件类型
              {deleteMailTypeTarget && (
                <span className="font-medium text-foreground"> {deleteMailTypeTarget.name}</span>
              )}
              吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteMailTypeTarget && deleteMailTypeMutation.mutate(deleteMailTypeTarget.id)}
              disabled={deleteMailTypeMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Folder Rule Confirm */}
      <AlertDialog
        open={deleteFolderRuleTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteFolderRuleTarget(null) }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-destructive/10">
              <TriangleAlert className="h-6 w-6 text-destructive" />
            </div>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除文件夹规则
              {deleteFolderRuleTarget && (
                <span className="font-medium text-foreground"> {deleteFolderRuleTarget.name}</span>
              )}
              吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteFolderRuleTarget && deleteFolderRuleMutation.mutate(deleteFolderRuleTarget.id)}
              disabled={deleteFolderRuleMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Content Rule Dialog */}
      <ContentRuleDialog
        open={contentRuleDialogOpen}
        onClose={() => setContentRuleDialogOpen(false)}
        isEdit={isEditContentRule}
        form={contentRuleForm}
        setForm={setContentRuleForm}
        mailTypes={mailTypes}
        onSave={handleSaveContentRule}
        isSaving={isSavingContentRule}
      />

      {/* Delete Content Rule Confirm */}
      <AlertDialog
        open={deleteContentRuleTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteContentRuleTarget(null) }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-destructive/10">
              <TriangleAlert className="h-6 w-6 text-destructive" />
            </div>
            <AlertDialogTitle>{t('common.confirm')}</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除内容规则
              {deleteContentRuleTarget && (
                <span className="font-medium text-foreground"> {deleteContentRuleTarget.name}</span>
              )}
              吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deleteContentRuleTarget && deleteContentRuleMutation.mutate(deleteContentRuleTarget.id)}
              disabled={deleteContentRuleMutation.isPending}
            >
              {t('common.yes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

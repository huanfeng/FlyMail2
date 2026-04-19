import { useState, useEffect, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  RefreshCw,
  Save,
  Download,
  Upload,
  Sun,
  Moon,
  Database,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { ThemePicker } from '@/components/ThemePicker'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
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
import i18n from '@/i18n'
import api from '@/services/api'

// ─── Types ───────────────────────────────────────────────────────────────────

interface ConfigData {
  timezone?: string
  server_time?: string
  timezones?: string[]
  quiet_enabled?: boolean
  quiet_start?: string
  quiet_end?: string
  night_enabled?: boolean
  night_start?: string
  night_end?: string
}

interface ExportSections {
  accounts: boolean
  proxies: boolean
  channels: boolean
  settings: boolean
}

interface ImportConflicts {
  accounts?: string[]
  proxies?: string[]
  channels?: string[]
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function getTimezoneOffsetMinutes(tz: string): number {
  try {
    const now = new Date()
    const utc = new Date(now.toLocaleString('en-US', { timeZone: 'UTC' }))
    const zoned = new Date(now.toLocaleString('en-US', { timeZone: tz }))
    return Math.round((zoned.getTime() - utc.getTime()) / 60000)
  } catch {
    return 0
  }
}

function formatOffsetLabel(minutes: number): string {
  const sign = minutes >= 0 ? '+' : '-'
  const abs = Math.abs(minutes)
  const hours = String(Math.floor(abs / 60)).padStart(2, '0')
  const mins = String(abs % 60).padStart(2, '0')
  return `${sign}${hours}:${mins}`
}

function buildTimezoneOptions(
  rawList: string[],
  lang: string
): Array<{ value: string; label: string }> {
  const unique = Array.from(new Set(rawList)).filter(Boolean)
  const enriched = unique.map((tz) => {
    const offsetMinutes = getTimezoneOffsetMinutes(tz)
    const offset = formatOffsetLabel(offsetMinutes)
    let name = tz
    try {
      const formatter = new Intl.DateTimeFormat(lang, {
        timeZone: tz,
        timeZoneName: 'longGeneric',
      } as Intl.DateTimeFormatOptions)
      const part = formatter.formatToParts(new Date()).find((p) => p.type === 'timeZoneName')
      if (part?.value && part.value !== tz) name = part.value
    } catch {
      /* ignore */
    }
    const label = `(UTC${offset}) ${name}`
    return { tz, offsetMinutes, label }
  })

  enriched.sort((a, b) =>
    a.offsetMinutes !== b.offsetMinutes
      ? a.offsetMinutes - b.offsetMinutes
      : a.label.localeCompare(b.label)
  )

  const labelCount = new Map<string, number>()
  enriched.forEach(({ label }) => labelCount.set(label, (labelCount.get(label) ?? 0) + 1))

  return enriched.map(({ tz, label }) => ({
    value: tz,
    label: (labelCount.get(label) ?? 0) > 1 ? `${label} - ${tz}` : label,
  }))
}

// ─── Section wrapper ──────────────────────────────────────────────────────────

function Section({
  icon: Icon,
  title,
  description,
  action,
  children,
}: {
  icon: React.ElementType
  title: string
  description: string
  action?: React.ReactNode
  children?: React.ReactNode
}) {
  return (
    <div className="rounded-xl border bg-card p-6 flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="h-9 w-9 rounded-lg bg-primary/8 flex items-center justify-center shrink-0 mt-0.5">
            <Icon className="h-4 w-4 text-primary" />
          </div>
          <div>
            <div className="font-semibold text-base">{title}</div>
            <div className="text-sm text-muted-foreground mt-0.5">{description}</div>
          </div>
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
      {children && <div className="flex flex-col gap-4 pl-12">{children}</div>}
    </div>
  )
}

function SettingRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[200px_1fr] gap-4 items-start">
      <div className="font-medium text-sm pt-1.5">{label}</div>
      <div className="flex flex-col gap-1.5">{children}</div>
    </div>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export function SettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // ── local form state ──
  const [lang, setLang] = useState(i18n.language ?? 'zh')
  const [timezone, setTimezone] = useState('UTC')
  const [quietEnabled, setQuietEnabled] = useState(false)
  const [quietStart, setQuietStart] = useState('')
  const [quietEnd, setQuietEnd] = useState('')
  const [nightEnabled, setNightEnabled] = useState(false)
  const [nightStart, setNightStart] = useState('')
  const [nightEnd, setNightEnd] = useState('')
  const [tzOptions, setTzOptions] = useState<Array<{ value: string; label: string }>>([])
  const [serverTime, setServerTime] = useState('')

  // ── export dialog state ──
  const [exportOpen, setExportOpen] = useState(false)
  const [exportPassword, setExportPassword] = useState('')
  const [exportSections, setExportSections] = useState<ExportSections>({
    accounts: true,
    proxies: true,
    channels: true,
    settings: true,
  })
  const [exportPasswordError, setExportPasswordError] = useState('')
  const [exporting, setExporting] = useState(false)

  // ── import dialog state ──
  const [importOpen, setImportOpen] = useState(false)
  const [importFileName, setImportFileName] = useState('')
  const [importSections, setImportSections] = useState<ExportSections>({
    accounts: false,
    proxies: false,
    channels: false,
    settings: false,
  })
  const [availableImport, setAvailableImport] = useState<ExportSections>({
    accounts: false,
    proxies: false,
    channels: false,
    settings: false,
  })
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const [parsedImportData, setParsedImportData] = useState<any>(null)
  const [importing, setImporting] = useState(false)
  const [conflictOpen, setConflictOpen] = useState(false)
  const [importConflicts, setImportConflicts] = useState<ImportConflicts>({})
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const [lastImportPayload, setLastImportPayload] = useState<any>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // ── config query ──
  const { isFetching, refetch } = useQuery({
    queryKey: ['config'],
    queryFn: async () => {
      const res = await api.get<ConfigData>('/config')
      return res.data
    },
    onSuccess(data: ConfigData) {
      setTimezone(data.timezone ?? 'UTC')
      setServerTime(data.server_time ?? '')
      setQuietEnabled(!!data.quiet_enabled)
      setQuietStart(data.quiet_start ?? '')
      setQuietEnd(data.quiet_end ?? '')
      setNightEnabled(!!data.night_enabled)
      setNightStart(data.night_start ?? '')
      setNightEnd(data.night_end ?? '')
      const raw = data.timezones?.length ? data.timezones : [data.timezone ?? 'UTC', 'UTC']
      setTzOptions(buildTimezoneOptions(raw, lang))
    },
  } as Parameters<typeof useQuery>[0])

  // rebuild tz labels when lang changes
  useEffect(() => {
    if (tzOptions.length > 0) {
      const rawValues = tzOptions.map((o) => o.value)
      setTzOptions(buildTimezoneOptions(rawValues, lang))
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lang])

  // ── save mutation ──
  const saveMutation = useMutation({
    mutationFn: async () => {
      await api.post('/config', {
        timezone,
        quiet_enabled: quietEnabled,
        quiet_start: quietStart,
        quiet_end: quietEnd,
        night_enabled: nightEnabled,
        night_start: nightStart,
        night_end: nightEnd,
      })
      // apply language
      await i18n.changeLanguage(lang)
      localStorage.setItem('mail2im_ui_language', lang)
    },
    onSuccess: () => {
      toast.success(t('settings.saved_success'))
      queryClient.invalidateQueries({ queryKey: ['config'] })
      refetch()
    },
    onError: () => {
      toast.error(t('settings.saved_error'))
    },
  })

  // ── export ──
  const handleExport = async () => {
    const sections = (Object.entries(exportSections) as [string, boolean][])
      .filter(([, v]) => v)
      .map(([k]) => k)

    if (sections.length === 0) {
      toast.warning(t('settings.export_select_warning'))
      return
    }
    if (sections.includes('accounts') && !exportPassword) {
      setExportPasswordError(t('settings.export_password_required'))
      return
    }
    setExportPasswordError('')
    setExporting(true)
    try {
      const res = await api.post('/config/export', {
        sections,
        password: sections.includes('accounts') ? exportPassword : '',
      })
      const blob = new Blob([JSON.stringify(res.data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `mail2im_export_${new Date().toISOString()}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      toast.success(t('settings.export_success'))
      setExportOpen(false)
      setExportPassword('')
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: string } } }
      if (e.response?.data?.error === 'invalid_password') {
        setExportPasswordError(t('settings.export_password_invalid'))
      } else {
        toast.error(t('settings.export_error'))
      }
    } finally {
      setExporting(false)
    }
  }

  // ── import file parse ──
  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    let json: unknown
    try {
      const text = await file.text()
      json = JSON.parse(text)
    } catch {
      toast.error(t('settings.import_parse_error'))
      e.target.value = ''
      return
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const data = json as any
    setParsedImportData(data)
    setImportFileName(file.name)

    const avail = {
      accounts: Array.isArray(data.accounts) && data.accounts.length > 0,
      proxies: Array.isArray(data.proxies) && data.proxies.length > 0,
      channels: Array.isArray(data.channels) && data.channels.length > 0,
      settings: !!data.system_settings && Object.keys(data.system_settings).length > 0,
    }
    setAvailableImport(avail)
    setImportSections({ ...avail })

    if (!avail.accounts && !avail.proxies && !avail.channels && !avail.settings) {
      toast.warning(t('settings.import_empty_warning'))
      setParsedImportData(null)
      setImportFileName('')
    }
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const buildImportPayload = (): any | null => {
    if (!parsedImportData) return null
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const payload: any = {}
    if (importSections.accounts && availableImport.accounts) payload.accounts = parsedImportData.accounts
    if (importSections.proxies && availableImport.proxies) payload.proxies = parsedImportData.proxies
    if (importSections.channels && availableImport.channels) payload.channels = parsedImportData.channels
    if (importSections.settings && availableImport.settings) payload.system_settings = parsedImportData.system_settings
    return payload
  }

  const performImport = async (overwrite = false, payloadOverride?: unknown) => {
    const payload = payloadOverride ?? buildImportPayload()
    if (!payload || Object.keys(payload as object).length === 0) {
      toast.warning(t('settings.import_select_warning'))
      return
    }
    setImporting(true)
    try {
      await api.post('/config/import', { ...(payload as object), overwrite })
      toast.success(t('settings.import_success'))
      setImportOpen(false)
      setConflictOpen(false)
      setImportFileName('')
      setParsedImportData(null)
      refetch()
    } catch (err: unknown) {
      const e = err as { response?: { status?: number; data?: { conflicts?: ImportConflicts } } }
      if (e.response?.status === 409) {
        setLastImportPayload(buildImportPayload())
        setImportConflicts(e.response.data?.conflicts ?? {})
        setConflictOpen(true)
      } else {
        toast.error(t('settings.import_error'))
      }
    } finally {
      setImporting(false)
    }
  }

  const openExportDialog = () => {
    setExportSections({ accounts: true, proxies: true, channels: true, settings: true })
    setExportPassword('')
    setExportPasswordError('')
    setExportOpen(true)
  }

  const openImportDialog = () => {
    setImportSections({ accounts: false, proxies: false, channels: false, settings: false })
    setAvailableImport({ accounts: false, proxies: false, channels: false, settings: false })
    setParsedImportData(null)
    setImportFileName('')
    setConflictOpen(false)
    setLastImportPayload(null)
    setImportConflicts({})
    setImportOpen(true)
  }

  const isSaving = saveMutation.isPending

  return (
    <div className="flex flex-col gap-6 max-w-3xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t('settings.title')}</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label={t('common.refresh')}
          >
            <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
          </Button>
          <Button
            variant="outline"
            onClick={() => saveMutation.reset()}
            disabled={isSaving}
          >
            {t('common.cancel')}
          </Button>
          <Button onClick={() => saveMutation.mutate()} disabled={isSaving}>
            <Save className="h-4 w-4 mr-1.5" />
            {t('settings.save')}
          </Button>
        </div>
      </div>

      {/* Appearance */}
      <Section
        icon={Sun}
        title={t('settings.appearance')}
        description={t('settings.appearance_desc')}
      >
        <SettingRow label={t('settings.language')}>
          <Select value={lang} onValueChange={setLang}>
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="zh">中文</SelectItem>
              <SelectItem value="en">English</SelectItem>
            </SelectContent>
          </Select>
        </SettingRow>
        <SettingRow label={t('settings.theme') || '主题'}>
          <div style={{
            border: '1px solid var(--rule, rgba(0,0,0,0.08))',
            borderRadius: 12,
            overflow: 'hidden',
            background: 'var(--surface, #fff)',
            maxWidth: 320,
          }}>
            <ThemePicker />
          </div>
        </SettingRow>
      </Section>

      {/* Delivery / Scheduling */}
      <Section
        icon={Moon}
        title={t('settings.delivery_block')}
        description={t('settings.delivery_desc')}
      >
        {/* Timezone */}
        <SettingRow label={t('settings.timezone')}>
          <Select value={timezone} onValueChange={setTimezone}>
            <SelectTrigger className="w-full max-w-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="max-h-72">
              {tzOptions.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {serverTime && (
            <span className="text-xs text-muted-foreground">
              {t('settings.server_time', { time: serverTime })}
            </span>
          )}
        </SettingRow>

        {/* Quiet hours */}
        <SettingRow label={t('settings.quiet')}>
          <div className="flex items-center gap-2">
            <Switch
              checked={quietEnabled}
              onCheckedChange={setQuietEnabled}
              id="quiet-enabled"
            />
            <Label htmlFor="quiet-enabled" className="text-sm cursor-pointer">
              {t('settings.quiet_enable')}
            </Label>
          </div>
          <div className="flex gap-3">
            <div className="flex flex-col gap-1 flex-1">
              <Label className="text-xs text-muted-foreground">{t('settings.quiet_start')}</Label>
              <Input
                type="time"
                value={quietStart}
                onChange={(e) => setQuietStart(e.target.value)}
                disabled={!quietEnabled}
                className="w-36"
              />
            </div>
            <div className="flex flex-col gap-1 flex-1">
              <Label className="text-xs text-muted-foreground">{t('settings.quiet_end')}</Label>
              <Input
                type="time"
                value={quietEnd}
                onChange={(e) => setQuietEnd(e.target.value)}
                disabled={!quietEnabled}
                className="w-36"
              />
            </div>
          </div>
          <span className="text-xs text-muted-foreground">{t('settings.quiet_desc')}</span>
        </SettingRow>

        {/* Night window */}
        <SettingRow label={t('settings.night_window')}>
          <div className="flex items-center gap-2">
            <Switch
              checked={nightEnabled}
              onCheckedChange={setNightEnabled}
              id="night-enabled"
            />
            <Label htmlFor="night-enabled" className="text-sm cursor-pointer">
              {t('settings.night_enable')}
            </Label>
          </div>
          <div className="flex gap-3">
            <div className="flex flex-col gap-1 flex-1">
              <Label className="text-xs text-muted-foreground">{t('settings.night_start')}</Label>
              <Input
                type="time"
                value={nightStart}
                onChange={(e) => setNightStart(e.target.value)}
                disabled={!nightEnabled}
                className="w-36"
              />
            </div>
            <div className="flex flex-col gap-1 flex-1">
              <Label className="text-xs text-muted-foreground">{t('settings.night_end')}</Label>
              <Input
                type="time"
                value={nightEnd}
                onChange={(e) => setNightEnd(e.target.value)}
                disabled={!nightEnabled}
                className="w-36"
              />
            </div>
          </div>
          <span className="text-xs text-muted-foreground">{t('settings.night_desc')}</span>
        </SettingRow>
      </Section>

      {/* Data Transfer */}
      <Section
        icon={Database}
        title={t('settings.data_transfer')}
        description={t('settings.data_transfer_desc')}
        action={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={openExportDialog}>
              <Download className="h-4 w-4 mr-1.5" />
              {t('settings.export_btn')}
            </Button>
            <Button size="sm" onClick={openImportDialog}>
              <Upload className="h-4 w-4 mr-1.5" />
              {t('settings.import_btn')}
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted-foreground -mt-2">{t('settings.data_transfer_hint')}</p>
      </Section>

      {/* ── Export Dialog ── */}
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('settings.export_title')}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-5 py-2">
            {/* Section checkboxes */}
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-medium">{t('settings.export_title')}</Label>
              <div className="grid grid-cols-2 gap-2">
                {(['accounts', 'proxies', 'channels', 'settings'] as const).map((key) => (
                  <label key={key} className="flex items-center gap-2 cursor-pointer">
                    <Checkbox
                      checked={exportSections[key]}
                      onCheckedChange={(v) =>
                        setExportSections((prev) => ({ ...prev, [key]: !!v }))
                      }
                    />
                    <span className="text-sm">
                      {key === 'accounts'
                        ? t('settings.export_accounts')
                        : key === 'proxies'
                          ? t('settings.export_proxies')
                          : key === 'channels'
                            ? t('settings.export_channels')
                            : t('settings.export_system')}
                    </span>
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">{t('settings.export_hint')}</p>
            </div>

            {/* Password */}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="export-pwd" className="text-sm font-medium">
                {t('settings.export_password')}
              </Label>
              <Input
                id="export-pwd"
                type="password"
                value={exportPassword}
                onChange={(e) => {
                  setExportPassword(e.target.value)
                  setExportPasswordError('')
                }}
                disabled={!exportSections.accounts}
                autoComplete="current-password"
                className={exportPasswordError ? 'border-destructive' : ''}
              />
              {exportPasswordError && (
                <p className="text-xs text-destructive">{exportPasswordError}</p>
              )}
              <p className="text-xs text-muted-foreground">{t('settings.export_password_hint')}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setExportOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleExport} disabled={exporting}>
              <Download className="h-4 w-4 mr-1.5" />
              {t('settings.export_btn')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Import Dialog ── */}
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('settings.import_title')}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-5 py-2">
            {/* File picker */}
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-medium">{t('settings.import_select_file')}</Label>
              <div className="flex items-center gap-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="application/json"
                  className="hidden"
                  onChange={handleFileChange}
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Upload className="h-4 w-4 mr-1.5" />
                  {t('settings.import_select_file')}
                </Button>
                <span className="text-sm text-muted-foreground truncate max-w-48">
                  {importFileName || t('settings.import_no_file')}
                </span>
              </div>
            </div>

            {/* Sections */}
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-medium">{t('settings.import_sections')}</Label>
              <div className="grid grid-cols-2 gap-2">
                {(['accounts', 'proxies', 'channels', 'settings'] as const).map((key) => (
                  <label
                    key={key}
                    className={`flex items-center gap-2 ${!availableImport[key] ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'}`}
                  >
                    <Checkbox
                      checked={importSections[key]}
                      disabled={!availableImport[key]}
                      onCheckedChange={(v) =>
                        setImportSections((prev) => ({ ...prev, [key]: !!v }))
                      }
                    />
                    <span className="text-sm">
                      {key === 'accounts'
                        ? t('settings.export_accounts')
                        : key === 'proxies'
                          ? t('settings.export_proxies')
                          : key === 'channels'
                            ? t('settings.export_channels')
                            : t('settings.export_system')}
                    </span>
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">{t('settings.import_hint')}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setImportOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              onClick={() => performImport(false)}
              disabled={!parsedImportData || importing}
            >
              <Upload className="h-4 w-4 mr-1.5" />
              {t('settings.import_start')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Conflict (overwrite) dialog ── */}
      <AlertDialog open={conflictOpen} onOpenChange={setConflictOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('settings.import_conflict_title')}</AlertDialogTitle>
            <AlertDialogDescription>{t('settings.import_conflict_desc')}</AlertDialogDescription>
          </AlertDialogHeader>
          {/* Conflict detail */}
          <div className="flex flex-col gap-2 text-sm max-h-48 overflow-y-auto">
            {(importConflicts.accounts ?? []).length > 0 && (
              <div>
                <p className="font-medium mb-1">{t('menu.accounts')}</p>
                <ul className="list-disc pl-4 text-muted-foreground">
                  {importConflicts.accounts!.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </div>
            )}
            {(importConflicts.proxies ?? []).length > 0 && (
              <div>
                <p className="font-medium mb-1">{t('menu.proxies')}</p>
                <ul className="list-disc pl-4 text-muted-foreground">
                  {importConflicts.proxies!.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </div>
            )}
            {(importConflicts.channels ?? []).length > 0 && (
              <div>
                <p className="font-medium mb-1">{t('menu.channels')}</p>
                <ul className="list-disc pl-4 text-muted-foreground">
                  {importConflicts.channels!.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => performImport(true, lastImportPayload ?? buildImportPayload())}
              disabled={importing}
            >
              {t('settings.import_overwrite')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

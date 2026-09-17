// 通知渠道添加/编辑对话框：与 AccountDialog 同一套 radix Dialog 模式。
// z-70/80 高于设置弹框(z-55)，可从设置内直接叠加打开。

import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isDirty, useDismissGuard } from '@/lib/dismiss-guard'
import { useCreateNotifyChannel, useUpdateNotifyChannel } from '@/lib/queries'
import type { NotifyChannel, NotifyContentLevel } from '@/lib/types'

const EVENT_TYPES = ['mail_new', 'mail_rule', 'sync_failed', 'account_status'] as const

// 推送内容的三档。顺序就是「带出去的内容由少到多」，让选择本身有方向感。
const CONTENT_LEVELS: NotifyContentLevel[] = ['basic', 'snippet', 'full']
const EVENT_LABEL: Record<string, string> = {
  mail_new: 'notif.tabMail',
  sync_failed: 'notif.tabSync',
  account_status: 'notif.tabAccount',
  mail_rule: 'notif.tabRule',
}

export interface ChannelDialogProps {
  open: boolean
  channel: NotifyChannel | null // null = 添加模式，非空 = 编辑模式
  onOpenChange: (open: boolean) => void
}

interface FormState {
  name: string
  kind: string
  url: string
  secret: string
  events: string[]
  contentLevel: NotifyContentLevel
}

function defaultForm(): FormState {
  // 默认摘要：与后端对老渠道的回落一致，也是大多数人想要的
  return { name: '', kind: 'webhook', url: '', secret: '', events: ['mail_new'], contentLevel: 'snippet' }
}

function formFromChannel(c: NotifyChannel): FormState {
  return {
    name: c.name,
    kind: c.kind,
    url: c.url,
    secret: '',
    events: c.events,
    contentLevel: c.content_level ?? 'snippet',
  }
}

interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}

function Field({ label, hint, children }: FieldProps) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>{label}</Label>
      {children}
      {hint && <div style={{ fontSize: '0.75rem', color: 'var(--ink-3)' }}>{hint}</div>}
    </div>
  )
}

export function ChannelDialog({ open, channel, onOpenChange }: ChannelDialogProps) {
  const { t } = useTranslation()
  const isEdit = channel !== null

  const [form, setForm] = React.useState<FormState>(defaultForm)
  const [validationError, setValidationError] = React.useState<string | null>(null)

  // 打开时根据模式初始化表单
  React.useEffect(() => {
    if (open) {
      setForm(channel ? formFromChannel(channel) : defaultForm())
      setValidationError(null)
    }
  }, [channel, open])

  // 有内容时拦住"点框外关闭"：渠道要填名称、URL、密钥、事件勾选，
  // 误点一次全部重来。基线与上面那个初始化 effect 用同一个表达式。
  const baseline = React.useMemo(
    () => (channel ? formFromChannel(channel) : defaultForm()),
    [channel],
  )
  const { contentRef, dismissProps } = useDismissGuard(() => isDirty(form, baseline))

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  function toggleEvent(ev: string) {
    setForm((prev) => ({
      ...prev,
      events: prev.events.includes(ev) ? prev.events.filter((e) => e !== ev) : [...prev.events, ev],
    }))
  }

  const createCh = useCreateNotifyChannel()
  const updateCh = useUpdateNotifyChannel()
  const isSaving = createCh.isPending || updateCh.isPending

  function handleSave() {
    setValidationError(null)
    if (!form.name.trim() || !form.url.trim()) {
      setValidationError(t('settings.notify.invalid'))
      return
    }
    const input = {
      name: form.name.trim(),
      kind: form.kind,
      url: form.url.trim(),
      secret: form.secret,
      events: form.events,
      enabled: channel?.enabled ?? true,
      content_level: form.contentLevel,
    }
    if (isEdit && channel) {
      updateCh.mutate({ id: channel.id, input }, { onSuccess: () => onOpenChange(false) })
    } else {
      createCh.mutate(input, { onSuccess: () => onOpenChange(false) })
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className="fixed inset-0 z-[70]"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />
        <Dialog.Content
          ref={contentRef}
          {...dismissProps}
          className="fixed left-1/2 top-1/2 z-[80] -translate-x-1/2 -translate-y-1/2 w-[460px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-hidden rounded-xl shadow-xl flex flex-col gap-0 outline-none"
          style={{ background: 'var(--surface)', color: 'var(--ink)' }}
          aria-describedby={undefined}
        >
          {/* 标题栏 */}
          <div
            className="flex items-center justify-between px-6 py-4"
            style={{ borderBottom: '1px solid var(--rule)' }}
          >
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {isEdit ? t('settings.notify.editChannel') : t('settings.notify.addChannel')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('settings.notify.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          {/* 表单主体（唯一滚动区：外层 overflow-hidden 保圆角，头尾固定） */}
          <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-6 py-5">
            <Field label={t('settings.notify.name')}>
              <Input value={form.name} onChange={(e) => set('name', e.target.value)} />
            </Field>

            <Field label={t('settings.notify.kind')}>
              <div className="mode-toggle" style={{ alignSelf: 'flex-start' }}>
                <button
                  type="button"
                  className={form.kind === 'webhook' ? 'active' : ''}
                  onClick={() => set('kind', 'webhook')}
                >
                  {t('settings.notify.kindWebhook')}
                </button>
                <button
                  type="button"
                  className={form.kind === 'feishu' ? 'active' : ''}
                  onClick={() => set('kind', 'feishu')}
                >
                  {t('settings.notify.kindFeishu')}
                </button>
              </div>
            </Field>

            <Field label={t('settings.notify.url')}>
              <Input
                value={form.url}
                onChange={(e) => set('url', e.target.value)}
                placeholder="https://..."
              />
            </Field>

            <Field
              label={t('settings.notify.secret')}
              hint={isEdit ? t('settings.notify.secretKeep') : t('settings.notify.secretHint')}
            >
              <Input
                type="password"
                value={form.secret}
                onChange={(e) => set('secret', e.target.value)}
                autoComplete="new-password"
              />
            </Field>

            <Field label={t('settings.notify.events')}>
              <div className="lt-chips">
                {EVENT_TYPES.map((ev) => (
                  <button
                    key={ev}
                    type="button"
                    className={'chip' + (form.events.includes(ev) ? ' active' : '')}
                    onClick={() => toggleEvent(ev)}
                  >
                    {t(EVENT_LABEL[ev])}
                  </button>
                ))}
              </div>
            </Field>

            <Field
              label={t('settings.notify.contentLevel')}
              hint={t(`settings.notify.level_${form.contentLevel}_hint`)}
            >
              <div className="mode-toggle" style={{ alignSelf: 'flex-start' }}>
                {CONTENT_LEVELS.map((lv) => (
                  <button
                    key={lv}
                    type="button"
                    className={form.contentLevel === lv ? 'active' : ''}
                    onClick={() => set('contentLevel', lv)}
                  >
                    {t(`settings.notify.level_${lv}`)}
                  </button>
                ))}
              </div>
            </Field>

            {validationError && (
              <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)' }}>{validationError}</div>
            )}
          </div>

          {/* 底部操作 */}
          <div
            className="flex items-center justify-end gap-2 px-6 py-4"
            style={{ borderTop: '1px solid var(--rule)' }}
          >
            <Dialog.Close asChild>
              <Button variant="outline">{t('settings.notify.cancel')}</Button>
            </Dialog.Close>
            <Button onClick={handleSave} disabled={isSaving}>
              {t('settings.notify.save')}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// AI 配置添加/编辑对话框：与 ChannelDialog 同一套 radix Dialog 模式，
// z-70/80 高于设置弹框(z-55)，可从设置内直接叠加打开。
//
// ── 为什么把预设地址摆出来 ─────────────────────────────────────────────────
//
// 「OpenAI 兼容」对用户不是一句自明的话：用户知道自己买了某家的额度，
// 但不知道该往框里填什么。列几条常见地址，配置这件事就从"去翻文档"
// 变成"照抄一行"。列表不求全，只求覆盖到最常见的几种来路。

import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiErrorMessage } from '@/lib/api'
import { isDirty, useDismissGuard } from '@/lib/dismiss-guard'
import { useCreateAIProvider, useUpdateAIProvider } from '@/lib/queries'
import type { AIProvider, AIProviderInput } from '@/lib/types'

/**
 * 常见服务的接口地址。
 *
 * 只列地址不列模型名：模型名各家改得勤，写死在界面上迟早过期，
 * 而过期的模型名比没有更糟——用户照抄之后得到的是一个 404。
 */
const PRESETS: readonly { label: string; url: string }[] = [
  { label: 'OpenAI', url: 'https://api.openai.com/v1' },
  { label: 'DeepSeek', url: 'https://api.deepseek.com/v1' },
  { label: 'Ollama', url: 'http://127.0.0.1:11434/v1' },
]

export interface AIProviderDialogProps {
  open: boolean
  provider: AIProvider | null // null = 添加模式
  onOpenChange: (open: boolean) => void
}

interface FormState {
  name: string
  baseURL: string
  model: string
  // 空串 = 不改动已保存的那份，不是「清成空」——清除走 clearKey。
  apiKey: string
  clearKey: boolean
}

function formFrom(p: AIProvider | null): FormState {
  return {
    name: p?.name ?? '',
    baseURL: p?.base_url ?? '',
    model: p?.model ?? '',
    apiKey: '',
    clearKey: false,
  }
}

function Field({ label, htmlFor, hint, children }: {
  label: string
  htmlFor: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={htmlFor} style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>{label}</Label>
      {children}
      {hint && <div style={{ fontSize: '0.75rem', color: 'var(--ink-3)' }}>{hint}</div>}
    </div>
  )
}

export function AIProviderDialog({ open, provider, onOpenChange }: AIProviderDialogProps) {
  const { t } = useTranslation()
  const isEdit = provider !== null
  // ⚠ 初值只在**挂载时**求一次，而调用方是「打开时才挂载」（见 AISection），
  // 所以每次打开都天然是干净状态，不需要一个 open 变真就重置表单的 effect——
  // 那种写法会在 effect 里同步 setState，触发级联渲染（react-hooks/set-state-in-effect）。
  const [form, setForm] = React.useState<FormState>(() => formFrom(provider))
  const [error, setError] = React.useState<string | null>(null)

  // 有改动时拦住「点框外关闭」：地址、模型、密钥误点一下全部重来。
  const baseline = React.useMemo(() => formFrom(provider), [provider])
  const { contentRef, dismissProps } = useDismissGuard(() => isDirty(form, baseline))

  const create = useCreateAIProvider()
  const update = useUpdateAIProvider()
  const saving = create.isPending || update.isPending

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
    setError(null)
  }

  function handleSave() {
    if (!form.baseURL.trim() || !form.model.trim()) {
      setError(t('settings.ai.invalid'))
      return
    }
    const input: AIProviderInput = {
      name: form.name.trim(),
      base_url: form.baseURL.trim(),
      model: form.model.trim(),
    }
    if (form.clearKey) input.clear_key = true
    else if (form.apiKey.trim() !== '') input.api_key = form.apiKey.trim()

    // 后端的拒绝理由是写给人看的中文（地址少了 scheme 之类），原样显示。
    const opts = {
      onSuccess: () => onOpenChange(false),
      onError: (err: unknown) => setError(apiErrorMessage(err, t('settings.ai.errSave'))),
    }
    if (isEdit && provider) update.mutate({ id: provider.id, input }, opts)
    else create.mutate(input, opts)
  }

  const keySaved = provider?.key_set === true

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
          className="fixed left-1/2 top-1/2 z-[80] -translate-x-1/2 -translate-y-1/2 w-[480px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-hidden rounded-xl shadow-xl flex flex-col gap-0 outline-none"
          style={{ background: 'var(--surface)', color: 'var(--ink)' }}
          aria-describedby={undefined}
        >
          <div
            className="flex items-center justify-between px-6 py-4"
            style={{ borderBottom: '1px solid var(--rule)' }}
          >
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {isEdit ? t('settings.ai.editTitle') : t('settings.ai.addTitle')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('settings.ai.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-6 py-5">
            <Field label={t('settings.ai.name')} htmlFor="ai-name">
              <Input
                id="ai-name"
                value={form.name}
                placeholder={t('settings.ai.namePlaceholder')}
                onChange={(e) => set('name', e.target.value)}
              />
            </Field>

            <Field label={t('settings.ai.baseUrl')} htmlFor="ai-base-url" hint={t('settings.ai.baseUrlHelp')}>
              <Input
                id="ai-base-url"
                placeholder="https://api.openai.com/v1"
                value={form.baseURL}
                onChange={(e) => set('baseURL', e.target.value)}
              />
              <div className="settings-copybar">
                <span className="sf-help" style={{ margin: 0 }}>{t('settings.ai.presets')}</span>
                {PRESETS.map((p) => (
                  <Button key={p.url} size="sm" variant="outline" onClick={() => set('baseURL', p.url)}>
                    {p.label}
                  </Button>
                ))}
              </div>
            </Field>

            <Field label={t('settings.ai.model')} htmlFor="ai-model" hint={t('settings.ai.modelHelp')}>
              <Input
                id="ai-model"
                placeholder="gpt-4o-mini"
                value={form.model}
                onChange={(e) => set('model', e.target.value)}
              />
            </Field>

            <Field label={t('settings.ai.apiKey')} htmlFor="ai-api-key" hint={t('settings.ai.apiKeyHelp')}>
              <Input
                id="ai-api-key"
                type="password"
                autoComplete="new-password"
                disabled={form.clearKey}
                placeholder={
                  form.clearKey
                    ? t('settings.ai.keyWillClear')
                    : keySaved ? t('settings.ai.keySaved') : t('settings.ai.keyEmpty')
                }
                value={form.apiKey}
                onChange={(e) => set('apiKey', e.target.value)}
              />
              {keySaved && (
                <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12.5, color: 'var(--ink-2)' }}>
                  <input
                    type="checkbox"
                    checked={form.clearKey}
                    onChange={(e) => setForm((prev) => ({ ...prev, clearKey: e.target.checked, apiKey: '' }))}
                  />
                  {t('settings.ai.clearKey')}
                </label>
              )}
            </Field>

            {error && (
              <div role="alert" style={{ fontSize: '0.8125rem', color: 'var(--destructive)' }}>{error}</div>
            )}
          </div>

          <div
            className="flex items-center justify-end gap-2 px-6 py-4"
            style={{ borderTop: '1px solid var(--rule)' }}
          >
            <Dialog.Close asChild>
              <Button variant="outline">{t('settings.ai.cancel')}</Button>
            </Dialog.Close>
            <Button onClick={handleSave} disabled={saving}>
              {t('settings.ai.save')}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// 设置 → AI 翻译：翻译偏好 + 多条 OpenAI 兼容接口配置（使用列表）。
//
// ── 为什么是一张列表而不是一个表单 ─────────────────────────────────────────
//
// 单个接口一欠费、一限流，翻译就整体不可用，只能手动来这里改配置。
// 配成列表后，翻译按顺序使用：前一条失败就整封换下一条重翻（见后端
// translate.translateWithFailover）。所以这里的顺序本身就是配置的一部分，
// 每条的健康状态也要摆出来——用户得看得到「为什么没用第一条」。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { apiErrorMessage } from '@/lib/api'
import {
  useAIProviders,
  useDeleteAIProvider,
  useReorderAIProviders,
  useResetAIProvider,
  useSettings,
  useTestAIProvider,
  useTranslateLanguages,
  useUpdateAIProvider,
  useUpdateSettings,
} from '@/lib/queries'
import { moveItem } from '@/lib/reorder'
import type { AIProvider } from '@/lib/types'
import { AIProviderDialog } from './AIProviderDialog'

/** 从完整的 chat/completions 地址里取主机名，列表里显示它比整串地址好认 */
function hostOf(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

type Tone = 'ok' | 'warn' | 'muted'

/**
 * 一条配置此刻的状态文案。
 *
 * 「上次失败」只在失败比成功更近时才显示：成功一次之后，更早的那次失败
 * 已经不说明任何问题，再挂着只会让人以为它还坏着。
 */
function describeStatus(
  p: AIProvider,
  t: (k: string, o?: Record<string, unknown>) => string,
  locale: string,
): { text: string; tone: Tone } {
  const st = p.status
  const kindText = st.last_kind ? t(`settings.ai.kind_${st.last_kind}`) : ''
  if (!p.enabled) return { text: t('settings.ai.stateDisabled'), tone: 'muted' }
  if (p.cooling && st.cooldown_until) {
    const time = new Date(st.cooldown_until).toLocaleTimeString(locale)
    return { text: `${t('settings.ai.stateCooling', { time })} · ${kindText}`, tone: 'warn' }
  }
  const okAt = st.last_ok_at ? Date.parse(st.last_ok_at) : 0
  const failAt = st.last_fail_at ? Date.parse(st.last_fail_at) : 0
  if (failAt > okAt) {
    // 手动解除暂停会把连续失败次数清零，但保留最后一次错误：这时还显示成警告色的
    // 「上次失败」，用户会以为解除没生效。
    if (st.failures === 0) return { text: t('settings.ai.stateResumed'), tone: 'muted' }
    return { text: `${t('settings.ai.stateFailed')} · ${kindText}`, tone: 'warn' }
  }
  if (okAt > 0) {
    const time = new Date(okAt).toLocaleString(locale)
    return { text: `${t('settings.ai.stateOk')} · ${t('settings.ai.lastOk', { time })}`, tone: 'ok' }
  }
  return { text: t('settings.ai.stateUnknown'), tone: 'muted' }
}

const TONE_COLOR: Record<Tone, string> = {
  ok: 'var(--accent)',
  warn: 'var(--destructive)',
  muted: 'var(--ink-3)',
}

export function AISection() {
  // 时间按界面语言排版，不跟浏览器区域：中文界面里冒出一个「10:25:37 PM」很扎眼
  const { t, i18n } = useTranslation()
  const confirm = useConfirm()
  const { toast } = useToast()
  const { data: providers = [] } = useAIProviders()
  const updateP = useUpdateAIProvider()
  const deleteP = useDeleteAIProvider()
  const reorder = useReorderAIProviders()
  const testP = useTestAIProvider()
  const resetP = useResetAIProvider()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<AIProvider | null>(null)
  const [testingID, setTestingID] = React.useState<number | null>(null)

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(p: AIProvider) {
    setEditing(p)
    setDialogOpen(true)
  }

  function handleMove(index: number, delta: number) {
    const next = moveItem(providers, index, delta)
    if (next === providers) return
    // 失败时列表会弹回原位，不说一声用户只会以为按钮坏了
    reorder.mutate(next.map((p) => p.id), {
      onError: () => toast(t('settings.ai.reorderFailed')),
    })
  }

  async function handleDelete(p: AIProvider) {
    const ok = await confirm({
      title: t('settings.ai.deleteConfirm'),
      body: p.name,
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    deleteP.mutate(p.id, { onError: failToast })
  }

  // 开关、删除、解除暂停失败时必须说一声：界面上什么都没变，用户只会以为按钮坏了
  function failToast(err: unknown) {
    toast(apiErrorMessage(err, t('settings.ai.errSave')))
  }

  function handleTest(p: AIProvider) {
    setTestingID(p.id)
    testP.mutate(p.id, {
      onSuccess: (res) => {
        if (res.ok) toast(t('settings.ai.testOk', { name: p.name, ms: res.latency_ms }))
        else toast(t('settings.ai.testFail', { name: p.name, error: res.error ?? '' }))
      },
      onError: (err) => toast(t('settings.ai.testFail', { name: p.name, error: apiErrorMessage(err, '') })),
      onSettled: () => setTestingID(null),
    })
  }

  function handleReset(p: AIProvider) {
    resetP.mutate(p.id, { onSuccess: () => toast(t('settings.ai.resetDone')), onError: failToast })
  }

  const anyEnabled = providers.some((p) => p.enabled)

  return (
    <div className="settings-block">
      <h3>{t('settings.ai.title')}</h3>
      <p className="help">{t('settings.ai.intro')}</p>

      <TargetLangField />

      <h4 style={{ margin: '22px 0 4px' }}>{t('settings.ai.providersTitle')}</h4>
      <p className="help">{t('settings.ai.providersHelp')}</p>

      {/* 用户来这个页面十有八九是因为翻译按钮没反应，第一句话就该回答「为什么」。 */}
      {!anyEnabled && (
        <p className="help" style={{ color: 'var(--ink-3)' }}>
          {providers.length === 0 ? t('settings.ai.statusEmpty') : t('settings.ai.allDisabled')}
        </p>
      )}

      {providers.map((p, i) => {
        const status = describeStatus(p, t, i18n.language)
        return (
          <div key={p.id} className="account-card" data-testid={`ai-provider-${p.id}`}>
            <div className="ac-avatar" style={{ background: p.enabled ? 'var(--accent)' : 'var(--ink-3)' }} aria-hidden="true">
              {i + 1}
            </div>
            <div style={{ minWidth: 0 }}>
              <div className="ac-name">{p.name}</div>
              <div className="ac-mail" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {p.model} · {hostOf(p.base_url)}{p.key_set ? '' : ` · ${t('settings.ai.noKey')}`}
              </div>
              <div
                className="ai-status"
                title={p.status.last_error}
                style={{ fontSize: 11.5, marginTop: 3, color: TONE_COLOR[status.tone] }}
              >
                {status.text}
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
              {/* 首行上移、末行下移置灰而不是隐藏，免得按钮列在各行之间错位 */}
              <div className="ac-reorder">
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => handleMove(i, -1)}
                  disabled={i === 0}
                  title={t('settings.ai.moveUp')}
                  aria-label={t('settings.ai.moveUp')}
                >
                  <Icon name="chevron-up" size={13} />
                </button>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => handleMove(i, 1)}
                  disabled={i === providers.length - 1}
                  title={t('settings.ai.moveDown')}
                  aria-label={t('settings.ai.moveDown')}
                >
                  <Icon name="chevron-down" size={13} />
                </button>
              </div>
              {p.cooling && p.enabled && (
                <button type="button" className="pill-btn" onClick={() => handleReset(p)} disabled={resetP.isPending}>
                  {t('settings.ai.reset')}
                </button>
              )}
              <button
                type="button"
                role="switch"
                aria-checked={p.enabled}
                className={'toggle' + (p.enabled ? ' on' : '')}
                onClick={() => updateP.mutate({ id: p.id, input: { enabled: !p.enabled } }, { onError: failToast })}
                aria-label={p.enabled ? t('settings.ai.disable') : t('settings.ai.enable')}
              />
              <button
                type="button"
                className="icon-btn"
                title={testingID === p.id ? t('settings.ai.testing') : t('settings.ai.test')}
                aria-label={t('settings.ai.test')}
                onClick={() => handleTest(p)}
                disabled={testingID != null}
              >
                <Icon name="send" size={13} />
              </button>
              <button type="button" className="icon-btn" title={t('settings.ai.edit')} aria-label={t('settings.ai.edit')} onClick={() => openEdit(p)}>
                <Icon name="compose" size={13} />
              </button>
              <button
                type="button"
                className="icon-btn"
                title={t('settings.ai.delete')}
                aria-label={t('settings.ai.delete')}
                onClick={() => handleDelete(p)}
                style={{ color: 'var(--destructive)' }}
              >
                <Icon name="trash" size={13} />
              </button>
            </div>
          </div>
        )
      })}

      <button type="button" className="pill-btn" style={{ marginTop: 14 }} onClick={openAdd}>
        <Icon name="plus" size={12} />{t('settings.ai.add')}
      </button>

      {/* 打开时才挂载：表单初值在挂载时从 provider 求出，见 AIProviderDialog */}
      {dialogOpen && <AIProviderDialog open provider={editing} onOpenChange={setDialogOpen} />}
    </div>
  )
}

/**
 * 默认目标语言：选中即保存。
 *
 * 只有一个下拉框，再配一个「保存」按钮纯属多一步；而且那个按钮以前和接口配置
 * 共用，改个语言会连带把没动过的地址也提交一遍。
 */
function TargetLangField() {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const { data: langs } = useTranslateLanguages()
  const update = useUpdateSettings()
  const stored = settings?.translate_target_lang ?? 'zh'
  const [saved, setSaved] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  function change(value: string) {
    setError(null)
    update.mutate({ translate_target_lang: value }, {
      onSuccess: () => {
        setSaved(true)
        setTimeout(() => setSaved(false), 2500)
      },
      onError: (err) => setError(apiErrorMessage(err, t('settings.ai.errSave'))),
    })
  }

  return (
    <div className="settings-field">
      <label className="sf-label" htmlFor="ai-target-lang">{t('settings.ai.targetLang')}</label>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <select
          id="ai-target-lang"
          value={stored}
          disabled={update.isPending}
          onChange={(e) => change(e.target.value)}
        >
          {/* 清单来自后端：语言代码同时用于提示词与校验，前端再维护一份
              迟早会漂移，表现为"选得到却报不支持"。 */}
          {(langs?.languages ?? [{ code: stored, name: stored, native: stored }]).map((l) => (
            <option key={l.code} value={l.code}>{l.native}</option>
          ))}
        </select>
        {saved && <span style={{ fontSize: 12, color: 'var(--accent)' }}>{t('settings.ai.saved')}</span>}
      </div>
      <p className="sf-help">{t('settings.ai.targetLangHelp')}</p>
      {error != null && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: 4 }}>{error}</div>
      )}
    </div>
  )
}

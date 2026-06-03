// 设置 → 通知渠道：外发推送渠道(通用 webhook / 飞书)的增删改 + 测试 + 投递日志。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import {
  useNotifyChannels,
  useCreateNotifyChannel,
  useUpdateNotifyChannel,
  useDeleteNotifyChannel,
  useTestNotifyChannel,
  useNotifyLogs,
} from '@/lib/queries'
import type { NotifyChannel } from '@/lib/types'

const EVENT_TYPES = ['mail_new', 'sync_failed', 'account_status'] as const
const EVENT_LABEL: Record<string, string> = {
  mail_new: 'notif.tabMail',
  sync_failed: 'notif.tabSync',
  account_status: 'notif.tabAccount',
}

interface FormState {
  id: number | null
  name: string
  kind: string
  url: string
  secret: string
  events: string[]
  enabled: boolean
}

function emptyForm(): FormState {
  return { id: null, name: '', kind: 'webhook', url: '', secret: '', events: ['mail_new'], enabled: true }
}

export function NotifyChannelsSection() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { data: channels = [] } = useNotifyChannels()
  const { data: logs = [] } = useNotifyLogs()
  const createCh = useCreateNotifyChannel()
  const updateCh = useUpdateNotifyChannel()
  const deleteCh = useDeleteNotifyChannel()
  const testCh = useTestNotifyChannel()

  const [form, setForm] = React.useState<FormState | null>(null) // null = 表单关闭
  const [showLogs, setShowLogs] = React.useState(false)

  function openAdd() { setForm(emptyForm()) }
  function openEdit(c: NotifyChannel) {
    setForm({ id: c.id, name: c.name, kind: c.kind, url: c.url, secret: '', events: c.events, enabled: c.enabled })
  }
  function toggleEvent(ev: string) {
    setForm((f) => {
      if (!f) return f
      const events = f.events.includes(ev) ? f.events.filter((e) => e !== ev) : [...f.events, ev]
      return { ...f, events }
    })
  }

  function handleSave() {
    if (!form) return
    if (!form.name.trim() || !form.url.trim()) {
      toast(t('settings.notify.invalid'))
      return
    }
    const input = {
      name: form.name.trim(), kind: form.kind, url: form.url.trim(),
      secret: form.secret, events: form.events, enabled: form.enabled,
    }
    if (form.id != null) {
      updateCh.mutate({ id: form.id, input }, { onSuccess: () => { toast(t('settings.notify.saved')); setForm(null) } })
    } else {
      createCh.mutate(input, { onSuccess: () => { toast(t('settings.notify.saved')); setForm(null) } })
    }
  }

  function handleDelete(c: NotifyChannel) {
    if (!window.confirm(t('settings.notify.deleteConfirm'))) return
    deleteCh.mutate(c.id)
  }

  function handleTest(c: NotifyChannel) {
    testCh.mutate(c.id, {
      onSuccess: () => toast(t('settings.notify.testOk')),
      onError: () => toast(t('settings.notify.testFail')),
    })
  }

  function handleToggleEnabled(c: NotifyChannel) {
    updateCh.mutate({ id: c.id, input: { name: c.name, kind: c.kind, url: c.url, events: c.events, enabled: !c.enabled } })
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.notify.title')}</h3>
      <p className="help">{t('settings.notify.help')}</p>

      {/* 渠道列表 */}
      {channels.length === 0 && !form && (
        <div style={{ color: 'var(--ink-3)', fontSize: 13, padding: '10px 0' }}>{t('settings.notify.none')}</div>
      )}
      {channels.map((c) => (
        <div key={c.id} className="account-card">
          <div className="ac-avatar" style={{ background: 'var(--accent)' }} aria-hidden="true">
            {c.kind === 'feishu' ? '飞' : 'W'}
          </div>
          <div style={{ minWidth: 0 }}>
            <div className="ac-name">{c.name}</div>
            <div className="ac-mail" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {c.kind === 'feishu' ? t('settings.notify.kindFeishu') : t('settings.notify.kindWebhook')} · {c.url}
            </div>
            <div style={{ display: 'flex', gap: 4, marginTop: 4, flexWrap: 'wrap' }}>
              {c.events.map((ev) => (
                <span key={ev} className="chip" style={{ fontSize: 11, padding: '1px 7px' }}>
                  {t(EVENT_LABEL[ev] ?? ev)}
                </span>
              ))}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
            <button
              type="button"
              role="switch"
              aria-checked={c.enabled}
              className={'toggle' + (c.enabled ? ' on' : '')}
              onClick={() => handleToggleEnabled(c)}
              aria-label={c.enabled ? t('settings.account.disable') : t('settings.account.enable')}
            />
            <button type="button" className="icon-btn" title={t('settings.notify.test')} onClick={() => handleTest(c)} disabled={testCh.isPending}>
              <Icon name="send" size={13} />
            </button>
            <button type="button" className="icon-btn" title={t('settings.account.edit')} onClick={() => openEdit(c)}>
              <Icon name="compose" size={13} />
            </button>
            <button type="button" className="icon-btn" title={t('settings.account.delete')} onClick={() => handleDelete(c)} style={{ color: 'var(--destructive)' }}>
              <Icon name="trash" size={13} />
            </button>
          </div>
        </div>
      ))}

      {/* 新增/编辑表单 */}
      {form ? (
        <div className="account-card" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 10 }}>
          <div className="settings-row">
            <div className="sr-label">{t('settings.notify.name')}</div>
            <input className="inline-input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="settings-row">
            <div className="sr-label">{t('settings.notify.kind')}</div>
            <div className="mode-toggle">
              <button type="button" className={form.kind === 'webhook' ? 'active' : ''} onClick={() => setForm({ ...form, kind: 'webhook' })}>
                {t('settings.notify.kindWebhook')}
              </button>
              <button type="button" className={form.kind === 'feishu' ? 'active' : ''} onClick={() => setForm({ ...form, kind: 'feishu' })}>
                {t('settings.notify.kindFeishu')}
              </button>
            </div>
          </div>
          <div className="settings-row">
            <div className="sr-label">{t('settings.notify.url')}</div>
            <input className="inline-input" style={{ minWidth: 240 }} value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://..." />
          </div>
          <div className="settings-row">
            <div>
              <div className="sr-label">{t('settings.notify.secret')}</div>
              <div className="sr-help">{form.id != null ? t('settings.notify.secretKeep') : t('settings.notify.secretHint')}</div>
            </div>
            <input className="inline-input" type="password" value={form.secret} onChange={(e) => setForm({ ...form, secret: e.target.value })} autoComplete="new-password" />
          </div>
          <div className="settings-row">
            <div className="sr-label">{t('settings.notify.events')}</div>
            <div className="filter-chips" style={{ padding: 0 }}>
              {EVENT_TYPES.map((ev) => (
                <button key={ev} type="button" className={'chip' + (form.events.includes(ev) ? ' active' : '')} onClick={() => toggleEvent(ev)}>
                  {t(EVENT_LABEL[ev])}
                </button>
              ))}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 10, marginTop: 4 }}>
            <button type="button" className="pill-btn primary" onClick={handleSave} disabled={createCh.isPending || updateCh.isPending}>
              {t('settings.notify.save')}
            </button>
            <button type="button" className="pill-btn" onClick={() => setForm(null)}>{t('settings.notify.cancel')}</button>
          </div>
        </div>
      ) : (
        <button type="button" className="pill-btn" style={{ marginTop: 14 }} onClick={openAdd}>
          <Icon name="plus" size={12} /> {t('settings.notify.addChannel')}
        </button>
      )}

      {/* 投递日志（可折叠） */}
      <div style={{ marginTop: 20 }}>
        <button
          type="button"
          className="pill-btn"
          onClick={() => setShowLogs((s) => !s)}
        >
          {t('settings.notify.logsTitle')} ({logs.length})
        </button>
        {showLogs && (
          <div style={{ marginTop: 10 }}>
            {logs.length === 0 ? (
              <div style={{ color: 'var(--ink-3)', fontSize: 13 }}>{t('settings.notify.noLogs')}</div>
            ) : (
              logs.map((l) => (
                <div key={l.id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '6px 0', fontSize: 12.5, borderBottom: '1px solid var(--rule)' }}>
                  <span style={{ color: l.status === 'ok' ? 'var(--accent)' : 'var(--destructive)', fontWeight: 600, width: 48 }}>
                    {l.status === 'ok' ? t('settings.notify.statusOk') : t('settings.notify.statusFailed')}
                  </span>
                  <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {l.channel_name} · {t(EVENT_LABEL[l.type] ?? l.type)}
                    {l.error && <span style={{ color: 'var(--ink-3)' }}> — {l.error}</span>}
                  </span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--ink-3)', flexShrink: 0 }}>
                    {new Date(l.created_at).toLocaleString()}
                  </span>
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  )
}

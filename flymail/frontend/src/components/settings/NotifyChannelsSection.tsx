// 设置 → 通知渠道：外发推送渠道(通用 webhook / 飞书)的增删改 + 测试 + 投递日志。
// 添加/编辑走 ChannelDialog 对话框（radix，浮于设置弹框之上），列表本身不做内嵌变形。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { ChannelDialog } from './ChannelDialog'
import {
  useNotifyChannels,
  useUpdateNotifyChannel,
  useDeleteNotifyChannel,
  useTestNotifyChannel,
  useNotifyLogs,
} from '@/lib/queries'
import type { NotifyChannel } from '@/lib/types'

const EVENT_LABEL: Record<string, string> = {
  mail_new: 'notif.tabMail',
  sync_failed: 'notif.tabSync',
  account_status: 'notif.tabAccount',
}

export function NotifyChannelsSection() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { data: channels = [] } = useNotifyChannels()
  const { data: logs = [] } = useNotifyLogs()
  const updateCh = useUpdateNotifyChannel()
  const deleteCh = useDeleteNotifyChannel()
  const testCh = useTestNotifyChannel()

  // 对话框状态：open + 编辑目标（null = 添加模式）
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<NotifyChannel | null>(null)
  const [showLogs, setShowLogs] = React.useState(false)

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(c: NotifyChannel) {
    setEditing(c)
    setDialogOpen(true)
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
      {channels.length === 0 && (
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

      <button type="button" className="pill-btn" style={{ marginTop: 14 }} onClick={openAdd}>
        <Icon name="plus" size={12} /> {t('settings.notify.addChannel')}
      </button>

      {/* 添加/编辑对话框 */}
      <ChannelDialog open={dialogOpen} channel={editing} onOpenChange={setDialogOpen} />

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

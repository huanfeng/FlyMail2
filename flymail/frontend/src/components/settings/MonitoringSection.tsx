// 设置 → 系统监控：系统概览 + 各账户健康（只读，自动刷新）。

import { useTranslation } from 'react-i18next'
import { useMonitoringOverview, useMonitoringAccounts } from '@/lib/queries'

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function fmtUptime(sec: number, isZh: boolean): string {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return isZh ? `${d} 天 ${h} 小时` : `${d}d ${h}h`
  if (h > 0) return isZh ? `${h} 小时 ${m} 分` : `${h}h ${m}m`
  return isZh ? `${m} 分` : `${m}m`
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div
      style={{
        border: '1px solid var(--rule)', borderRadius: 10, padding: '12px 14px',
        background: 'var(--surface)', minWidth: 0,
      }}
    >
      <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 6 }}>{label}</div>
      <div style={{ fontSize: 22, fontWeight: 600, color: 'var(--ink)', fontFamily: 'var(--font-display)', overflow: 'hidden', textOverflow: 'ellipsis' }}>
        {value}
      </div>
    </div>
  )
}

export function MonitoringSection() {
  const { t, i18n } = useTranslation()
  const isZh = i18n.language.startsWith('zh')
  const { data: ov } = useMonitoringOverview(true)
  const { data: accounts = [] } = useMonitoringAccounts(true)

  return (
    <>
      <div className="settings-block">
        <h3>{t('settings.monitoring.title')}</h3>
        <p className="help">{t('settings.monitoring.help')}</p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(120px, 1fr))', gap: 10, marginTop: 8 }}>
          <StatCard label={t('settings.monitoring.accounts')} value={ov?.accounts ?? '—'} />
          <StatCard label={t('settings.monitoring.folders')} value={ov?.folders ?? '—'} />
          <StatCard label={t('settings.monitoring.messages')} value={ov?.messages ?? '—'} />
          <StatCard label={t('settings.monitoring.unread')} value={ov?.unread ?? '—'} />
          <StatCard label={t('settings.monitoring.workers')} value={ov?.active_workers ?? '—'} />
          <StatCard label={t('settings.monitoring.pendingWriteback')} value={ov?.pending_writeback ?? '—'} />
          <StatCard label={t('settings.monitoring.pollInterval')} value={ov ? `${ov.poll_interval_sec}s` : '—'} />
          <StatCard label={t('settings.monitoring.uptime')} value={ov ? fmtUptime(ov.uptime_sec, isZh) : '—'} />
          <StatCard label={t('settings.monitoring.dbSize')} value={ov ? fmtBytes(ov.db_size_bytes) : '—'} />
          <StatCard label={t('settings.monitoring.version')} value={ov?.version ?? '—'} />
        </div>
      </div>

      <div className="settings-block">
        <h3>{t('settings.monitoring.accountsHealth')}</h3>
        {accounts.length === 0 ? (
          <div style={{ color: 'var(--ink-3)', fontSize: 13, padding: '8px 0' }}>{t('settings.account.none')}</div>
        ) : (
          accounts.map((a) => (
            <div key={a.id} className="account-card">
              {/* 后台 worker 状态指示灯 */}
              <div
                className="ac-avatar"
                style={{ background: a.has_worker ? 'var(--accent)' : 'var(--bg-sunk)', position: 'relative' }}
                aria-hidden="true"
              >
                {(a.name || a.email).slice(0, 1).toUpperCase()}
              </div>
              <div style={{ minWidth: 0 }}>
                <div className="ac-name">
                  {a.name || a.email}
                  {!a.enabled && (
                    <span style={{ marginLeft: 8, fontSize: 11, color: 'var(--ink-3)' }}>
                      ({t('settings.account.disabled')})
                    </span>
                  )}
                </div>
                <div className="ac-mail" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {a.email}
                </div>
                <div style={{ fontSize: 11.5, color: 'var(--ink-3)', marginTop: 3, fontFamily: 'var(--font-mono)' }}>
                  {t('settings.account.messages')}: {a.message_count} · {t('settings.account.folders')}: {a.folder_count}
                  {' · '}
                  {t('settings.monitoring.lastSync')}: {a.last_sync_at ? new Date(a.last_sync_at).toLocaleString() : t('settings.account.never')}
                  {a.queue_depth > 0 && ` · ${t('settings.monitoring.queueDepth')}: ${a.queue_depth}`}
                </div>
                {a.sync_error && (
                  <div style={{ fontSize: 11.5, color: 'var(--destructive)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {a.sync_error}
                  </div>
                )}
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 4, flexShrink: 0 }}>
                <span className="chip" style={{ fontSize: 11, padding: '1px 8px' }}>
                  {a.has_worker ? t('settings.monitoring.worker') : t('settings.monitoring.noWorker')}
                </span>
                {a.breaker_open && (
                  <span
                    className="chip"
                    style={{ fontSize: 11, padding: '1px 8px', color: 'var(--destructive)', borderColor: 'var(--destructive)' }}
                  >
                    {t('settings.monitoring.breakerOpen')}
                  </span>
                )}
                <span style={{ fontSize: 11, color: a.sync_phase === 'error' ? 'var(--destructive)' : 'var(--ink-3)' }}>
                  {t(`settings.monitoring.phase.${a.sync_phase}`)}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </>
  )
}

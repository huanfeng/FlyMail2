// 设置 → 系统监控：系统概览 + 各账户健康（只读，自动刷新）。
// 账户行可展开查看运行时诊断：当前模式/IDLE 三态/熔断/队列/事件时间线。

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useMonitoringOverview, useMonitoringAccounts, useMonitoringDiagnostics } from '@/lib/queries'
import type { AccountHealth, DiagEvent } from '@/lib/types'

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

// fmtDur 把秒数格式化为 "1h2m" / "3m20s" / "45s"。
function fmtDur(sec: number): string {
  if (sec < 60) return `${sec}s`
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (h > 0) return `${h}h${m}m`
  return `${m}m${s}s`
}

// modeColor 依模式给徽标配色（用主题令牌）。
function modeColor(mode: string): string {
  switch (mode) {
    case 'idle':
      return 'var(--accent)'
    case 'polling':
    case 'inbox_sync':
    case 'task':
      return 'var(--accent)'
    case 'breaker_open':
    case 'backoff':
      return 'var(--destructive)'
    default: // disconnected / connecting
      return 'var(--ink-3)'
  }
}

// modeLabel 取模式的 i18n 文案（未知模式回退原值）。
function modeLabel(mode: string, t: TFunction): string {
  const k = `settings.monitoring.mode.${mode}`
  const v = t(k)
  return v === k ? mode : v
}

function eventLabel(type: string, t: TFunction): string {
  const k = `settings.monitoring.event.${type}`
  const v = t(k)
  return v === k ? type : v
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

// ModeBadge 显示当前模式（带颜色圆点）。
function ModeBadge({ mode, t }: { mode: string; t: TFunction }) {
  if (!mode) return null
  return (
    <span className="chip" style={{ fontSize: 11, padding: '1px 8px', display: 'inline-flex', alignItems: 'center', gap: 5 }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', background: modeColor(mode), flexShrink: 0 }} />
      {modeLabel(mode, t)}
    </span>
  )
}

// DiagFlag 是 IDLE 三态的小标记。
function DiagFlag({ label, on }: { label: string; on: boolean }) {
  return (
    <span style={{ fontSize: 11, color: on ? 'var(--ink)' : 'var(--ink-3)' }}>
      {on ? '✓' : '✕'} {label}
    </span>
  )
}

// AccountDiagPanel 展开后的运行时诊断详情（仅展开时轮询）。
function AccountDiagPanel({ accountId, expanded }: { accountId: number; expanded: boolean }) {
  const { t } = useTranslation()
  const { data } = useMonitoringDiagnostics(accountId, expanded)

  if (!data) return <div style={{ fontSize: 12, color: 'var(--ink-3)', padding: '8px 0' }}>…</div>
  if (!data.running || !data.diagnostics) {
    return <div style={{ fontSize: 12, color: 'var(--ink-3)', padding: '8px 0' }}>{t('settings.monitoring.noRunner')}</div>
  }
  const d = data.diagnostics
  return (
    <div style={{ marginTop: 8, paddingTop: 10, borderTop: '1px dashed var(--rule)', display: 'flex', flexDirection: 'column', gap: 8 }}>
      {/* 模式 + IDLE 三态 + 队列 */}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center', fontSize: 12 }}>
        <span>
          {t('settings.monitoring.currentMode')}: <b>{modeLabel(d.mode, t)}</b>{' '}
          <span style={{ color: 'var(--ink-3)' }}>({fmtDur(d.mode_seconds)})</span>
        </span>
        <DiagFlag label={t('settings.monitoring.idleCapable')} on={d.idle_capable} />
        <DiagFlag label={t('settings.monitoring.idleAllowed')} on={d.idle_allowed} />
        <DiagFlag label={t('settings.monitoring.idleActive')} on={d.idle_active} />
        <span style={{ color: 'var(--ink-3)' }}>
          {t('settings.monitoring.queueDepth')}: {d.queue_depth}
        </span>
        {d.breaker_open && (
          <span style={{ color: 'var(--destructive)' }}>
            {t('settings.monitoring.breakerOpen')} ({d.breaker_failures})
          </span>
        )}
      </div>
      {d.last_error && (
        <div style={{ fontSize: 11.5, color: 'var(--destructive)', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {t('settings.monitoring.lastError')}: {d.last_error}
        </div>
      )}
      {/* 事件时间线（最新在上） */}
      <div>
        <div style={{ fontSize: 11.5, color: 'var(--ink-3)', marginBottom: 4 }}>{t('settings.monitoring.events')}</div>
        {d.events.length === 0 ? (
          <div style={{ fontSize: 12, color: 'var(--ink-3)' }}>—</div>
        ) : (
          <div style={{ maxHeight: 220, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 2, fontFamily: 'var(--font-mono)', fontSize: 11 }}>
            {[...d.events].reverse().map((e: DiagEvent, i: number) => (
              <div key={i} style={{ display: 'flex', gap: 8, color: 'var(--ink-2, var(--ink-3))' }}>
                <span style={{ color: 'var(--ink-3)', flexShrink: 0 }}>{new Date(e.at).toLocaleTimeString()}</span>
                <span style={{ flexShrink: 0, minWidth: 96 }}>{eventLabel(e.type, t)}</span>
                {e.detail && <span style={{ color: 'var(--ink-3)', overflow: 'hidden', textOverflow: 'ellipsis' }}>{e.detail}</span>}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// AccountHealthRow 单账户健康行（可展开诊断）。
function AccountHealthRow({ a }: { a: AccountHealth }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  return (
    <div className="account-card" style={{ flexDirection: 'column', alignItems: 'stretch' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
        <div
          className="ac-avatar"
          style={{ background: a.has_worker ? 'var(--accent)' : 'var(--bg-sunk)', position: 'relative' }}
          aria-hidden="true"
        >
          {(a.name || a.email).slice(0, 1).toUpperCase()}
        </div>
        <div style={{ minWidth: 0, flex: 1 }}>
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
          </div>
          {a.sync_error && (
            <div style={{ fontSize: 11.5, color: 'var(--destructive)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {a.sync_error}
            </div>
          )}
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 4, flexShrink: 0 }}>
          <ModeBadge mode={a.mode} t={t} />
          {a.breaker_open && (
            <span className="chip" style={{ fontSize: 11, padding: '1px 8px', color: 'var(--destructive)', borderColor: 'var(--destructive)' }}>
              {t('settings.monitoring.breakerOpen')}
            </span>
          )}
          <button
            onClick={() => setExpanded((v) => !v)}
            className="chip"
            style={{ fontSize: 11, padding: '1px 8px', cursor: 'pointer', background: 'transparent' }}
          >
            {expanded ? t('settings.monitoring.collapse') : t('settings.monitoring.diagnose')}
          </button>
        </div>
      </div>
      {expanded && <AccountDiagPanel accountId={a.id} expanded={expanded} />}
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
          accounts.map((a) => <AccountHealthRow key={a.id} a={a} />)
        )}
      </div>
    </>
  )
}

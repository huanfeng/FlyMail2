// 设置 → 同步：同步深度、轮询间隔、正文预取范围。
//
// 原「邮件」页里还有会话视图（阅读偏好，已移到「阅读」）与重建索引/线程
// （维护操作，已移到「服务器」）。三类东西共用一个「保存」按钮上下，
// 用户分不清哪些要点保存、哪些改完就生效。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings, useUpdateSettings } from '@/lib/queries'
import type { BodySyncMode } from '@/lib/types'
import { Row } from '../controls'

// ── 常量 ─────────────────────────────────────────────────
const SYNC_DEPTH_MIN = 100
const SYNC_DEPTH_MAX = 5000
const POLL_INTERVAL_MIN = 30
const POLL_INTERVAL_MAX = 3600
// 正文预取的天数窗口上下限（与后端 body_sync_recent_days 校验一致）
const BODY_DAYS_MIN = 1
const BODY_DAYS_MAX = 3650

// ════════════════════════════════════════════════════════════
// 同步分区（同步深度 + 轮询间隔 + 正文预取）
// ════════════════════════════════════════════════════════════

export function SyncSection() {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()

  const [syncDepth, setSyncDepth] = React.useState<number>(settings?.sync_depth ?? 1000)
  const [pollInterval, setPollInterval] = React.useState<number>(settings?.sync_poll_interval ?? 180)
  const [bodyMode, setBodyMode] = React.useState<BodySyncMode>(settings?.body_sync_mode ?? 'new')
  const [bodyDays, setBodyDays] = React.useState<number>(settings?.body_sync_recent_days ?? 30)
  const [depthError, setDepthError] = React.useState<string | null>(null)
  const [intervalError, setIntervalError] = React.useState<string | null>(null)
  const [bodyDaysError, setBodyDaysError] = React.useState<string | null>(null)
  const [saved, setSaved] = React.useState(false)

  // 服务端数据加载后同步到本地
  React.useEffect(() => {
    if (settings?.sync_depth != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSyncDepth(settings.sync_depth)
    }
  }, [settings?.sync_depth])

  React.useEffect(() => {
    if (settings?.sync_poll_interval != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPollInterval(settings.sync_poll_interval)
    }
  }, [settings?.sync_poll_interval])

  React.useEffect(() => {
    if (settings?.body_sync_mode != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBodyMode(settings.body_sync_mode)
    }
  }, [settings?.body_sync_mode])

  React.useEffect(() => {
    if (settings?.body_sync_recent_days != null) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBodyDays(settings.body_sync_recent_days)
    }
  }, [settings?.body_sync_recent_days])

  function handleSave() {
    setDepthError(null)
    setIntervalError(null)
    setBodyDaysError(null)
    setSaved(false)

    if (syncDepth < SYNC_DEPTH_MIN || syncDepth > SYNC_DEPTH_MAX) {
      setDepthError(t('settings.mail.invalidDepth'))
      return
    }
    if (pollInterval < POLL_INTERVAL_MIN || pollInterval > POLL_INTERVAL_MAX) {
      setIntervalError(t('settings.mail.invalidInterval'))
      return
    }
    if (bodyMode === 'recent' && (bodyDays < BODY_DAYS_MIN || bodyDays > BODY_DAYS_MAX)) {
      setBodyDaysError(t('settings.mail.invalidBodyDays'))
      return
    }

    updateSettings.mutate(
      {
        sync_depth: String(syncDepth),
        sync_poll_interval: String(pollInterval),
        body_sync_mode: bodyMode,
        body_sync_recent_days: String(bodyDays),
      },
      {
        onSuccess: () => {
          setSaved(true)
          setTimeout(() => setSaved(false), 2500)
        },
      },
    )
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.mail.title')}</h3>

      {/* 同步深度 */}
      <Row
        label={t('settings.mail.syncDepth')}
        help={t('settings.mail.syncDepthHint')}
      >
        <div className="slider-row" style={{ width: 200 }}>
          <input
            type="range"
            min={SYNC_DEPTH_MIN}
            max={SYNC_DEPTH_MAX}
            step={100}
            value={syncDepth}
            onChange={(e) => { setDepthError(null); setSyncDepth(Number(e.target.value)) }}
          />
          <span className="slider-val">{syncDepth}</span>
        </div>
      </Row>
      {depthError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {depthError}
        </div>
      )}

      {/* 轮询间隔 */}
      <Row
        label={t('settings.mail.syncInterval')}
        help={t('settings.mail.syncIntervalHint')}
      >
        <div className="slider-row" style={{ width: 200 }}>
          <input
            type="range"
            min={POLL_INTERVAL_MIN}
            max={POLL_INTERVAL_MAX}
            step={30}
            value={pollInterval}
            onChange={(e) => { setIntervalError(null); setPollInterval(Number(e.target.value)) }}
          />
          <span className="slider-val">{pollInterval}s</span>
        </div>
      </Row>
      {intervalError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {intervalError}
        </div>
      )}

      {/* 正文同步范围：决定同步时把哪些邮件的正文一并下载到本地 */}
      <Row
        label={t('settings.mail.bodySync')}
        help={t('settings.mail.bodySyncHint')}
      >
        <div className="mode-toggle">
          {(['new', 'recent', 'all'] as BodySyncMode[]).map((mode) => (
            <button
              key={mode}
              type="button"
              className={bodyMode === mode ? 'active' : ''}
              onClick={() => { setBodyDaysError(null); setBodyMode(mode) }}
            >
              {t(`settings.mail.bodySync_${mode}`)}
            </button>
          ))}
        </div>
      </Row>

      {/* 天数窗口只在「最近」档有意义 */}
      {bodyMode === 'recent' && (
        <Row label={t('settings.mail.bodyDays')} help={t('settings.mail.bodyDaysHint')}>
          <div className="slider-row" style={{ width: 200 }}>
            <input
              type="range"
              min={BODY_DAYS_MIN}
              max={365}
              step={5}
              value={bodyDays}
              onChange={(e) => { setBodyDaysError(null); setBodyDays(Number(e.target.value)) }}
            />
            <span className="slider-val">{t('settings.mail.daysValue', { count: bodyDays })}</span>
          </div>
        </Row>
      )}
      {bodyDaysError && (
        <div style={{ color: 'var(--destructive)', fontSize: 12, marginTop: -6, paddingBottom: 8 }}>
          {bodyDaysError}
        </div>
      )}

      {/* 保存按钮 */}
      <div style={{ marginTop: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <button
          type="button"
          className="pill-btn"
          onClick={handleSave}
          disabled={updateSettings.isPending}
        >
          {t('settings.mail.save')}
        </button>
        {saved && (
          <span style={{ fontSize: 13, color: 'var(--accent)' }}>
            {t('settings.mail.saved')}
          </span>
        )}
      </div>
    </div>
  )
}

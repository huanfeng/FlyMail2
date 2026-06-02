import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings, useUpdateSettings } from '@/lib/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

// ────────────────────────────────────────────────────────────────────────────────
// Constants
// ────────────────────────────────────────────────────────────────────────────────

const LOAD_REMOTE_IMAGES_KEY = 'flymail_load_remote_images'
const SYNC_DEPTH_MIN = 100
const SYNC_DEPTH_MAX = 5000
const POLL_INTERVAL_MIN = 30
const POLL_INTERVAL_MAX = 3600

// ────────────────────────────────────────────────────────────────────────────────
// MailSection
// ────────────────────────────────────────────────────────────────────────────────

export function MailSection() {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()

  // 同步深度本地编辑状态
  const [syncDepth, setSyncDepth] = React.useState<number>(
    settings?.sync_depth ?? 1000,
  )
  const [depthError, setDepthError] = React.useState<string | null>(null)

  // 后台轮询间隔本地编辑状态
  const [pollInterval, setPollInterval] = React.useState<number>(
    settings?.sync_poll_interval ?? 180,
  )
  const [intervalError, setIntervalError] = React.useState<string | null>(null)

  const [saved, setSaved] = React.useState(false)

  // 远程图片开关
  const [loadRemoteImages, setLoadRemoteImages] = React.useState<boolean>(
    () => localStorage.getItem(LOAD_REMOTE_IMAGES_KEY) === 'true',
  )

  // 当服务端数据加载完成后同步到本地 state（仅首次）
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

  function handleRemoteImagesChange(e: React.ChangeEvent<HTMLInputElement>) {
    const checked = e.target.checked
    setLoadRemoteImages(checked)
    localStorage.setItem(LOAD_REMOTE_IMAGES_KEY, String(checked))
  }

  function handleSave() {
    setDepthError(null)
    setIntervalError(null)
    setSaved(false)

    if (syncDepth < SYNC_DEPTH_MIN || syncDepth > SYNC_DEPTH_MAX) {
      setDepthError(t('settings.mail.invalidDepth'))
      return
    }

    if (pollInterval < POLL_INTERVAL_MIN || pollInterval > POLL_INTERVAL_MAX) {
      setIntervalError(t('settings.mail.invalidInterval'))
      return
    }

    updateSettings.mutate(
      {
        sync_depth: String(syncDepth),
        sync_poll_interval: String(pollInterval),
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
    <div className="flex flex-col gap-6 p-6">
      <div
        className="text-sm font-semibold"
        style={{ color: 'var(--ink-2)' }}
      >
        {t('settings.mail.title')}
      </div>

      <div className="flex flex-col gap-4 max-w-sm">
        {/* 同步深度 */}
        <div className="flex flex-col gap-1.5">
          <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
            {t('settings.mail.syncDepth')}
          </Label>
          <Input
            type="number"
            min={SYNC_DEPTH_MIN}
            max={SYNC_DEPTH_MAX}
            value={syncDepth}
            onChange={(e) => {
              setDepthError(null)
              setSyncDepth(Number(e.target.value))
            }}
          />
          <span
            className="text-xs"
            style={{ color: 'var(--ink-3)' }}
          >
            {t('settings.mail.syncDepthHint')}
          </span>
        </div>

        {/* 深度校验错误 */}
        {depthError && (
          <div
            className="rounded-md px-3 py-2 text-sm"
            style={{
              background: 'oklch(0.577 0.245 27.325 / 0.1)',
              color: 'var(--destructive)',
            }}
          >
            {depthError}
          </div>
        )}

        {/* 后台轮询间隔 */}
        <div className="flex flex-col gap-1.5">
          <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>
            {t('settings.mail.syncInterval')}
          </Label>
          <Input
            type="number"
            min={POLL_INTERVAL_MIN}
            max={POLL_INTERVAL_MAX}
            value={pollInterval}
            onChange={(e) => {
              setIntervalError(null)
              setPollInterval(Number(e.target.value))
            }}
          />
          <span
            className="text-xs"
            style={{ color: 'var(--ink-3)' }}
          >
            {t('settings.mail.syncIntervalHint')}
          </span>
        </div>

        {/* 轮询间隔校验错误 */}
        {intervalError && (
          <div
            className="rounded-md px-3 py-2 text-sm"
            style={{
              background: 'oklch(0.577 0.245 27.325 / 0.1)',
              color: 'var(--destructive)',
            }}
          >
            {intervalError}
          </div>
        )}

        {/* 远程图片开关 */}
        <div className="flex items-center gap-2">
          <input
            id="load-remote-images"
            type="checkbox"
            checked={loadRemoteImages}
            onChange={handleRemoteImagesChange}
            className="h-4 w-4 accent-[var(--accent-color)] cursor-pointer"
          />
          <Label
            htmlFor="load-remote-images"
            style={{ color: 'var(--ink-2)', fontSize: '0.8125rem', cursor: 'pointer' }}
          >
            {t('settings.mail.loadRemoteImages')}
          </Label>
        </div>

        {/* 保存按钮 + 成功提示 */}
        <div className="flex items-center gap-3">
          <Button
            size="sm"
            onClick={handleSave}
            disabled={updateSettings.isPending}
          >
            {t('settings.mail.save')}
          </Button>
          {saved && (
            <span
              className="text-sm"
              style={{ color: 'var(--accent-color)' }}
            >
              {t('settings.mail.saved')}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

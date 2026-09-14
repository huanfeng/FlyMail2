// 账户配置的导出 / 导入对话框。
//
// 与 ChannelDialog 同一套 radix Dialog 模式（z-70/80，可叠在设置弹框之上）。

import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { useExportAccounts, useImportAccounts } from '@/lib/queries'
import { downloadJson } from '@/lib/download'
import { parseBundle } from '@/lib/portable-file'
import type { Account, ImportMode, ImportResult, PortableBundle } from '@/lib/types'

export interface PortabilityDialogProps {
  open: boolean
  /** 'export' | 'import'，决定进哪一半 */
  mode: 'export' | 'import'
  accounts: Account[]
  onOpenChange: (open: boolean) => void
}

const shellClass =
  'fixed left-1/2 top-1/2 z-[80] -translate-x-1/2 -translate-y-1/2 w-[520px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-hidden rounded-xl shadow-xl flex flex-col gap-0 outline-none'

/** 勾选行：复选框 + 主标题 + 副标题。导出与导入两侧共用。 */
function PickRow({
  checked,
  onChange,
  title,
  sub,
  tag,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  title: string
  sub: string
  tag?: React.ReactNode
}) {
  return (
    <label className="portable-row">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="portable-row-text">
        <span className="portable-row-title">{title}</span>
        <span className="portable-row-sub">{sub}</span>
      </span>
      {tag}
    </label>
  )
}

export function PortabilityDialog({ open, mode, accounts, onOpenChange }: PortabilityDialogProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const doExport = useExportAccounts()
  const doImport = useImportAccounts()

  // ── 导出侧状态 ──
  //
  // ⚠ 这些初值只在**挂载时**求一次，而调用方是「打开时才挂载」
  //（SettingsDialog 里 `{portable != null && <PortabilityDialog …/>}`）。
  // 因此每次打开都天然是干净状态，不需要一个 open 变真就重置一遍的 effect——
  // 那种写法会在 effect 里同步 setState，触发级联渲染（react-hooks/set-state-in-effect）。
  const [picked, setPicked] = React.useState<Set<number>>(() => new Set(accounts.map((a) => a.id)))
  const [withPasswords, setWithPasswords] = React.useState(false)
  // 明文风险的二次确认。做成对话框内的一步而不是 confirm()：
  // 用户要在**勾选的同时**看见后果，而不是点了「导出」才被拦一下。
  const [riskAck, setRiskAck] = React.useState(false)

  // ── 导入侧状态 ──
  const [bundle, setBundle] = React.useState<PortableBundle | null>(null)
  const [fileError, setFileError] = React.useState<string | null>(null)
  const [pickedEmails, setPickedEmails] = React.useState<Set<string>>(new Set())
  const [conflict, setConflict] = React.useState<ImportMode>('skip')
  const [result, setResult] = React.useState<ImportResult | null>(null)
  const fileRef = React.useRef<HTMLInputElement>(null)

  const existingEmails = React.useMemo(
    () => new Set(accounts.map((a) => a.email.toLowerCase())),
    [accounts],
  )

  function toggle<T>(set: Set<T>, key: T, on: boolean): Set<T> {
    const next = new Set(set)
    if (on) next.add(key)
    else next.delete(key)
    return next
  }

  // ── 导出 ──
  async function handleExport() {
    try {
      const b = await doExport.mutateAsync({
        ids: [...picked],
        includePasswords: withPasswords,
      })
      const stamp = new Date().toISOString().slice(0, 10)
      downloadJson(b, `flymail-accounts-${stamp}.json`)
      toast(t('settings.portable.exportDone', { count: b.accounts.length }))
      onOpenChange(false)
    } catch {
      toast(t('settings.portable.exportFailed'))
    }
  }

  // ── 导入 ──
  async function handleFile(file: File) {
    setResult(null)
    const parsed = await parseBundle(file)
    if (!parsed.ok) {
      setBundle(null)
      setFileError(t(parsed.reasonKey))
      return
    }
    setFileError(null)
    setBundle(parsed.bundle)
    setPickedEmails(new Set(parsed.bundle.accounts.map((a) => a.email.toLowerCase())))
  }

  async function handleImport() {
    if (!bundle) return
    try {
      const res = await doImport.mutateAsync({ bundle, mode: conflict, only: [...pickedEmails] })
      setResult(res)
      // 不自动关窗：逐条结果（尤其是失败项的原因）正是用户要看的东西。
      // 关掉它等于把"哪个没导进来、为什么"一并丢掉。
    } catch {
      toast(t('settings.portable.importFailed'))
    }
  }

  const exporting = doExport.isPending
  const importing = doImport.isPending
  // 勾了含密码就必须先确认风险，否则导出按钮不可用
  const exportBlocked = picked.size === 0 || (withPasswords && !riskAck) || exporting

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className="fixed inset-0 z-[70]"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />
        <Dialog.Content className={shellClass} style={{ background: 'var(--surface)', color: 'var(--ink)' }} aria-describedby={undefined}>
          <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: '1px solid var(--rule)' }}>
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {t(mode === 'export' ? 'settings.portable.exportTitle' : 'settings.portable.importTitle')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('common.cancel')}
              >
                <Icon name="close" size={16} />
              </button>
            </Dialog.Close>
          </div>

          <div className="portable-body">
            {mode === 'export' ? (
              <>
                <p className="help">{t('settings.portable.exportHelp')}</p>

                <div className="portable-list" role="group" aria-label={t('settings.portable.pickAccounts')}>
                  {accounts.map((a) => (
                    <PickRow
                      key={a.id}
                      checked={picked.has(a.id)}
                      onChange={(v) => setPicked((s) => toggle(s, a.id, v))}
                      title={a.name || a.email}
                      sub={a.email}
                    />
                  ))}
                  {accounts.length === 0 && (
                    <div className="settings-empty">{t('settings.portable.noAccounts')}</div>
                  )}
                </div>

                <label className="portable-check">
                  <input
                    type="checkbox"
                    checked={withPasswords}
                    onChange={(e) => {
                      setWithPasswords(e.target.checked)
                      // 取消勾选时把确认也撤回：否则再勾一次就直接可导出了，
                      // 警告等于只看过一次。
                      if (!e.target.checked) setRiskAck(false)
                    }}
                  />
                  <span>{t('settings.portable.includePasswords')}</span>
                </label>

                {/* ⚠ 这段警告是这个功能的安全前提，不是装饰。
                    库里的密码是 AES 密文，而密钥随部署走——换台机器就解不开，
                    所以导出必须先解密，落到文件里就是明文。用户必须在勾选的
                    同一屏看见这件事，并明确确认。 */}
                {withPasswords && (
                  <div className="portable-warn" role="alert">
                    <div className="portable-warn-title">
                      <Icon name="shield" size={14} />
                      {t('settings.portable.plaintextTitle')}
                    </div>
                    <p>{t('settings.portable.plaintextBody')}</p>
                    <label className="portable-check">
                      <input type="checkbox" checked={riskAck} onChange={(e) => setRiskAck(e.target.checked)} />
                      <span>{t('settings.portable.plaintextAck')}</span>
                    </label>
                  </div>
                )}
              </>
            ) : (
              <>
                <p className="help">{t('settings.portable.importHelp')}</p>

                <input
                  ref={fileRef}
                  type="file"
                  accept="application/json,.json"
                  className="sr-only"
                  onChange={(e) => {
                    const f = e.target.files?.[0]
                    if (f) void handleFile(f)
                    // 复位 value：同一个文件连选两次也要能触发 change
                    e.target.value = ''
                  }}
                />
                <button type="button" className="pill-btn" onClick={() => fileRef.current?.click()}>
                  <Icon name="folder" size={12} />
                  {t('settings.portable.chooseFile')}
                </button>

                {fileError && (
                  <div className="portable-error" role="alert">
                    {fileError}
                  </div>
                )}

                {bundle && !result && (
                  <>
                    <div className="portable-list" role="group" aria-label={t('settings.portable.pickAccounts')}>
                      {bundle.accounts.map((a) => {
                        const key = a.email.toLowerCase()
                        const dup = existingEmails.has(key)
                        return (
                          <PickRow
                            key={key}
                            checked={pickedEmails.has(key)}
                            onChange={(v) => setPickedEmails((s) => toggle(s, key, v))}
                            title={a.name || a.email}
                            sub={`${a.email} · ${a.imap_host}:${a.imap_port}`}
                            tag={
                              <span className="portable-tag">
                                {dup ? t('settings.portable.tagExists') : t('settings.portable.tagNew')}
                                {a.password != null && ` · ${t('settings.portable.tagHasPassword')}`}
                              </span>
                            }
                          />
                        )
                      })}
                    </div>

                    {/* 只有真的存在同名账户时才问怎么处理——没有冲突却先让用户
                        做一道选择题，是纯粹的噪音。 */}
                    {bundle.accounts.some((a) => existingEmails.has(a.email.toLowerCase())) && (
                      <fieldset className="portable-fieldset">
                        <legend>{t('settings.portable.conflictTitle')}</legend>
                        <label className="portable-check">
                          <input type="radio" name="conflict" checked={conflict === 'skip'} onChange={() => setConflict('skip')} />
                          <span>{t('settings.portable.conflictSkip')}</span>
                        </label>
                        <label className="portable-check">
                          <input type="radio" name="conflict" checked={conflict === 'overwrite'} onChange={() => setConflict('overwrite')} />
                          <span>{t('settings.portable.conflictOverwrite')}</span>
                        </label>
                      </fieldset>
                    )}
                  </>
                )}

                {result && (
                  <div className="portable-result">
                    <div className="portable-result-sum">
                      {t('settings.portable.resultSummary', {
                        created: result.created,
                        updated: result.updated,
                        skipped: result.skipped,
                        failed: result.failed,
                      })}
                    </div>
                    {result.outcomes.map((o) => (
                      <div key={o.email} className={'portable-outcome' + (o.action === 'failed' ? ' failed' : '')}>
                        <span>{o.email}</span>
                        <span>{o.error ? o.error : t(`settings.portable.action.${o.action}`)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </>
            )}
          </div>

          <div className="flex items-center justify-end gap-2 px-6 py-4" style={{ borderTop: '1px solid var(--rule)' }}>
            <Dialog.Close asChild>
              <Button variant="outline">{result ? t('common.close') : t('common.cancel')}</Button>
            </Dialog.Close>
            {mode === 'export' ? (
              <Button onClick={() => void handleExport()} disabled={exportBlocked}>
                {t('settings.portable.doExport')}
              </Button>
            ) : (
              !result && (
                <Button onClick={() => void handleImport()} disabled={!bundle || pickedEmails.size === 0 || importing}>
                  {t('settings.portable.doImport')}
                </Button>
              )
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

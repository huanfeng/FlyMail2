// 设置 → 签名：每个账户一份签名，可分别决定新建信和回复要不要自动带上。
//
// 编辑器复用撰写器那一套（同一个 RichEditor）。理由不只是省事：签名在写信时会被
// 原样插进正文，所见即所得只有在"编辑签名"和"写信"用同一个 schema 时才成立——
// 换一个编辑器，就意味着签名里能编出撰写器表达不了的结构。
//
// 签名里的图片存成 data: URI（和草稿同一套逻辑），发送时再由 prepareInlineForSend
// 转成 cid: 内联附件。存 blob URL 是不行的：那东西换个页面就失效。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { RichEditor } from '@/components/mail/composer/RichEditor'
import type { RichEditorHandle } from '@/components/mail/composer/RichEditor'
import { useToast } from '@/components/ui/Toast'
import { useInlineImages } from '@/hooks/useInlineImages'
import { apiErrorMessage } from '@/lib/api'
import { prepareInlineForDraft } from '@/lib/inline-images'
import { useAccounts, useSaveSignature, useSignature } from '@/lib/queries'

/**
 * 单账户的签名编辑器。
 *
 * 由外层用 `key={accountId}` 挂载——换账户就整块重挂，本地编辑态天然清空。
 * 这比"用 effect 把查询结果同步进 state"可靠：后者要处理"数据还没到"
 * 和"用户已经改了但请求刚返回"两种时序，而重挂载一种都不用处理。
 */
function SignatureEditor({ accountId }: { accountId: number }) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const sigQuery = useSignature(accountId)
  const saveSignature = useSaveSignature()
  const editorRef = React.useRef<RichEditorHandle>(null)
  const inline = useInlineImages()

  // 开关：没动过就跟随服务端值，动过之后以本地为准
  const [overrides, setOverrides] = React.useState<{ onNew?: boolean; onReply?: boolean }>({})
  const [error, setError] = React.useState<string | null>(null)

  const sig = sigQuery.data
  const useOnNew = overrides.onNew ?? sig?.use_on_new ?? false
  const useOnReply = overrides.onReply ?? sig?.use_on_reply ?? false

  React.useEffect(() => () => { inline.reset() }, [inline])

  async function handleSave() {
    setError(null)
    const raw = editorRef.current?.getHTML() ?? ''
    const { html, truncated } = await prepareInlineForDraft(raw, (src) => inline.toDataUri(src))
    if (truncated) toast(t('settings.signature.tooLarge'))
    saveSignature.mutate(
      { accountId, input: { body_html: html, use_on_new: useOnNew, use_on_reply: useOnReply } },
      {
        onSuccess: () => toast(t('settings.signature.saved')),
        onError: (err) => setError(apiErrorMessage(err, t('settings.signature.saveFailed'))),
      },
    )
  }

  return (
    <>
      {error && (
        <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginBottom: 8 }}>
          {error}
        </div>
      )}

      <div className="signature-editor">
        <RichEditor
          ref={editorRef}
          initialHtml={sig?.body_html ?? ''}
          // 查询落定时翻一次，把已加载的签名灌进编辑器；之后后台刷新不再冲掉用户的编辑
          resetKey={`${accountId}:${sigQuery.isSuccess ? 'loaded' : 'pending'}`}
          minHeight={160}
          registerInlineImage={(file) => inline.register(file)}
        />
      </div>

      <label className="settings-check" style={{ marginTop: 12 }}>
        <input
          type="checkbox"
          checked={useOnNew}
          onChange={(e) => setOverrides((p) => ({ ...p, onNew: e.target.checked }))}
        />
        <span>{t('settings.signature.useOnNew')}</span>
      </label>
      <label className="settings-check">
        <input
          type="checkbox"
          checked={useOnReply}
          onChange={(e) => setOverrides((p) => ({ ...p, onReply: e.target.checked }))}
        />
        <span>{t('settings.signature.useOnReply')}</span>
      </label>

      <div style={{ marginTop: 14 }}>
        <button
          type="button"
          className="pill-btn primary"
          disabled={saveSignature.isPending}
          onClick={() => { void handleSave() }}
        >
          {saveSignature.isPending ? t('settings.signature.saving') : t('settings.signature.save')}
        </button>
      </div>
    </>
  )
}

export function SignatureSection() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const [selected, setSelected] = React.useState<number | null>(null)

  const accountId = selected ?? accounts[0]?.id ?? null

  return (
    <div className="settings-block">
      <h3>{t('settings.signature.title')}</h3>
      <p className="help">{t('settings.signature.help')}</p>

      {accounts.length === 0 ? (
        <div className="settings-empty">{t('settings.signature.noAccount')}</div>
      ) : (
        <>
          {accounts.length > 1 && (
            <div className="settings-field" style={{ marginTop: 12 }}>
              <label>{t('settings.signature.account')}</label>
              <select
                value={accountId ?? ''}
                onChange={(e) => setSelected(Number(e.target.value))}
              >
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.name} — {a.email}</option>
                ))}
              </select>
            </div>
          )}
          {accountId != null && <SignatureEditor key={accountId} accountId={accountId} />}
        </>
      )}
    </div>
  )
}

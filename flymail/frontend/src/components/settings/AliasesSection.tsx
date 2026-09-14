// 设置 → 发件别名：一个账户可以配多个发信地址。
//
// 别名只改 `From:` 头，SMTP 信封发件人仍是账户主地址——这不是偷懒，是唯一能投递出去的
// 组合：多数服务器只允许信封发件人等于认证账户，而 SPF 校验的正是信封域。
// 同域别名（sales@x.com 之于 admin@x.com）因此能正常投递；跨域别名的 DMARC 对齐
// 仍会失败，那是协议约束，界面上不作任何承诺（见 docs/flymail/m13-composer.md）。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { apiErrorMessage } from '@/lib/api'
import {
  useAccounts,
  useAliases,
  useCreateAlias,
  useDeleteAlias,
  useUpdateAlias,
} from '@/lib/queries'
import type { Alias } from '@/lib/types'

/** 够用的地址粗筛：真正的校验在后端，这里只挡住明显打错的输入 */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

interface DraftAlias {
  /** null = 新增 */
  id: number | null
  email: string
  displayName: string
  isDefault: boolean
}

function emptyDraft(): DraftAlias {
  return { id: null, email: '', displayName: '', isDefault: false }
}

/**
 * 单账户的别名列表 + 增改表单。
 *
 * 外层用 `key={accountId}` 挂载：换账户整块重挂，正在编辑的表单态自然清空，
 * 不需要一个 effect 去追"账户变了要重置什么"。
 */
function AliasList({ accountId }: { accountId: number }) {
  const { t } = useTranslation()
  const confirm = useConfirm()
  const { data: aliases = [] } = useAliases(accountId)
  const createAlias = useCreateAlias()
  const updateAlias = useUpdateAlias()
  const deleteAlias = useDeleteAlias()

  const [draft, setDraft] = React.useState<DraftAlias | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  const busy = createAlias.isPending || updateAlias.isPending || deleteAlias.isPending

  function openAdd() {
    setError(null)
    setDraft(emptyDraft())
  }

  function openEdit(a: Alias) {
    setError(null)
    setDraft({ id: a.id, email: a.email, displayName: a.display_name ?? '', isDefault: a.is_default })
  }

  function handleSave() {
    if (!draft) return
    const email = draft.email.trim()
    if (!EMAIL_RE.test(email)) {
      setError(t('settings.aliases.emailInvalid'))
      return
    }
    setError(null)
    const input = { email, display_name: draft.displayName.trim(), is_default: draft.isDefault }
    const onDone = {
      onSuccess: () => setDraft(null),
      onError: (err: unknown) => setError(apiErrorMessage(err, t('settings.aliases.saveFailed'))),
    }
    if (draft.id == null) createAlias.mutate({ accountId, input }, onDone)
    else updateAlias.mutate({ accountId, aliasId: draft.id, input }, onDone)
  }

  async function handleDelete(a: Alias) {
    const ok = await confirm({
      title: t('settings.aliases.removeConfirm', { email: a.email }),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok) return
    setError(null)
    deleteAlias.mutate(
      { accountId, aliasId: a.id },
      { onError: (err) => setError(apiErrorMessage(err, t('settings.aliases.removeFailed'))) },
    )
  }

  return (
    <>
      {error && (
        <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginBottom: 8 }}>
          {error}
        </div>
      )}

      <div style={{ marginTop: 12 }}>
        {aliases.length === 0 ? (
          <div className="settings-empty">{t('settings.aliases.none')}</div>
        ) : (
          aliases.map((a) => (
            <div key={a.id} className="settings-list-row">
              <Icon name="mail" size={13} />
              <span className="slr-grow slr-mono">
                {a.display_name ? `${a.display_name} <${a.email}>` : a.email}
              </span>
              {a.is_default && (
                <span className="chip slr-fixed" style={{ fontSize: 11, padding: '1px 7px' }}>
                  {t('settings.aliases.defaultBadge')}
                </span>
              )}
              <button
                type="button"
                className="icon-btn slr-fixed"
                title={t('settings.aliases.edit')}
                aria-label={t('settings.aliases.edit')}
                disabled={busy}
                onClick={() => openEdit(a)}
              >
                <Icon name="settings" size={13} />
              </button>
              <button
                type="button"
                className="icon-btn slr-fixed"
                title={t('settings.aliases.remove')}
                aria-label={t('settings.aliases.remove')}
                disabled={busy}
                onClick={() => handleDelete(a)}
                style={{ color: 'var(--destructive)' }}
              >
                <Icon name="trash" size={13} />
              </button>
            </div>
          ))
        )}
      </div>

      {draft ? (
        <div className="alias-form">
          <div className="settings-field">
            <label>{t('settings.aliases.email')}</label>
            <input
              value={draft.email}
              placeholder="sales@example.com"
              onChange={(e) => setDraft({ ...draft, email: e.target.value })}
            />
          </div>
          <div className="settings-field">
            <label>{t('settings.aliases.displayName')}</label>
            <input
              value={draft.displayName}
              placeholder={t('settings.aliases.displayNamePh')}
              onChange={(e) => setDraft({ ...draft, displayName: e.target.value })}
            />
          </div>
          <label className="settings-check">
            <input
              type="checkbox"
              checked={draft.isDefault}
              onChange={(e) => setDraft({ ...draft, isDefault: e.target.checked })}
            />
            <span>{t('settings.aliases.isDefault')}</span>
          </label>
          <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
            <button type="button" className="pill-btn primary" disabled={busy} onClick={handleSave}>
              {t('settings.aliases.save')}
            </button>
            <button type="button" className="pill-btn" disabled={busy} onClick={() => setDraft(null)}>
              {t('settings.aliases.cancel')}
            </button>
          </div>
        </div>
      ) : (
        <button type="button" className="pill-btn" style={{ marginTop: 14 }} onClick={openAdd}>
          <Icon name="plus" size={12} />
          {t('settings.aliases.add')}
        </button>
      )}
    </>
  )
}

export function AliasesSection() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const [selected, setSelected] = React.useState<number | null>(null)

  const accountId = selected ?? accounts[0]?.id ?? null

  return (
    <div className="settings-block">
      <h3>{t('settings.aliases.title')}</h3>
      <p className="help">{t('settings.aliases.help')}</p>

      {accounts.length === 0 ? (
        <div className="settings-empty">{t('settings.aliases.noAccount')}</div>
      ) : (
        <>
          {accounts.length > 1 && (
            <div className="settings-field">
              <label>{t('settings.aliases.account')}</label>
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
          {accountId != null && <AliasList key={accountId} accountId={accountId} />}
        </>
      )}
    </div>
  )
}

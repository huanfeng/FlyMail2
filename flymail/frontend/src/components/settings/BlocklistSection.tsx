// 设置 → 黑名单：命中的发件人在入库后直接被移走（垃圾箱，没有则回收站），不再走后续规则。
// 只有「加一条 / 删一条」两个动作，不值得再开一个对话框，输入框直接内嵌在列表上方。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { Input } from '@/components/ui/input'
import { useToast } from '@/components/ui/Toast'
import { apiErrorMessage } from '@/lib/api'
import { useAddBlock, useBlocklist, useDeleteBlock } from '@/lib/queries'
import { isValidBlockPattern, normalizeBlockPattern } from '@/lib/rules'
import type { BlockEntry } from '@/lib/types'

export function BlocklistSection() {
  const { t } = useTranslation()
  const confirm = useConfirm()
  const { toast } = useToast()
  const { data: entries = [] } = useBlocklist()
  const addBlock = useAddBlock()
  const deleteBlock = useDeleteBlock()

  const [pattern, setPattern] = React.useState('')
  const [note, setNote] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)

  function handleAdd() {
    // 归一化后再校验：用户粘进来的往往是 `Alice <a@x.com>` 或 `@example.com`
    const normalized = normalizeBlockPattern(pattern)
    if (!isValidBlockPattern(normalized)) {
      setError(t('settings.blocklist.invalid'))
      return
    }
    setError(null)
    addBlock.mutate(
      { pattern: normalized, note: note.trim() || undefined },
      {
        onSuccess: (res) => {
          setPattern('')
          setNote('')
          toast(res.existed ? t('settings.blocklist.exists', { pattern: normalized }) : t('settings.blocklist.added', { pattern: normalized }))
        },
        // 后端会拒绝一些前端看不出问题的输入（比如本地账户自己的邮箱），照它的文案说
        onError: (e) => setError(apiErrorMessage(e, t('settings.blocklist.addFailed'))),
      },
    )
  }

  async function handleDelete(e: BlockEntry) {
    const ok = await confirm({
      title: t('settings.blocklist.deleteConfirm', { pattern: e.pattern }),
      confirmLabel: t('common.confirm'),
      danger: true,
    })
    if (!ok) return
    deleteBlock.mutate(e.id, {
      onError: (err) => setError(apiErrorMessage(err, t('settings.blocklist.deleteFailed'))),
    })
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.blocklist.title')}</h3>
      <p className="help">{t('settings.blocklist.help')}</p>

      {/* 添加行 */}
      <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
        <Input
          style={{ flex: '1 1 220px', minWidth: 0 }}
          value={pattern}
          onChange={(e) => setPattern(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') handleAdd() }}
          placeholder={t('settings.blocklist.patternPlaceholder')}
          aria-label={t('settings.blocklist.pattern')}
        />
        <Input
          style={{ flex: '1 1 160px', minWidth: 0 }}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') handleAdd() }}
          placeholder={t('settings.blocklist.notePlaceholder')}
          aria-label={t('settings.blocklist.note')}
        />
        <button type="button" className="pill-btn" onClick={handleAdd} disabled={addBlock.isPending} style={{ flexShrink: 0, height: 36 }}>
          <Icon name="plus" size={12} />{t('settings.blocklist.add')}
        </button>
      </div>
      {error && <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginTop: 8 }}>{error}</div>}

      {/* 列表 */}
      <div style={{ marginTop: 14 }}>
        {entries.length === 0 ? (
          <div className="settings-empty">{t('settings.blocklist.none')}</div>
        ) : (
          entries.map((e) => (
            <div key={e.id} className="settings-list-row">
              <Icon name="shield" size={13} />
              <span className="slr-fixed slr-mono">{e.pattern}</span>
              <span className="slr-grow slr-dim">{e.note}</span>
              <span className="slr-fixed slr-mono slr-dim">{new Date(e.created_at).toLocaleDateString()}</span>
              <button
                type="button"
                className="icon-btn slr-fixed"
                title={t('settings.blocklist.remove')}
                aria-label={t('settings.blocklist.remove')}
                onClick={() => handleDelete(e)}
                style={{ color: 'var(--destructive)' }}
              >
                <Icon name="trash" size={13} />
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

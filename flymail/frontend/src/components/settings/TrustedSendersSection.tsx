// 设置 → 隐私 → 发件人信任名单：名单里的地址，其邮件正文中的远程图片自动加载。
//
// 这里只有「看」和「删」：条目由阅读界面的「总是显示此发件人的图片」产生。
// 不做手工添加输入框是有意的——凭空往名单里敲一个地址，用户既看不到那封邮件、
// 也说不清自己在信任什么；信任应该发生在看到具体一封信的那个瞬间。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useConfirm } from '@/components/ui/Confirm'
import { Icon } from '@/components/ui/Icon'
import { apiErrorMessage } from '@/lib/api'
import { useDeleteTrustedSender, useTrustedSenders } from '@/lib/queries'
import type { TrustedSender } from '@/lib/types'

export function TrustedSendersSection() {
  const { t } = useTranslation()
  const confirm = useConfirm()
  const { data: senders = [] } = useTrustedSenders()
  const deleteSender = useDeleteTrustedSender()
  const [error, setError] = React.useState<string | null>(null)

  async function handleDelete(s: TrustedSender) {
    const ok = await confirm({
      title: t('settings.privacy.trusted.deleteConfirm', { address: s.address }),
      confirmLabel: t('common.confirm'),
      danger: true,
    })
    if (!ok) return
    setError(null)
    deleteSender.mutate(s.id, {
      onError: (err) => setError(apiErrorMessage(err, t('settings.privacy.trusted.deleteFailed'))),
    })
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.privacy.trusted.title')}</h3>
      <p className="help">{t('settings.privacy.trusted.help')}</p>

      {error && (
        <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginBottom: 8 }}>
          {error}
        </div>
      )}

      <div style={{ marginTop: 14 }}>
        {senders.length === 0 ? (
          <div className="settings-empty">{t('settings.privacy.trusted.none')}</div>
        ) : (
          senders.map((s) => (
            <div key={s.id} className="settings-list-row">
              <Icon name="mail" size={13} />
              <span className="slr-grow slr-mono">{s.address}</span>
              <span className="slr-fixed slr-mono slr-dim">
                {new Date(s.created_at).toLocaleDateString()}
              </span>
              <button
                type="button"
                className="icon-btn slr-fixed"
                title={t('settings.privacy.trusted.remove')}
                aria-label={t('settings.privacy.trusted.remove')}
                onClick={() => handleDelete(s)}
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

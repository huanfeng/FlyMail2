// 设置 → 服务器：这台部署的对外地址与维护操作。
//
// 对外访问地址此前放在通知页，但它不只服务通知：OAuth 回调地址也由它拼出来。
// 放在「系统」组下，配置一次就不用再管。
// 重建索引/线程是偶尔才用的维护动作，此前夹在同步偏好的保存按钮下面，
// 容易被当成「保存」的一部分。

import { useTranslation } from 'react-i18next'
import { useToast } from '@/components/ui/Toast'
import { BaseUrlSection } from '@/components/settings/BaseUrlSection'
import { useRebuildThreads, useReindexSearch } from '@/lib/queries'
import { Row } from '../controls'

export function ServerSection() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const reindex = useReindexSearch()
  const rebuildThreads = useRebuildThreads()

  return (
    <>
      <BaseUrlSection Row={Row} />

      {/* 重建搜索索引：全文索引与邮件表失配时的兜底 */}
      <div className="settings-block">
        <h3>{t('settings.server.maintenance')}</h3>
        <p className="help">{t('settings.server.maintenanceHelp')}</p>
        <Row label={t('settings.mail.reindex')} help={t('settings.mail.reindexHint')}>
          <button
            type="button"
            className="pill-btn"
            onClick={() => {
              reindex.mutate(undefined, {
                onSuccess: () => toast(t('settings.mail.reindexDone')),
                onError: () => toast(t('settings.mail.reindexFailed')),
              })
            }}
            disabled={reindex.isPending}
          >
            {reindex.isPending ? t('settings.mail.reindexing') : t('settings.mail.reindexAction')}
          </button>
        </Row>

        {/* 重建会话归属：老库里的邮件没有 In-Reply-To/References 头，
            只有跑一趟按主题兜底的重放才能把它们并成会话。 */}
        <Row label={t('settings.mail.rebuildThreads')} help={t('settings.mail.rebuildThreadsHint')}>
          <button
            type="button"
            className="pill-btn"
            onClick={() => {
              rebuildThreads.mutate(undefined, {
                onSuccess: (n) => toast(t('settings.mail.rebuildThreadsDone', { count: n })),
                onError: () => toast(t('settings.mail.rebuildThreadsFailed')),
              })
            }}
            disabled={rebuildThreads.isPending}
          >
            {rebuildThreads.isPending
              ? t('settings.mail.rebuildingThreads')
              : t('settings.mail.rebuildThreadsAction')}
          </button>
        </Row>
      </div>
    </>
  )
}

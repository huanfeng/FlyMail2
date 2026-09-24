// 设置 → 阅读：会话视图 + 阅读隐私（远程图片、深色正文、发件人信任名单）。
//
// 这几项都是「邮件打开时长什么样」，并且都是改完立即生效的本地偏好，
// 放在一页里用户不必再猜哪个要点保存。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { TrustedSendersSection } from '@/components/settings/TrustedSendersSection'
import {
  getDarkBody,
  getRemoteImageDefault,
  setDarkBody,
  setRemoteImageDefault,
} from '@/lib/privacy-prefs'
import { Row, Toggle } from '../controls'

// ════════════════════════════════════════════════════════
// 子组件：隐私分区（M12）
// ════════════════════════════════════════════════════════

/**
 * 阅读隐私：远程图片默认开关 + 发件人信任名单。
 *
 * 开关从「通用」搬到这里而不是两处都放：它存在 localStorage 里，
 * 两个入口各自持一份 state 早晚会对不上，而隐私开关显示错值比不显示更糟。
 */
export interface ReadingSectionProps {
  conversationView: boolean
  onChangeConversationView: (on: boolean) => void
}

export function ReadingSection({ conversationView, onChangeConversationView }: ReadingSectionProps) {
  const { t } = useTranslation()
  const [loadRemoteImages, setLoadRemoteImages] = React.useState<boolean>(() =>
    getRemoteImageDefault(),
  )
  const [darkBody, setDarkBodyState] = React.useState<boolean>(() => getDarkBody())

  function handleRemoteImages(next: boolean) {
    setLoadRemoteImages(next)
    // 写完就结束：开关是可订阅的（见 privacy-prefs），已挂载的 useMessageDetail
    // 会因此重新渲染、把 remote 换进 query key，新 key 自然去取新口径的正文。
    // ⚙ 不能在这里 invalidate ['message']：那一瞬间阅读器还没重渲染，失效的是旧 key，
    // 结果是按旧口径白白多打一趟网络。
    setRemoteImageDefault(next)
  }

  function handleDarkBody(next: boolean) {
    setDarkBodyState(next)
    // 与上面同理：开关可订阅，已挂载的正文组件会重新渲染并重建 iframe 文档。
    setDarkBody(next)
  }

  return (
    <>
      {/* 会话视图：纯前端偏好，改完立即生效 */}
      <div className="settings-block">
        <h3>{t('settings.reading.viewTitle')}</h3>
        <Row
          label={t('settings.mail.conversationView')}
          help={t('settings.mail.conversationViewHint')}
        >
          <Toggle on={conversationView} onChange={onChangeConversationView} />
        </Row>
      </div>

      <div className="settings-block">
        <h3>{t('settings.privacy.reading')}</h3>
        <Row label={t('settings.privacy.remoteImages')} help={t('settings.privacy.remoteImagesHint')}>
          <Toggle on={loadRemoteImages} onChange={handleRemoteImages} />
        </Row>
        <Row label={t('settings.privacy.darkBody')} help={t('settings.privacy.darkBodyHint')}>
          <Toggle on={darkBody} onChange={handleDarkBody} />
        </Row>
      </div>

      <TrustedSendersSection />
    </>
  )
}

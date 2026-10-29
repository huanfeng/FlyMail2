// 设置 → 阅读里的正文预览：用阅读区同一个 buildFrameDocument 与 sandbox 渲染一段示例邮件，
// 偏好改动即时反映。不注入引导脚本（nonce/token 为 null）：示例内容固定，不需要自适应高度。
import { useTranslation } from 'react-i18next'
import { buildFrameDocument, bodyStyleFrom, MAIL_FRAME_SANDBOX } from '@/lib/mail-frame'
import type { Appearance } from '@/lib/appearance-prefs'

export function BodyPreview({ appearance }: { appearance: Appearance }) {
  const { t } = useTranslation()
  const html =
    `<p>${t('settings.reading.pvGreeting')}</p>` +
    `<p>${t('settings.reading.pvBody1')}</p>` +
    `<p>${t('settings.reading.pvBody2')}</p>` +
    `<p style="color:#57606a">— ${t('settings.reading.pvSign')}</p>`
  return (
    <div className="body-preview">
      <iframe
        title={t('settings.reading.previewTitle')}
        sandbox={MAIL_FRAME_SANDBOX}
        srcDoc={buildFrameDocument({
          html, allowRemote: false, foldQuote: false, quoteHideCss: '',
          nonce: null, token: null, bodyStyle: bodyStyleFrom(appearance),
        })}
      />
    </div>
  )
}

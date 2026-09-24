// 设置 → 通知：这台设备上的提醒 + 外发推送（正文长度、渠道）。

import { BrowserNotifySection } from '@/components/settings/BrowserNotifySection'
import { NotifyBodySection } from '@/components/settings/NotifyBodySection'
import { NotifyChannelsSection } from '@/components/settings/NotifyChannelsSection'
import { Row, Toggle } from '../controls'

export function NotifySection() {
  return (
    <>
      {/* 这台设备上的提醒排在外发渠道之前：多数人要的是「让这个浏览器
          提醒我」，而不是先去配一个 webhook */}
      <BrowserNotifySection Row={Row} Toggle={Toggle} />
      {/* 长度上限排在渠道之前：它是所有外发渠道共用的排版设置，
          而渠道列表是一串条目，夹在中间会像是某个渠道的属性 */}
      <NotifyBodySection Row={Row} />
      <NotifyChannelsSection />
    </>
  )
}

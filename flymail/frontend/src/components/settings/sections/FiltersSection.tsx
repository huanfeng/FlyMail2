// 设置 → 过滤：收信规则 + 黑名单。
//
// 黑名单本质上就是一条「删除/丢进垃圾箱」的规则，用户找「怎么挡掉这个发件人」时
// 会先想到规则，两者放在一页。

import { BlocklistSection } from '@/components/settings/BlocklistSection'
import { RulesSection } from '@/components/settings/RulesSection'

export function FiltersSection() {
  return (
    <>
      <RulesSection />
      <BlocklistSection />
    </>
  )
}

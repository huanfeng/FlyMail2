// 设置 → 撰写：签名 + 发件别名。
//
// 两者都是「发出去的信带着什么身份」，此前是导航里相邻的两项。

import { AliasesSection } from '@/components/settings/AliasesSection'
import { SignatureSection } from '@/components/settings/SignatureSection'

export function ComposeSection() {
  return (
    <>
      <SignatureSection />
      <AliasesSection />
    </>
  )
}

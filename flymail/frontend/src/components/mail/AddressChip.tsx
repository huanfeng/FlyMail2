// 邮件头里的一个地址：可点开菜单做操作。
//
// ── 为什么不再显示「我」 ─────────────────────────────────────────────────────
//
// 原先收件人里凡是命中当前账户邮箱的都渲染成「我」。单账户下没问题，多账户下
// 就丢信息了：同一封邮件可能同时发给你的两三个邮箱，一律显示成「我」之后，
// 你看不出这封到底进了哪个信箱、该用哪个身份回。所以自己的地址也如实显示，
// 只在旁边加一个低调的标记表明「这是你的」。
//
// ── 为什么每个地址单独成块 ───────────────────────────────────────────────────
//
// 原先发件人是一段纯文本、收件人是一串用逗号拼起来的文本，对着某一个人想做点
// 什么（搜他的邮件、给他回信、把他拉黑）都无从下手——只能自己把地址选中复制，
// 再去别处操作。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { DropMenu } from '@/components/ui/DropMenu'
import type { CtxMenuItem } from '@/components/ui/ContextMenu'
import type { Address } from '@/lib/types'

export interface AddressChipProps {
  addr: Address
  /** 这个地址属于本地某个账户 */
  isSelf: boolean
  /** 发件人还是收件人：决定给哪些菜单项 */
  role: 'from' | 'to'
  /** 写邮件给这个地址 */
  onCompose: (email: string) => void
  /** 用这条查询去搜索（形如 `from:a@b.com`） */
  onSearch: (query: string) => void
  /** 把这个发件人加进信任名单（此后自动显示远程内容）；null 表示不提供该操作 */
  onTrust: ((email: string) => void) | null
  /** 屏蔽这个发件人；null 表示不可屏蔽（自己的地址、或格式不合法） */
  onBlock: ((email: string) => void) | null
}

export function AddressChip({ addr, isSelf, role, onCompose, onSearch, onTrust, onBlock }: AddressChipProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = React.useState(false)
  const email = addr.email.trim()
  if (email === '' && !addr.name) return null

  // 自己的地址显示邮箱本身：多账户下「我」说不清是哪个信箱（见文件头注释）
  const label = isSelf ? email || addr.name : addr.name || email

  async function copy() {
    try {
      await navigator.clipboard.writeText(email)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      /* 剪贴板不可用（非安全上下文 / 无权限）：不提示，菜单已经关了 */
    }
  }

  const items: CtxMenuItem[] = [
    { key: 'copy', label: t('addr.copy'), icon: 'tag', onSelect: () => void copy() },
    { key: 'compose', label: t('addr.composeTo'), icon: 'compose', onSelect: () => onCompose(email) },
    {
      key: 'search-from',
      label: t('addr.searchFrom'),
      icon: 'search',
      onSelect: () => onSearch(`from:${email}`),
    },
    {
      key: 'search-to',
      label: t('addr.searchTo'),
      icon: 'search',
      onSelect: () => onSearch(`to:${email}`),
    },
  ]
  // 信任与屏蔽只对「发件人」有意义：它们管的是「这个人发来的邮件」怎么处理，
  // 挂在收件人上会让人以为是在管发给他的邮件。
  if (role === 'from' && (onTrust || onBlock)) {
    items.push({ key: 'sep', separator: true })
    if (onTrust) {
      items.push({ key: 'trust', label: t('addr.trust'), icon: 'shield', onSelect: () => onTrust(email) })
    }
    if (onBlock) {
      items.push({
        key: 'block',
        label: t('addr.block'),
        icon: 'shield',
        destructive: true,
        onSelect: () => onBlock(email),
      })
    }
  }

  return (
    <DropMenu
      align="start"
      items={items}
      trigger={
        <button
          type="button"
          className={'addr-chip' + (isSelf ? ' is-self' : '')}
          // 完整地址放 title：显示名存在时 chip 上只有名字，光看不出是谁
          title={addr.name && email ? `${addr.name} <${email}>` : email}
          onClick={(e) => e.stopPropagation()}
        >
          <span className="addr-chip-text">{copied ? t('addr.copied') : label}</span>
          {isSelf && <span className="addr-chip-self">{t('addr.mine')}</span>}
        </button>
      }
    />
  )
}

// 搜索语法帮助浮层（搜索框右侧的 ? 按钮）。
//
// 后端支持 Gmail 风格限定符，但没有入口用户就不会知道它们存在。
// 这里用 radix Popover 而非 DropMenu：DropMenu 的条目只有一行标签，
// 而这里每条要「示例写法 + 一句说明」两列，且点了要把片段插回输入框。
//
// 点击一条 → 把示例前缀追加到搜索框并聚焦（由 MailList 的 onPick 落地），
// 用户接着敲取值即可，不必记拼写。

import { useState } from 'react'
import { Popover } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'

interface SyntaxEntry {
  /** i18n 子键，同时用作 React key */
  key: string
  /** 浮层里展示的示例写法 */
  token: string
  /**
   * 点击后追加到搜索框的片段；省略表示这是纯说明行（如「"短语"」）——
   * 插一个孤零零的引号对用户没有帮助。
   * 取值型限定符不带尾随空格，光标停在冒号后正好继续输入。
   */
  insert?: string
}

const ENTRIES: SyntaxEntry[] = [
  { key: 'from',       token: 'from:',            insert: 'from:' },
  { key: 'to',         token: 'to:',              insert: 'to:' },
  { key: 'subject',    token: 'subject:',         insert: 'subject:' },
  { key: 'attachment', token: 'has:attachment',   insert: 'has:attachment ' },
  { key: 'unread',     token: 'is:unread',        insert: 'is:unread ' },
  { key: 'starred',    token: 'is:starred',       insert: 'is:starred ' },
  { key: 'date',       token: 'before: / after:', insert: 'before:' },
  { key: 'folder',     token: 'in:',              insert: 'in:' },
  { key: 'account',    token: 'account:',         insert: 'account:' },
  { key: 'phrase',     token: '"…"' },
]

interface Props {
  /** 把一段语法片段追加进搜索框（调用方负责拼空格与聚焦） */
  onPick: (fragment: string) => void
}

export function SearchSyntaxHelp({ onPick }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="icon-btn mini"
          style={{ flex: '0 0 auto' }}
          title={t('list.syntax.title')}
          aria-label={t('list.syntax.title')}
        >
          <Icon name="help" size={13} />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="search-help"
          align="end"
          sideOffset={8}
          collisionPadding={12}
        >
          <div className="sh-title">{t('list.syntax.title')}</div>

          <div className="sh-rows">
            {ENTRIES.map((e) => {
              const insert = e.insert
              const desc = t(`list.syntax.${e.key}`)
              if (insert === undefined) {
                return (
                  <div key={e.key} className="sh-row static">
                    <code>{e.token}</code>
                    <span>{desc}</span>
                  </div>
                )
              }
              return (
                <button
                  key={e.key}
                  type="button"
                  className="sh-row"
                  onClick={() => { setOpen(false); onPick(insert) }}
                >
                  <code>{e.token}</code>
                  <span>{desc}</span>
                </button>
              )
            })}
          </div>

          <div className="sh-foot">{t('list.syntax.hint')}</div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

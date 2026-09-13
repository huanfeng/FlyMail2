import { describe, it, expect } from 'vitest'
import { folderLabel, formatAddresses, formatDate, senderInitial } from '@/lib/mail-format'
import type { Address, Folder } from '@/lib/types'

function addr(name: string, email: string): Address {
  return { name, email }
}

describe('formatAddresses', () => {
  it('name 存在时渲染成 name <email>', () => {
    expect(formatAddresses([addr('张三', 'a@b.com')], '', '我')).toBe('张三 <a@b.com>')
  })

  it('没有 name 时只渲染邮箱', () => {
    expect(formatAddresses([addr('', 'a@b.com')], '', '我')).toBe('a@b.com')
  })

  it('多个收件人用逗号连接', () => {
    const got = formatAddresses([addr('张三', 'a@b.com'), addr('', 'c@d.com')], '', '我')
    expect(got).toBe('张三 <a@b.com>, c@d.com')
  })

  // 归一化：邮箱本地部分理论上大小写敏感，但没有服务商真的这么用，
  // 按大小写敏感比会让「我」漏判，收件人里就会躺着一串自己的地址。
  it('本人地址大小写不同也识别为「我」', () => {
    expect(formatAddresses([addr('Me', 'ME@Example.COM')], 'me@example.com', '我')).toBe('我')
  })

  it('本人地址带首尾空白也识别为「我」', () => {
    expect(formatAddresses([addr('Me', '  me@example.com ')], ' ME@example.com  ', '我')).toBe('我')
  })

  it('selfAddr 为空串时不把任何人当成「我」', () => {
    expect(formatAddresses([addr('', 'a@b.com')], '   ', '我')).toBe('a@b.com')
  })

  it('本人与他人混排时只替换本人那一个', () => {
    const got = formatAddresses(
      [addr('张三', 'a@b.com'), addr('Me', 'me@x.com')],
      'me@x.com',
      '我',
    )
    expect(got).toBe('张三 <a@b.com>, 我')
  })

  it('空列表返回空串', () => {
    expect(formatAddresses([], 'me@x.com', '我')).toBe('')
  })
})

describe('senderInitial', () => {
  it('优先取显示名首字母并大写', () => {
    expect(senderInitial('zhang san', 'a@b.com')).toBe('Z')
  })

  it('没有显示名时取邮箱首字母', () => {
    expect(senderInitial('', 'alice@b.com')).toBe('A')
  })

  it('都为空时给问号', () => {
    expect(senderInitial('', '')).toBe('?')
  })

  it('忽略首尾空白', () => {
    expect(senderInitial('  张三 ', '')).toBe('张')
  })
})

describe('formatDate', () => {
  it('非法日期原样返回，不抛异常', () => {
    expect(formatDate('not-a-date')).toBe('Invalid Date')
  })

  it('合法日期能格式化出年份', () => {
    expect(formatDate('2026-09-07T10:00:00Z')).toContain('2026')
  })
})

/**
 * 文件夹显示名。
 *
 * 它此前是 ThreadReader.tsx 里的一个局部函数；单封阅读区要显示同一个标签时
 * 抽了出来。抽出来的理由不是复用省几行，而是**判据不能有两份**：
 * `type === 'custom'` 一旦分叉，同一个文件夹在会话视图和单封视图里会显示
 * 两个不同的名字，而那种不一致没人会当成 bug 报上来。
 */
describe('folderLabel', () => {
  const t = (k: string) => k
  function folder(id: number, path: string, displayName: string, type: string): Folder {
    return {
      id,
      account_id: 1,
      path,
      display_name: displayName,
      type,
      selectable: true,
      total_count: 0,
      unread_count: 0,
      sort_order: id,
    }
  }
  // 写全字段而不是 `as Folder` 断言：漏字段时 tsc 会报，而断言会把
  // 「服务端多给/少给一个字段」这类变化一并盖掉。
  const folders: Folder[] = [
    folder(1, 'INBOX', '收件箱', 'inbox'),
    folder(2, 'Work/2026', '2026 项目', 'custom'),
  ]

  it('系统文件夹走 i18n 键，不用服务器给的名字', () => {
    // 服务器给的是 IMAP 那边的原名（INBOX / Sent Items / 已发送…），
    // 各家服务商拼法不一，直接显示会让同一个概念在不同账户下叫不同名字。
    expect(folderLabel(folders, 1, t)).toBe('folder.inbox')
  })

  it('自定义文件夹用服务器给的显示名', () => {
    expect(folderLabel(folders, 2, t)).toBe('2026 项目')
  })

  it('文件夹还没加载到时返回 null，而不是空字符串', () => {
    // 调用方据此决定「不渲染这个标签」。返回 '' 的话会渲染出一个空标签，
    // 在会话折叠行里就是一块无缘无故的间距。
    expect(folderLabel(folders, 999, t)).toBeNull()
    expect(folderLabel([], 1, t)).toBeNull()
  })
})

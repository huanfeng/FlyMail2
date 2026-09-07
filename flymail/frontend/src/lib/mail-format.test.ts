import { describe, it, expect } from 'vitest'
import { formatAddresses, formatDate, senderInitial } from '@/lib/mail-format'
import type { Address } from '@/lib/types'

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

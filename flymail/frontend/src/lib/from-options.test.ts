import { describe, it, expect } from 'vitest'
import {
  buildFromOptions,
  defaultFromOption,
  findFromOption,
  pickFromOption,
} from '@/lib/from-options'
import type { Account, Alias } from '@/lib/types'

function acct(id: number, email: string, name = ''): Account {
  return {
    id,
    name,
    email,
    auth_type: 'password',
    imap_host: '', imap_port: 993, imap_security: 'ssl',
    smtp_host: '', smtp_port: 465, smtp_security: 'ssl',
    status: 'ok',
    enabled: true,
  }
}

function alias(id: number, accountId: number, email: string, isDefault = false, name = ''): Alias {
  return { id, account_id: accountId, email, display_name: name, is_default: isDefault }
}

const A = acct(1, 'admin@x.com', '小明')
const B = acct(2, 'me@y.com')

describe('buildFromOptions', () => {
  it('账户主地址排在该账户第一位', () => {
    const opts = buildFromOptions([A], { 1: [alias(10, 1, 'sales@x.com')] })
    expect(opts.map((o) => o.email)).toEqual(['admin@x.com', 'sales@x.com'])
    expect(opts[0].alias).toBe('')
    expect(opts[1].alias).toBe('sales@x.com')
  })

  it('label 有显示名时写成 名字 <地址>', () => {
    const opts = buildFromOptions([A], { 1: [alias(10, 1, 'sales@x.com', false, '销售')] })
    expect(opts[0].label).toBe('小明 <admin@x.com>')
    expect(opts[1].label).toBe('销售 <sales@x.com>')
  })

  it('与主地址重名的别名不重复出现', () => {
    const opts = buildFromOptions([A], { 1: [alias(10, 1, 'ADMIN@x.com')] })
    expect(opts).toHaveLength(1)
  })

  it('多账户按账户顺序铺开，key 不冲突', () => {
    const opts = buildFromOptions([A, B], { 1: [alias(10, 1, 'sales@x.com')], 2: [] })
    expect(opts.map((o) => o.key)).toEqual(['1|admin@x.com', '1|sales@x.com', '2|me@y.com'])
  })

  it('别名地址为空的脏数据被跳过', () => {
    const opts = buildFromOptions([A], { 1: [alias(10, 1, '')] })
    expect(opts).toHaveLength(1)
  })
})

describe('findFromOption', () => {
  const opts = buildFromOptions([A, B], { 1: [alias(10, 1, 'sales@x.com')], 2: [] })

  it('按账户 + 别名精确命中', () => {
    expect(findFromOption(opts, 1, 'sales@x.com')?.key).toBe('1|sales@x.com')
  })

  it('空别名命中账户主地址', () => {
    expect(findFromOption(opts, 1, '')?.email).toBe('admin@x.com')
  })

  it('别名大小写不敏感', () => {
    expect(findFromOption(opts, 1, 'SALES@X.COM')?.key).toBe('1|sales@x.com')
  })

  it('别名已被删除时返回 null', () => {
    expect(findFromOption(opts, 1, 'gone@x.com')).toBeNull()
  })

  it('accountId 为空返回 null', () => {
    expect(findFromOption(opts, null, '')).toBeNull()
  })
})

describe('defaultFromOption', () => {
  it('有 is_default 别名时选它', () => {
    const aliases = { 1: [alias(10, 1, 'sales@x.com'), alias(11, 1, 'ceo@x.com', true)] }
    const opts = buildFromOptions([A], aliases)
    expect(defaultFromOption(opts, 1, aliases)?.email).toBe('ceo@x.com')
  })

  it('没有默认别名时用主地址', () => {
    const aliases = { 1: [alias(10, 1, 'sales@x.com')] }
    const opts = buildFromOptions([A], aliases)
    expect(defaultFromOption(opts, 1, aliases)?.email).toBe('admin@x.com')
  })

  it('该账户没有任何选项时返回 null', () => {
    expect(defaultFromOption([], 1, {})).toBeNull()
  })
})

describe('pickFromOption', () => {
  const aliases = { 1: [alias(10, 1, 'sales@x.com'), alias(11, 1, 'ceo@x.com', true)] }
  const opts = buildFromOptions([A], aliases)

  it('草稿里存了别名就用它，哪怕账户有别的默认别名', () => {
    expect(pickFromOption(opts, 1, 'sales@x.com', aliases)?.email).toBe('sales@x.com')
  })

  it('草稿里存的是空串（明确选了主地址）就用主地址', () => {
    // 文档口径：from_alias 为空 = 用账户主地址
    expect(pickFromOption(opts, 1, '', aliases)?.email).toBe('admin@x.com')
  })

  it('新建撰写（无别名信息）走账户默认别名', () => {
    expect(pickFromOption(opts, 1, undefined, aliases)?.email).toBe('ceo@x.com')
  })

  it('草稿里的别名已被删除时退回默认项，而不是留空发不出去', () => {
    expect(pickFromOption(opts, 1, 'gone@x.com', aliases)?.email).toBe('ceo@x.com')
  })

  it('accountId 为空时退回列表首项', () => {
    expect(pickFromOption(opts, null, undefined, aliases)?.email).toBe('admin@x.com')
  })

  it('没有任何账户时返回 null', () => {
    expect(pickFromOption([], 1, undefined, {})).toBeNull()
  })
})

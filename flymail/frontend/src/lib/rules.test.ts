import { describe, it, expect } from 'vitest'
import {
  isCompilableRegex,
  isKnownActionType,
  isValidBlockPattern,
  moveArrayItem,
  normalizeAction,
  normalizeBlockPattern,
  normalizeCondition,
  opsForField,
  parseRunAction,
  splitMoveNote,
  validateRuleInput,
} from '@/lib/rules'
import type { RuleInput } from '@/lib/types'

/** 造一条合法规则，测试里只覆盖要验证的那一项 */
function makeRule(patch: Partial<RuleInput> = {}): RuleInput {
  return {
    name: '广告归档',
    enabled: true,
    account_id: 0,
    match: 'all',
    conditions: [{ field: 'subject', op: 'contains', value: '促销' }],
    actions: [{ type: 'mark_read', value: '' }],
    stop_processing: false,
    ...patch,
  }
}

describe('validateRuleInput', () => {
  it('完整规则通过校验', () => {
    expect(validateRuleInput(makeRule())).toBeNull()
  })

  it('名称为空白时报 name', () => {
    expect(validateRuleInput(makeRule({ name: '   ' }))?.code).toBe('name')
  })

  it('没有条件时报 conditions', () => {
    expect(validateRuleInput(makeRule({ conditions: [] }))?.code).toBe('conditions')
  })

  it('没有动作时报 actions', () => {
    expect(validateRuleInput(makeRule({ actions: [] }))?.code).toBe('actions')
  })

  it('文本条件值为空时报 value 并带出行号', () => {
    const err = validateRuleInput(
      makeRule({
        conditions: [
          { field: 'subject', op: 'contains', value: '促销' },
          { field: 'from', op: 'contains', value: '  ' },
        ],
      }),
    )
    expect(err?.code).toBe('value')
    expect(err?.index).toBe(1)
  })

  it('has_attachment 不需要填值', () => {
    expect(
      validateRuleInput(makeRule({ conditions: [{ field: 'has_attachment', op: 'equals', value: 'true' }] })),
    ).toBeNull()
  })

  it('正则语法错误时报 regex 并回带原始 pattern', () => {
    const err = validateRuleInput(
      makeRule({ conditions: [{ field: 'subject', op: 'regex', value: '([a-z' }] }),
    )
    expect(err?.code).toBe('regex')
    expect(err?.detail).toBe('([a-z')
  })

  it('Go 内联标志写法的正则通过（JS 不认 (?i)，需先翻译）', () => {
    expect(
      validateRuleInput(makeRule({ conditions: [{ field: 'body', op: 'regex', value: '(?i)invoice\\d+' }] })),
    ).toBeNull()
  })

  it('带作用域的标志组 (?i:...) 必须通过预校验（JS 同样不认，需先翻译）', () => {
    expect(
      validateRuleInput(makeRule({ conditions: [{ field: 'subject', op: 'regex', value: '(?i:invoice)\\s*\\d+' }] })),
    ).toBeNull()
  })

  it('move 动作没选目标文件夹时报 moveTarget', () => {
    const err = validateRuleInput(makeRule({ actions: [{ type: 'move', value: '' }] }))
    expect(err?.code).toBe('moveTarget')
    expect(err?.index).toBe(0)
  })

  it('move 动作选了目标就通过', () => {
    expect(validateRuleInput(makeRule({ actions: [{ type: 'move', value: '归档' }] }))).toBeNull()
  })
})

describe('isCompilableRegex', () => {
  it('接受普通正则', () => {
    expect(isCompilableRegex('^invoice-\\d{4}$')).toBe(true)
  })

  it('接受 Go 内联标志组', () => {
    expect(isCompilableRegex('(?i)hello')).toBe(true)
    expect(isCompilableRegex('(?is)a.b')).toBe(true)
    expect(isCompilableRegex('(?-s)a.b')).toBe(true)
  })

  it('接受带作用域的标志组 (?i:...)', () => {
    expect(isCompilableRegex('(?i:abc)def')).toBe(true)
    expect(isCompilableRegex('(?s:.)')).toBe(true)
    expect(isCompilableRegex('(?is:a.b)|(?i)c')).toBe(true)
  })

  it('放行 RE2 其实拒绝的写法，交给后端定夺', () => {
    // 反向引用与断言在 JS 合法、RE2 不支持：前端不武断拦下，界面提示也只说「可能」
    expect(isCompilableRegex('(\\w)\\1')).toBe(true)
    expect(isCompilableRegex('(?=foo)bar')).toBe(true)
    expect(isCompilableRegex('(?<=a)b')).toBe(true)
  })

  it('接受 Go 命名捕获 (?P<name>...)', () => {
    expect(isCompilableRegex('(?P<id>\\d+)')).toBe(true)
  })

  it('拒绝两边都不合法的写法', () => {
    expect(isCompilableRegex('([a-z')).toBe(false)
    expect(isCompilableRegex('a{2,1}')).toBe(false)
    expect(isCompilableRegex('*abc')).toBe(false)
  })
})

describe('normalizeCondition / normalizeAction', () => {
  it('切到 has_attachment 时运算符收窄为 equals，值兜底为 true', () => {
    expect(normalizeCondition({ field: 'has_attachment', op: 'contains', value: 'x' })).toEqual({
      field: 'has_attachment',
      op: 'equals',
      value: 'true',
    })
  })

  it('has_attachment 的 false 保持不变', () => {
    expect(normalizeCondition({ field: 'has_attachment', op: 'equals', value: 'false' }).value).toBe('false')
  })

  it('从布尔字段切回文本字段时清掉残留的哨兵值', () => {
    expect(normalizeCondition({ field: 'subject', op: 'equals', value: 'true' }).value).toBe('')
  })

  it('文本字段保留原值与原运算符', () => {
    expect(normalizeCondition({ field: 'from', op: 'ends_with', value: '@spam.io' })).toEqual({
      field: 'from',
      op: 'ends_with',
      value: '@spam.io',
    })
  })

  it('非 move 动作的取值被清空', () => {
    expect(normalizeAction({ type: 'star', value: '归档' })).toEqual({ type: 'star', value: '' })
  })

  it('move 动作保留取值', () => {
    expect(normalizeAction({ type: 'move', value: '归档' })).toEqual({ type: 'move', value: '归档' })
  })

  it('opsForField 对布尔字段只给 equals', () => {
    expect(opsForField('has_attachment')).toEqual(['equals'])
    expect(opsForField('subject').length).toBeGreaterThan(1)
  })
})

describe('parseRunAction', () => {
  it('单个无参动作', () => {
    expect(parseRunAction('mark_read')).toEqual([{ type: 'mark_read', value: '', raw: 'mark_read' }])
  })

  it('逗号分隔的多个动作', () => {
    expect(parseRunAction('mark_read,star').map((tk) => tk.type)).toEqual(['mark_read', 'star'])
  })

  it('带参数的 move 拆出目标文件夹名', () => {
    expect(parseRunAction('move:归档')).toEqual([{ type: 'move', value: '归档', raw: 'move:归档' }])
  })

  it('黑名单命中拆出地址', () => {
    expect(parseRunAction('block:spam.io')[0]).toEqual({ type: 'block', value: 'spam.io', raw: 'block:spam.io' })
  })

  it('混合串按顺序拆开并去掉多余空白', () => {
    expect(parseRunAction('mark_read, move:归档 , notify')).toEqual([
      { type: 'mark_read', value: '', raw: 'mark_read' },
      { type: 'move', value: '归档', raw: 'move:归档' },
      { type: 'notify', value: '', raw: 'notify' },
    ])
  })

  it('只按第一个冒号切分，值里的冒号原样保留', () => {
    expect(parseRunAction('move:INBOX:归档')[0].value).toBe('INBOX:归档')
  })

  it('文件夹名里的逗号不再被拆开', () => {
    // 逗号后面不是已知动作前缀，就不是分隔符
    expect(parseRunAction('move:发票, 收据')).toEqual([
      { type: 'move', value: '发票, 收据', raw: 'move:发票, 收据' },
    ])
    expect(parseRunAction('mark_read,move:发票, 收据').map((tk) => tk.value)).toEqual(['', '发票, 收据'])
  })

  it('空串与全空白得到空数组', () => {
    expect(parseRunAction('')).toEqual([])
    expect(parseRunAction(' , ')).toEqual([])
  })

  it('未知取值原样带出，交给调用方兜底显示', () => {
    expect(parseRunAction('forward:a@b.com')[0]).toEqual({
      type: 'forward',
      value: 'a@b.com',
      raw: 'forward:a@b.com',
    })
  })
})

describe('splitMoveNote', () => {
  it('拆出后端加的结果标记', () => {
    expect(splitMoveNote('归档(missing)')).toEqual({ target: '归档', note: 'missing' })
    expect(splitMoveNote('归档(skipped)')).toEqual({ target: '归档', note: 'skipped' })
    expect(splitMoveNote('收件箱 (same)')).toEqual({ target: '收件箱', note: 'same' })
  })

  it('没有标记时原样返回', () => {
    expect(splitMoveNote('归档')).toEqual({ target: '归档', note: '' })
  })

  it('文件夹名自带的括号不当成标记', () => {
    expect(splitMoveNote('归档(2026)')).toEqual({ target: '归档(2026)', note: '' })
  })
})

describe('isKnownActionType', () => {
  it('识别五种已知动作', () => {
    for (const ty of ['move', 'mark_read', 'star', 'delete', 'notify']) {
      expect(isKnownActionType(ty)).toBe(true)
    }
  })

  it('block 与未知动作都不算已知动作类型', () => {
    // block 是黑名单专用的日志取值，不是可配置的动作，单独走一条文案
    expect(isKnownActionType('block')).toBe(false)
    expect(isKnownActionType('forward')).toBe(false)
  })
})

describe('moveArrayItem', () => {
  it('向前移动', () => {
    expect(moveArrayItem([1, 2, 3, 4], 2, 0)).toEqual([3, 1, 2, 4])
  })

  it('向后移动', () => {
    expect(moveArrayItem([1, 2, 3, 4], 0, 2)).toEqual([2, 3, 1, 4])
  })

  it('相邻交换', () => {
    expect(moveArrayItem(['a', 'b', 'c'], 1, 2)).toEqual(['a', 'c', 'b'])
  })

  it('越界时原样返回，不产生空洞', () => {
    expect(moveArrayItem([1, 2, 3], 0, -1)).toEqual([1, 2, 3])
    expect(moveArrayItem([1, 2, 3], 3, 0)).toEqual([1, 2, 3])
    expect(moveArrayItem([1, 2, 3], 1, 1)).toEqual([1, 2, 3])
  })

  it('不修改入参数组', () => {
    const src = [1, 2, 3]
    moveArrayItem(src, 0, 2)
    expect(src).toEqual([1, 2, 3])
  })
})

describe('normalizeBlockPattern', () => {
  it('去空白并小写', () => {
    expect(normalizeBlockPattern('  Alice@Example.COM ')).toBe('alice@example.com')
  })

  it('剥掉显示名，只留尖括号里的地址', () => {
    expect(normalizeBlockPattern('Alice <Alice@Example.com>')).toBe('alice@example.com')
  })

  it('剥掉裸尖括号', () => {
    expect(normalizeBlockPattern('<bob@example.com>')).toBe('bob@example.com')
  })

  it('剥掉 mailto: 前缀', () => {
    expect(normalizeBlockPattern('mailto:bob@example.com')).toBe('bob@example.com')
  })

  it('域名去掉前导 @', () => {
    expect(normalizeBlockPattern('@Spam.IO')).toBe('spam.io')
  })

  it('空串仍是空串', () => {
    expect(normalizeBlockPattern('   ')).toBe('')
  })
})

describe('isValidBlockPattern', () => {
  it('接受完整地址', () => {
    expect(isValidBlockPattern('alice@example.com')).toBe(true)
  })

  it('接受域名', () => {
    expect(isValidBlockPattern('example.com')).toBe(true)
    expect(isValidBlockPattern('mail.example.co.uk')).toBe(true)
  })

  it('拒绝空串与含空白的串', () => {
    expect(isValidBlockPattern('')).toBe(false)
    expect(isValidBlockPattern('a b@example.com')).toBe(false)
  })

  it('拒绝没有点的裸词', () => {
    expect(isValidBlockPattern('localhost')).toBe(false)
  })

  it('拒绝多个 @ 与半截地址', () => {
    expect(isValidBlockPattern('a@b@example.com')).toBe(false)
    expect(isValidBlockPattern('@example.com')).toBe(false)
    expect(isValidBlockPattern('alice@')).toBe(false)
  })

  it('拒绝非法域名段', () => {
    expect(isValidBlockPattern('exa_mple.com')).toBe(false)
    expect(isValidBlockPattern('example..com')).toBe(false)
    expect(isValidBlockPattern('-bad.com')).toBe(false)
  })
})

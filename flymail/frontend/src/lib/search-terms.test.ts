import { describe, it, expect } from 'vitest'
import { extractHighlightTerms } from '@/lib/search-terms'

describe('extractHighlightTerms', () => {
  it('空串返回空数组', () => {
    expect(extractHighlightTerms('')).toEqual([])
    expect(extractHighlightTerms('   ')).toEqual([])
  })

  it('自由词按长度降序返回', () => {
    expect(extractHighlightTerms('发票 电子发票')).toEqual(['电子发票', '发票'])
  })

  it('提取 from/to/subject 的取值', () => {
    expect(extractHighlightTerms('from:张三')).toEqual(['张三'])
    expect(extractHighlightTerms('to:lisi@x.com')).toEqual(['lisi@x.com'])
    expect(extractHighlightTerms('subject:发票')).toEqual(['发票'])
  })

  it('忽略结构化限定符（高亮它们只会误标正文里的同名词）', () => {
    const q = 'has:attachment is:unread before:2026-06-01 after:2026-01 in:inbox account:me@work.com'
    expect(extractHighlightTerms(q)).toEqual([])
  })

  it('限定符大小写不敏感', () => {
    expect(extractHighlightTerms('FROM:张三 Is:Unread')).toEqual(['张三'])
  })

  it('引号短语去掉引号后作为整体', () => {
    expect(extractHighlightTerms('"报销 发票"')).toEqual(['报销 发票'])
    expect(extractHighlightTerms('subject:"报销 发票"')).toEqual(['报销 发票'])
  })

  it('引号内的冒号不当作限定符', () => {
    expect(extractHighlightTerms('"is:unread"')).toEqual(['is:unread'])
  })

  it('未知限定符退化为普通文本（与后端一致）', () => {
    expect(extractHighlightTerms('foo:bar')).toEqual(['foo:bar'])
  })

  it('取值为空的限定符被丢弃', () => {
    expect(extractHighlightTerms('from: 发票')).toEqual(['发票'])
  })

  it('混合查询只留文本部分', () => {
    const q = 'is:unread from:张三 subject:"季度 报告" has:attachment 紧急'
    expect(extractHighlightTerms(q)).toEqual(['季度 报告', '张三', '紧急'])
  })

  it('去重按小写键，保留首次出现的写法', () => {
    expect(extractHighlightTerms('Invoice invoice INVOICE')).toEqual(['Invoice'])
    expect(extractHighlightTerms('subject:Report report')).toEqual(['Report'])
  })

  it('多个空白分隔正常切分', () => {
    expect(extractHighlightTerms('  a   bbb  ')).toEqual(['bbb', 'a'])
  })
})

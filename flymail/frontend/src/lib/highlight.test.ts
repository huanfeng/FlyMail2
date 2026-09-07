import { describe, it, expect } from 'vitest'
import { splitHighlight } from '@/lib/highlight'

describe('splitHighlight', () => {
  it('空文本返回空数组', () => {
    expect(splitHighlight('', ['a'])).toEqual([])
  })

  it('无词时整段未命中', () => {
    expect(splitHighlight('hello', [])).toEqual([{ text: 'hello', hit: false }])
    expect(splitHighlight('hello', [''])).toEqual([{ text: 'hello', hit: false }])
  })

  it('无命中时整段未命中', () => {
    expect(splitHighlight('hello', ['xyz'])).toEqual([{ text: 'hello', hit: false }])
  })

  it('切出命中段', () => {
    expect(splitHighlight('季度报告已发出', ['报告'])).toEqual([
      { text: '季度', hit: false },
      { text: '报告', hit: true },
      { text: '已发出', hit: false },
    ])
  })

  it('大小写不敏感，且保留原文写法', () => {
    expect(splitHighlight('Invoice #3', ['invoice'])).toEqual([
      { text: 'Invoice', hit: true },
      { text: ' #3', hit: false },
    ])
  })

  it('多个命中分别切出', () => {
    expect(splitHighlight('a-b-a', ['a'])).toEqual([
      { text: 'a', hit: true },
      { text: '-b-', hit: false },
      { text: 'a', hit: true },
    ])
  })

  it('相邻命中段合并为一段', () => {
    expect(splitHighlight('abab', ['ab'])).toEqual([{ text: 'abab', hit: true }])
  })

  it('长词优先（terms 已按长度降序）', () => {
    expect(splitHighlight('电子发票', ['电子发票', '发票'])).toEqual([
      { text: '电子发票', hit: true },
    ])
  })

  it('正则元字符被转义，按字面匹配', () => {
    expect(splitHighlight('a.b', ['.'])).toEqual([
      { text: 'a', hit: false },
      { text: '.', hit: true },
      { text: 'b', hit: false },
    ])
    expect(splitHighlight('axb', ['.'])).toEqual([{ text: 'axb', hit: false }])
    expect(splitHighlight('c++ 手册', ['c++'])).toEqual([
      { text: 'c++', hit: true },
      { text: ' 手册', hit: false },
    ])
  })

  it('整段命中时只有一段', () => {
    expect(splitHighlight('发票', ['发票'])).toEqual([{ text: '发票', hit: true }])
  })
})

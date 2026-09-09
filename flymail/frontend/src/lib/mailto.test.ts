import { describe, it, expect } from 'vitest'
import { parseMailto } from '@/lib/mailto'

describe('parseMailto', () => {
  it('只有收件人', () => {
    expect(parseMailto('mailto:alice@example.com')).toEqual({
      to: ['alice@example.com'],
      cc: [],
      bcc: [],
      subject: '',
      body: '',
    })
  })

  it('多个收件人用逗号分隔，首尾空白去掉', () => {
    expect(parseMailto('mailto:a@x.com,%20b@x.com')?.to).toEqual(['a@x.com', 'b@x.com'])
  })

  it('解析 subject / body / cc / bcc，并做百分号解码', () => {
    const f = parseMailto('mailto:a@x.com?subject=%E4%BD%A0%E5%A5%BD&body=line1%0Aline2&cc=c@x.com&bcc=d@x.com')
    expect(f).toEqual({
      to: ['a@x.com'],
      cc: ['c@x.com'],
      bcc: ['d@x.com'],
      subject: '你好',
      body: 'line1\nline2',
    })
  })

  it('查询串里的 to 与路径部分叠加，不是覆盖', () => {
    expect(parseMailto('mailto:a@x.com?to=b@x.com')?.to).toEqual(['a@x.com', 'b@x.com'])
  })

  it('不把 + 当空格：带 + 的邮箱地址不能被拆坏', () => {
    // RFC 6068 的查询串不是表单编码，+ 就是加号
    expect(parseMailto('mailto:a+tag@x.com')?.to).toEqual(['a+tag@x.com'])
    expect(parseMailto('mailto:?to=a+tag@x.com')?.to).toEqual(['a+tag@x.com'])
  })

  it('忽略 in-reply-to 之类的其它 hfield', () => {
    const f = parseMailto('mailto:a@x.com?in-reply-to=%3Cid%40host%3E&subject=hi')
    expect(f).toEqual({ to: ['a@x.com'], cc: [], bcc: [], subject: 'hi', body: '' })
  })

  it('非法百分号序列原样保留，不让整条链接失效', () => {
    expect(parseMailto('mailto:a@x.com?subject=100%25%zz')?.subject).toBe('100%25%zz')
  })

  it('大小写不敏感的协议名', () => {
    expect(parseMailto('MAILTO:a@x.com')?.to).toEqual(['a@x.com'])
  })

  it('不是 mailto、或者什么都没有时返回 null', () => {
    expect(parseMailto('https://x.com')).toBeNull()
    expect(parseMailto('mailto:')).toBeNull()
    expect(parseMailto('mailto:?bcc=a@x.com')).toBeNull()
  })
})

describe('parseMailto 控制字符', () => {
  it('地址里的 CRLF 被剥掉（否则一路走到发送请求里可做头部注入）', () => {
    const f = parseMailto('mailto:a@x.com%0D%0ABcc:%20victim@y.com')
    expect(f?.to).toEqual(['a@x.comBcc: victim@y.com'])
    expect(f?.to.join('')).not.toMatch(/[\r\n]/)
  })

  it('主题里的换行与控制字符被剥掉', () => {
    const f = parseMailto('mailto:a@x.com?subject=hi%0D%0AX-Evil:%201%00')
    expect(f?.subject).toBe('hiX-Evil: 1')
  })

  it('正文保留换行（mailto body 的正当用法），其余控制字符照剥', () => {
    const f = parseMailto('mailto:a@x.com?body=l1%0Al2%00%07')
    expect(f?.body).toBe('l1\nl2')
  })
})

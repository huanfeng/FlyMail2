import { describe, it, expect } from 'vitest'
import { presetForEmail } from '@/lib/providers'

describe('presetForEmail', () => {
  it('user@163.com 命中 163，imap.host 正确，smtp.port=465', () => {
    const r = presetForEmail('user@163.com')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('163')
    expect(r!.imap.host).toBe('imap.163.com')
    expect(r!.smtp.port).toBe(465)
  })

  it('a@gmail.com 命中 gmail', () => {
    const r = presetForEmail('a@gmail.com')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('gmail')
  })

  it('a@googlemail.com 也命中 gmail', () => {
    const r = presetForEmail('a@googlemail.com')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('gmail')
  })

  it('大小写不敏感：A@GMAIL.COM → gmail', () => {
    const r = presetForEmail('A@GMAIL.COM')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('gmail')
  })

  it('a@outlook.com 命中 outlook，smtp.security=starttls', () => {
    const r = presetForEmail('a@outlook.com')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('outlook')
    expect(r!.smtp.security).toBe('starttls')
  })

  it('无 @ 符号 → null', () => {
    expect(presetForEmail('notanemail')).toBeNull()
  })

  it('未知域名 a@example.com → null', () => {
    expect(presetForEmail('a@example.com')).toBeNull()
  })

  it('域名含尾部空格：a@163.com  → 仍命中 163', () => {
    // 注意：尾部空格紧跟域名，测试 trim() 逻辑
    const r = presetForEmail('a@163.com  ')
    expect(r).not.toBeNull()
    expect(r!.id).toBe('163')
  })
})

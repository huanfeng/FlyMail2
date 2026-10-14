import { describe, it, expect } from 'vitest'
import { avatarHue, avatarKey } from '@/lib/avatar-color'

describe('avatarHue', () => {
  it('结果落在 1..8', () => {
    for (const k of ['a@b.com', 'x', '', '张三', 'chip3@example.com']) {
      const h = avatarHue(k)
      expect(h).toBeGreaterThanOrEqual(1)
      expect(h).toBeLessThanOrEqual(8)
    }
  })
  it('同一键稳定、不同键有区分', () => {
    expect(avatarHue('chip3@example.com')).toBe(avatarHue('chip3@example.com'))
    const set = new Set(['a@x', 'b@x', 'c@x', 'd@x', 'e@x', 'f@x', 'g@x', 'h@x'].map(avatarHue))
    expect(set.size).toBeGreaterThan(2)
  })
  it('键优先用地址并忽略大小写与空白', () => {
    expect(avatarKey('张三', ' Zhang@Example.com ')).toBe('zhang@example.com')
    expect(avatarKey('张三', '')).toBe('张三')
  })
})

import { describe, it, expect, beforeEach } from 'vitest'
import { rememberAccount, loadLastAccount, resolveContextAccount } from '@/lib/last-account'

beforeEach(() => localStorage.clear())

/**
 * 「写邮件 / 草稿箱」该用哪个账户。
 *
 * ── 为什么不从 URL 取 ────────────────────────────────────────────────────────
 *
 * URL 应当准确描述「现在在看什么」。聚合视图是跨账户的，那里没有「当前账户」，
 * URL 里挂一个 account 是在陈述一件不成立的事，还会让人以为列表被它过滤了。
 * 但写邮件确实需要一个发件人——那是**上下文偏好**，属于这台设备，存在本地。
 */
describe('resolveContextAccount', () => {
  it('正在看某个账户时就用它', () => {
    expect(resolveContextAccount(3, [1, 3, 5], 5)).toBe(3)
  })

  it('聚合视图下（没有当前账户）退到记忆', () => {
    expect(resolveContextAccount(null, [1, 3, 5], 5)).toBe(5)
  })

  /**
   * ⚠ 记忆里那个账户可能已经被删了。
   *
   * 不校验的话，写邮件会带着一个不存在的 account_id 发出去——后端拒绝，
   * 而用户完全看不出哪里不对。
   */
  it('记忆指向已删除的账户时退到第一个', () => {
    expect(resolveContextAccount(null, [1, 3], 99)).toBe(1)
    expect(resolveContextAccount(99, [1, 3], null)).toBe(1)
  })

  it('一个账户都没有时返回 null', () => {
    expect(resolveContextAccount(null, [], null)).toBeNull()
  })
})

describe('rememberAccount / loadLastAccount', () => {
  it('记住并读回', () => {
    rememberAccount(7)
    expect(loadLastAccount()).toBe(7)
  })

  /** 聚合视图下 accountId 是 null，那一下不该把已有记忆冲掉。 */
  it('传 null 不冲掉已有记忆', () => {
    rememberAccount(7)
    rememberAccount(null)
    expect(loadLastAccount()).toBe(7)
  })

  it('没记过时是 null', () => {
    expect(loadLastAccount()).toBeNull()
  })

  it('存储里是垃圾时返回 null 而不是 NaN', () => {
    localStorage.setItem('flymail.lastAccount', 'abc')
    expect(loadLastAccount()).toBeNull()
    localStorage.setItem('flymail.lastAccount', '-1')
    expect(loadLastAccount()).toBeNull()
  })
})

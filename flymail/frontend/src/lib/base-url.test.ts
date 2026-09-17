import { describe, it, expect } from 'vitest'
import { checkBaseUrl } from '@/lib/base-url'

/**
 * 「对外访问地址」的校验。
 *
 * ── 为什么这项要认真校验 ─────────────────────────────────────────────────────
 *
 * 它唯一的用途是拼通知里的「打开邮件」链接。填错没有任何即时反馈——设置保存成功、
 * 通知照发，直到某天有人点了那条链接才发现打不开。能在保存那一刻判出来的错，
 * 必须当场拦下。
 *
 * 最容易漏的是 `mail.example.com` 这种没有 scheme 的写法：看起来完全正常，
 * 后端的 url.Parse 也不会报错（当成相对路径），拼出来是死链。
 */
describe('checkBaseUrl', () => {
  it('留空是合法的——表示不配，通知就不带链接', () => {
    expect(checkBaseUrl('')).toBe('ok')
    expect(checkBaseUrl('   ')).toBe('ok')
  })

  it('接受带主机名的 http/https 绝对地址', () => {
    for (const v of [
      'https://mail.example.com',
      'https://mail.example.com/',
      'http://192.168.5.11:8086',
      'https://example.com/mail', // 反代挂在子路径下
      'HTTPS://MAIL.EXAMPLE.COM', // scheme 大小写不敏感
    ]) {
      expect(checkBaseUrl(v), `${v} 应当合法`).toBe('ok')
    }
  })

  it('没有 scheme 的裸主机名要拒掉', () => {
    // ⚠ 这条是重点：它看起来最像"对的"，拼出来却是 mail.example.com/?message=1
    expect(checkBaseUrl('mail.example.com')).toBe('scheme')
    // 192 不是合法 scheme（必须字母开头），所以这也落进"没有 scheme"
    expect(checkBaseUrl('192.168.5.11:8086')).toBe('scheme')
  })

  it('非 http/https 的 scheme 要拒掉', () => {
    expect(checkBaseUrl('ftp://mail.example.com')).toBe('scheme')
    expect(checkBaseUrl('mailto:a@b.com')).toBe('scheme')
  })

  /**
   * ⚠ 这几条钉的是「前端判据不能比后端松」。
   *
   * `new URL()` 是 WHATWG 解析器，对 http/https 会自动补斜杠，于是
   * `http:/mail.example.com`（少打一个斜杠）在它眼里完全合法、hostname 还是对的；
   * 而后端 Go 的 url.Parse 认为 Host 为空，直接拒。只依赖 URL 构造函数的话，
   * 前端放行 → 用户点保存 → 撞一个 400。
   *
   * 判据往松了写在这里是**静默**的：功能看起来都正常，只有真去点保存才暴露。
   */
  it('有 scheme 但缺 //主机名 的要拒掉，判据不能比后端松', () => {
    expect(checkBaseUrl('http://'), 'http:// 后面空着').toBe('host')
    expect(checkBaseUrl('https:///path'), '三个斜杠，主机名位置是空的').toBe('host')
    expect(checkBaseUrl('http:/mail.example.com'), '少打一个斜杠——最常见的手误').toBe('host')
    expect(checkBaseUrl('http:mail.example.com'), '冒号后面直接跟主机名').toBe('host')
  })

  it('完全不是 URL 的输入不抛异常', () => {
    // 没有冒号 → 连 scheme 都没有，提示"必须以 http:// 开头"正是用户要看的
    expect(checkBaseUrl('这是一段说明文字')).toBe('scheme')
    // 形式对但 URL 构造函数仍解析不了的，落到 invalid
    expect(checkBaseUrl('http://[')).toBe('invalid')
  })
})

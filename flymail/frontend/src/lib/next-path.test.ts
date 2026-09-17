import { describe, it, expect } from 'vitest'
import { encodeNext, safeNext } from '@/lib/next-path'

describe('encodeNext', () => {
  it('把路径与查询串一起带上', () => {
    // 通知链接的全部信息都在查询串里，丢了它等于没带来路
    expect(safeNext(encodeNext('/', '?account=1&folder=7&message=42'))).toBe(
      '/?account=1&folder=7&message=42',
    )
  })
})

describe('safeNext', () => {
  it('接受站内路径', () => {
    expect(safeNext(encodeNext('/', '?message=42'))).toBe('/?message=42')
    expect(safeNext('%2Fsettings')).toBe('/settings')
  })

  it('没有来路时返回 null，调用方回落首页', () => {
    expect(safeNext(null)).toBeNull()
    expect(safeNext('')).toBeNull()
    expect(safeNext(undefined)).toBeNull()
  })

  /**
   * ⚠ 这一组是安全判据，不是输入校验。
   *
   * 放行站外地址就等于在我们自己的域名上开了一个重定向器：攻击者发
   * `https://mail.example.com/login?next=https://evil.example.com/`，
   * 用户看到的是从可信站点跳过去的，钓鱼页再仿一个登录框即可。
   *
   * 只判「以 / 开头」是不够的——`//evil.com` 和 `/\evil.com` 都以 / 开头，
   * 浏览器却都当跨站地址处理。这两条是这里最容易漏的。
   */
  it('拒绝一切会离开本站的目标', () => {
    for (const bad of [
      'https://evil.example.com/',
      'http://evil.example.com/',
      '//evil.example.com/', // 协议相对 URL
      '%2F%2Fevil.example.com', // 编码过的同一个东西
      '/\\evil.example.com', // 多数浏览器把 \ 当 /
      'evil.example.com',
      'javascript:alert(1)',
    ]) {
      expect(safeNext(bad), `${bad} 必须被拒`).toBeNull()
    }
  })

  it('拒绝带控制字符的目标', () => {
    expect(safeNext('/%0A%0Dhttps://evil.example.com')).toBeNull()
    expect(safeNext('/foo%00bar')).toBeNull()
  })

  it('拒绝畸形编码而不是抛异常', () => {
    expect(safeNext('%')).toBeNull()
    expect(safeNext('%zz')).toBeNull()
  })

  it('不把人送回登录页，否则登录完还在登录页', () => {
    expect(safeNext('/login')).toBeNull()
    expect(safeNext('%2Flogin%3Fnext%3D%252F')).toBeNull()
  })
})

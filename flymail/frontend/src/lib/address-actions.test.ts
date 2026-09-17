import { describe, it, expect } from 'vitest'
import { addressActions } from '@/lib/address-actions'

describe('addressActions', () => {
  it('别人发来的邮件：可以信任、可以屏蔽', () => {
    expect(addressActions('from', false, 'a@x.com')).toEqual({ canTrust: true, canBlock: true })
  })

  /**
   * ⚠ 这条是安全判据，不是交互偏好。
   *
   * 「已发送」里每封的发件人都是自己，抄送里也常有自己的其它信箱。给了屏蔽入口，
   * 点一下就把自己拉黑——之后所有自发自收、抄送自己的邮件都进垃圾箱，
   * 而用户完全不知道为什么。
   */
  it('自己的地址不给屏蔽入口', () => {
    expect(addressActions('from', true, 'me@x.com').canBlock).toBe(false)
    expect(addressActions('to', true, 'me@x.com').canBlock).toBe(false)
  })

  it('收件人不给信任/屏蔽——那两项管的是「谁发来的」，不是「发给谁」', () => {
    expect(addressActions('to', false, 'a@x.com')).toEqual({ canTrust: false, canBlock: false })
  })

  it('空地址不给任何一项', () => {
    expect(addressActions('from', false, '   ')).toEqual({ canTrust: false, canBlock: false })
  })
})

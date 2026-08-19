import { describe, it, expect, beforeEach } from 'vitest'
import { savedLogin } from './saved-login'

describe('savedLogin', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('保存后可读回（含非 ASCII 密码）', () => {
    savedLogin.save({ username: 'admin', password: '密码🔑p@ss' })
    expect(savedLogin.load()).toEqual({ username: 'admin', password: '密码🔑p@ss' })
  })

  it('未保存时返回 null', () => {
    expect(savedLogin.load()).toBeNull()
  })

  it('clear 后返回 null', () => {
    savedLogin.save({ username: 'admin', password: 'x' })
    savedLogin.clear()
    expect(savedLogin.load()).toBeNull()
  })

  it('数据损坏时返回 null 并清除', () => {
    localStorage.setItem('flymail_saved_login', 'not-json{')
    expect(savedLogin.load()).toBeNull()
    expect(localStorage.getItem('flymail_saved_login')).toBeNull()
  })

  it('密码不以明文落盘', () => {
    savedLogin.save({ username: 'admin', password: 'secret-pass' })
    expect(localStorage.getItem('flymail_saved_login')).not.toContain('secret-pass')
  })
})

import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  loadExpanded, saveExpanded, initialExpanded, anyExpanded, collapseAll,
} from '@/lib/sidebar-expand'

beforeEach(() => localStorage.clear())

/**
 * 侧栏展开状态。
 *
 * ── 原先的写法为什么是坏的 ──────────────────────────────────────────────────
 *
 * 原先是 `useState(() => { accounts.forEach((a,i) => init[a.id] = i<2) })`。
 * useState 的初始化函数只在**首次渲染**执行一次，而那一刻账户列表还在请求中、
 * 是个空数组——所以「默认展开前两个」从来没有生效过，侧栏永远全部折叠，
 * 而且刷新一次就忘光。
 */
describe('initialExpanded', () => {
  it('没存过时默认展开前两个', () => {
    expect(initialExpanded([7, 8, 9], {})).toEqual({ 7: true, 8: true, 9: false })
  })

  /**
   * ⚠ 存过 false 的必须保持折叠。
   *
   * 「没存过」和「存过 false」是两回事，用 `stored[id] || i < 2` 这种写法会把它们
   * 混为一谈——用户显式折叠掉的前两个账户，下次打开又自己展开了。
   */
  it('用户显式折叠过的不能又给他展开', () => {
    expect(initialExpanded([7, 8, 9], { 7: false, 9: true })).toEqual({ 7: false, 8: true, 9: true })
  })

  it('新加的账户按默认规则补位，不影响已有的', () => {
    const got = initialExpanded([7, 8, 9], { 7: false, 8: false })
    expect(got[7]).toBe(false)
    expect(got[8]).toBe(false)
    expect(got[9]).toBe(false) // 第三个，超出默认展开数
  })
})

describe('loadExpanded / saveExpanded', () => {
  it('存进去能读回来', () => {
    saveExpanded({ 3: true, 5: false })
    expect(loadExpanded()).toEqual({ 3: true, 5: false })
  })

  it('没存过时是空表', () => {
    expect(loadExpanded()).toEqual({})
  })

  /** 存储里的内容可能被别的东西写坏，读不出来时不能让侧栏崩掉。 */
  it('坏数据一律当作没存过', () => {
    localStorage.setItem('flymail.sidebar.expanded', '{不是 JSON')
    expect(loadExpanded()).toEqual({})
    localStorage.setItem('flymail.sidebar.expanded', '[1,2,3]')
    expect(loadExpanded()).toEqual({})
    localStorage.setItem('flymail.sidebar.expanded', '{"abc":true,"7":"yes","8":true}')
    expect(loadExpanded()).toEqual({ 8: true }) // 非法键与非布尔值都被丢掉
  })

  /** 隐私模式下 localStorage 会抛异常，不能让它冒泡到渲染。 */
  it('存储不可用时不抛异常', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    expect(() => saveExpanded({ 1: true })).not.toThrow()
    spy.mockRestore()

    const spy2 = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    expect(loadExpanded()).toEqual({})
    spy2.mockRestore()
  })
})

describe('anyExpanded / collapseAll', () => {
  it('有展开的才显示「全部折叠」，否则那是个死按钮', () => {
    expect(anyExpanded({ 1: false, 2: false })).toBe(false)
    expect(anyExpanded({ 1: false, 2: true })).toBe(true)
    expect(anyExpanded({})).toBe(false)
  })

  it('全部折叠后保留每个账户的键', () => {
    expect(collapseAll({ 1: true, 2: true, 3: false })).toEqual({ 1: false, 2: false, 3: false })
  })
})

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import axios from 'axios'
import MockAdapter from 'axios-mock-adapter'

// ── vi.hoisted 确保 mockAuthState 在 vi.mock 提升后仍可访问 ──
const mockAuthState = vi.hoisted(() => ({
  _access: 'old-access-token',
  _refresh: 'valid-refresh-token',
  get access() { return this._access },
  get refresh() { return this._refresh },
  set: vi.fn((a: string, r: string) => {
    mockAuthState._access = a
    mockAuthState._refresh = r
  }),
  clear: vi.fn(() => {
    mockAuthState._access = ''
    mockAuthState._refresh = ''
  }),
  isAuthenticated: vi.fn(() => !!mockAuthState._access),
}))

vi.mock('@/lib/auth', () => ({
  auth: mockAuthState,
}))

// 导入被测模块（在 mock 之后）
import api from '@/lib/api'

// jsdom 中 window.location 默认不可写，替换为可控对象
Object.defineProperty(window, 'location', {
  value: { pathname: '/', href: '' },
  writable: true,
})

describe('api 拦截器 - 401 自动刷新', () => {
  let mockApi: MockAdapter
  let mockAxios: MockAdapter

  beforeEach(() => {
    // 重置 auth 状态
    mockAuthState._access = 'old-access-token'
    mockAuthState._refresh = 'valid-refresh-token'
    mockAuthState.set.mockClear()
    mockAuthState.clear.mockClear()
    ;(window.location as { href: string }).href = ''

    mockApi = new MockAdapter(api)
    mockAxios = new MockAdapter(axios)
  })

  afterEach(() => {
    mockApi.restore()
    mockAxios.restore()
  })

  // ── 用例 A：刷新成功后重试请求 ──
  it('A: 首次 401 → 刷新 token → 重试请求成功', async () => {
    // /accounts 第一次 401，第二次 200
    mockApi
      .onGet('/accounts').replyOnce(401)
      .onGet('/accounts').replyOnce(200, { ok: true })

    // /auth/refresh 返回新 token
    mockAxios
      .onPost('/api/v1/auth/refresh').replyOnce(200, {
        access_token: 'new-access-token',
        refresh_token: 'new-refresh-token',
      })

    const res = await api.get('/accounts')
    expect(res.status).toBe(200)
    expect(res.data).toEqual({ ok: true })
    // auth.set 应被调用，保存新 token
    expect(mockAuthState.set).toHaveBeenCalledWith('new-access-token', 'new-refresh-token')
  })

  // ── 用例 B：刷新失败 → 清除登录态 ──
  it('B: 刷新 token 失败 → auth.clear() 被调用，请求 reject', async () => {
    mockApi.onGet('/accounts').replyOnce(401)
    // refresh 也返回 401
    mockAxios.onPost('/api/v1/auth/refresh').replyOnce(401)

    await expect(api.get('/accounts')).rejects.toThrow()
    expect(mockAuthState.clear).toHaveBeenCalled()
  })

  // ── 用例 C：singleflight —— 并发 401 只触发一次 refresh ──
  it('C: 两个并发请求同时 401 → refresh 只发生 1 次', async () => {
    mockApi
      .onGet('/accounts').replyOnce(401)
      .onGet('/accounts').replyOnce(200, { ok: true })
      .onGet('/folders').replyOnce(401)
      .onGet('/folders').replyOnce(200, { list: [] })

    let refreshCallCount = 0
    mockAxios.onPost('/api/v1/auth/refresh').reply(() => {
      refreshCallCount++
      return [200, { access_token: 'new-token', refresh_token: 'new-rt' }]
    })

    // 并发发起两个请求
    await Promise.all([
      api.get('/accounts').catch(() => null),
      api.get('/folders').catch(() => null),
    ])

    // refresh 只应被调用一次（singleflight）
    expect(refreshCallCount).toBe(1)
  })
})

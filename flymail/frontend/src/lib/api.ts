import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { auth } from '@/lib/auth'

// axios 实例：所有请求都走 /api/v1，开发环境由 vite proxy 转发到后端。
const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
})

// 请求拦截器：自动注入 Authorization: Bearer <access token>。
api.interceptors.request.use((config) => {
  const token = auth.access
  if (token) {
    config.headers = config.headers || {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

export interface LoginResponse {
  access_token: string
  refresh_token: string
}

// ---- 401 自动刷新 ----
// access token TTL 较短，过期后受保护接口会 401。这里用 refresh token 静默换新并重试原请求；
// 并发 401 共享同一次刷新（singleflight），避免重复刷新；刷新失败则清登录态并跳转登录页。
let refreshing: Promise<string | null> | null = null

async function doRefresh(): Promise<string | null> {
  const rt = auth.refresh
  if (!rt) return null
  try {
    // 用裸 axios（不经本实例拦截器），避免循环。
    const res = await axios.post<LoginResponse>(
      '/api/v1/auth/refresh',
      { refresh_token: rt },
      { headers: { 'Content-Type': 'application/json' } },
    )
    auth.set(res.data.access_token, res.data.refresh_token)
    return res.data.access_token
  } catch {
    return null
  }
}

function redirectToLogin(): void {
  if (window.location.pathname !== '/login') {
    window.location.href = '/login'
  }
}

api.interceptors.response.use(
  (resp) => resp,
  async (error: AxiosError) => {
    const original = error.config as (InternalAxiosRequestConfig & { _retry?: boolean }) | undefined
    const status = error.response?.status
    const url = original?.url ?? ''

    // 仅处理受保护请求的首次 401；认证端点本身的 401 不在此重试。
    const isAuthEndpoint = url.includes('/auth/login') || url.includes('/auth/refresh')
    if (status !== 401 || !original || original._retry || isAuthEndpoint) {
      if (status === 401 && url.includes('/auth/refresh')) {
        // refresh 也失效：会话彻底过期。
        auth.clear()
        redirectToLogin()
      }
      return Promise.reject(error)
    }

    original._retry = true
    if (!refreshing) {
      refreshing = doRefresh().finally(() => {
        refreshing = null
      })
    }
    const newToken = await refreshing
    if (!newToken) {
      auth.clear()
      redirectToLogin()
      return Promise.reject(error)
    }
    original.headers = original.headers ?? {}
    original.headers.Authorization = `Bearer ${newToken}`
    return api(original)
  },
)

/** 登录：调用后端 /auth/login，成功后保存 token。 */
export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await api.post<LoginResponse>('/auth/login', { username, password })
  const { access_token, refresh_token } = res.data
  auth.set(access_token, refresh_token)
  return res.data
}

export default api

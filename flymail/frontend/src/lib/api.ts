import axios from 'axios'
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

/** 登录：调用后端 /auth/login，成功后保存 token。 */
export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await api.post<LoginResponse>('/auth/login', { username, password })
  const { access_token, refresh_token } = res.data
  auth.set(access_token, refresh_token)
  return res.data
}

export default api

import http from './http'
import { useAuthStore } from '@/stores/auth'

let refreshPromise: Promise<string | null> | null = null

// 401 auto-refresh interceptor (depends on auth store, so kept separate from http.ts)
http.interceptors.response.use(
  undefined, // don't duplicate the success handler from http.ts
  async (error) => {
    const { response, config } = error
    if (!response || !config) return Promise.reject(error)

    const status = response.status
    const originalRequest = config as typeof config & { _retry?: boolean; _skipAuthRetry?: boolean }
    const url = config.url || ''
    const noRetryPaths = ['/auth/login', '/auth/setup', '/auth/refresh']
    const isNoRetry = noRetryPaths.some((p) => url.includes(p))

    if (status === 401 && !originalRequest._retry && !isNoRetry && !originalRequest._skipAuthRetry) {
      originalRequest._retry = true
      const auth = useAuthStore.getState()

      if (!refreshPromise) {
        refreshPromise = auth.refresh().finally(() => {
          refreshPromise = null
        })
      }

      const newToken = await refreshPromise
      if (newToken) {
        originalRequest.headers = originalRequest.headers || {}
        originalRequest.headers.Authorization = `Bearer ${newToken}`
        return http(originalRequest)
      }

      useAuthStore.getState().clear()
    }

    return Promise.reject(error)
  }
)

/** Extract array data from API response, handles both unwrapped and raw formats */
export function extractList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data
  if (data && typeof data === 'object' && 'data' in data) {
    const inner = (data as Record<string, unknown>).data
    if (Array.isArray(inner)) return inner
  }
  return []
}

export default http

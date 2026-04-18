import http from './http'
import { useAuthStore } from '@/stores/auth'

let refreshPromise: Promise<string | null> | null = null

// Request interceptor: inject Bearer token
http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token) {
    config.headers = config.headers || {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor: unwrap unified format + auto refresh
http.interceptors.response.use(
  (response) => {
    const body = response.data
    if (body && typeof body === 'object' && 'code' in body && body.code === 0) {
      response.data = body.data
    }
    return response
  },
  async (error) => {
    const { response, config } = error
    if (!response || !config) return Promise.reject(error)

    // Normalize error format
    const body = response.data
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      response.data.error = body.message || body.error?.details || 'unknown error'
    }

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

export default http

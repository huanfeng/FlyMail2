import axios from 'axios'

const STORAGE_KEY = 'mail2im_auth_session'

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api',
  headers: { 'Content-Type': 'application/json' },
})

// Request interceptor: inject Bearer token from localStorage (no store dependency)
http.interceptors.request.use((config) => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const session = JSON.parse(raw)
      if (session.accessToken) {
        config.headers = config.headers || {}
        config.headers.Authorization = `Bearer ${session.accessToken}`
      }
    }
  } catch { /* ignore */ }
  return config
})

// Response interceptor: unwrap unified format {code, message, data} → data
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

    // Normalize error: put message string into .error for backward compat
    const body = response.data
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      response.data.error = body.message || body.error?.details || 'unknown error'
    }

    return Promise.reject(error)
  }
)

export default http

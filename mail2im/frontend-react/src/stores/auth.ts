import { create } from 'zustand'
import http from '@/services/http'

const STORAGE_KEY = 'mail2im_auth_session'

/** Unwrap unified API response {code, data} → data, or return as-is */
function unwrap<T>(data: unknown): T {
  if (data && typeof data === 'object' && 'code' in data && 'data' in data) {
    return (data as Record<string, unknown>).data as T
  }
  return data as T
}

type UserInfo = {
  id: number
  username: string
  email?: string
  last_seen?: string
}

type SessionPayload = {
  user: UserInfo
  access_token: string
  refresh_token: string
  access_expires_at?: string
  refresh_expires_at?: string
}

type AuthState = {
  user: UserInfo | null
  accessToken: string
  refreshToken: string
  accessExpiresAt: string
  refreshExpiresAt: string
  initialized: boolean
  isAuthenticated: boolean
}

type AuthActions = {
  setSession: (payload: SessionPayload) => void
  loadFromStorage: () => void
  clear: () => void
  login: (identifier: string, password: string) => Promise<SessionPayload>
  setup: (username: string, password: string, email?: string) => Promise<SessionPayload>
  refresh: () => Promise<string | null>
  fetchProfile: () => Promise<UserInfo | null>
  updateProfile: (payload: { username: string; email?: string; current_password?: string; new_password?: string }) => Promise<SessionPayload>
}

export const useAuthStore = create<AuthState & AuthActions>((set, get) => ({
  user: null,
  accessToken: '',
  refreshToken: '',
  accessExpiresAt: '',
  refreshExpiresAt: '',
  initialized: false,
  isAuthenticated: false,

  setSession(payload) {
    set({
      user: payload.user,
      accessToken: payload.access_token,
      refreshToken: payload.refresh_token,
      accessExpiresAt: payload.access_expires_at || '',
      refreshExpiresAt: payload.refresh_expires_at || '',
      isAuthenticated: true,
    })
    const state = get()
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      user: state.user,
      accessToken: state.accessToken,
      refreshToken: state.refreshToken,
      accessExpiresAt: state.accessExpiresAt,
      refreshExpiresAt: state.refreshExpiresAt,
    }))
  },

  loadFromStorage() {
    if (get().initialized) return
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (raw) {
        const stored = JSON.parse(raw)
        set({
          user: stored.user,
          accessToken: stored.accessToken || '',
          refreshToken: stored.refreshToken || '',
          accessExpiresAt: stored.accessExpiresAt || '',
          refreshExpiresAt: stored.refreshExpiresAt || '',
          isAuthenticated: !!(stored.accessToken && stored.user),
        })
      }
    } catch { /* ignore */ }
    set({ initialized: true })
  },

  clear() {
    set({
      user: null,
      accessToken: '',
      refreshToken: '',
      accessExpiresAt: '',
      refreshExpiresAt: '',
      isAuthenticated: false,
    })
    localStorage.removeItem(STORAGE_KEY)
  },

  async login(identifier, password) {
    const res = await http.post('/auth/login', { identifier, password })
    const payload = unwrap<SessionPayload>(res.data)
    get().setSession(payload)
    return payload
  },

  async setup(username, password, email) {
    const res = await http.post('/auth/setup', { username, password, email })
    const payload = unwrap<SessionPayload>(res.data)
    get().setSession(payload)
    return payload
  },

  async refresh() {
    const { refreshToken } = get()
    if (!refreshToken) return null
    try {
      const res = await http.post('/auth/refresh', { refresh_token: refreshToken }, { _skipAuthRetry: true } as never)
      const payload = unwrap<SessionPayload>(res.data)
      get().setSession(payload)
      return get().accessToken
    } catch {
      get().clear()
      return null
    }
  },

  async fetchProfile() {
    try {
      const res = await http.get('/auth/me')
      const data = unwrap<{ user: UserInfo }>(res.data)
      const user = data.user
      set({ user })
      // persist
      const state = get()
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        user: state.user,
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
        accessExpiresAt: state.accessExpiresAt,
        refreshExpiresAt: state.refreshExpiresAt,
      }))
      return user
    } catch {
      return null
    }
  },

  async updateProfile(payload) {
    const res = await http.put('/auth/profile', payload)
    const data = unwrap<SessionPayload>(res.data)
    get().setSession(data)
    return data
  },
}))

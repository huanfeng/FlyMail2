import { defineStore } from 'pinia';
import http from '../services/http';
import { getJSON, setJSON, remove, KEYS } from '../utils/storage';

type UserInfo = {
  id: number;
  username: string;
  email?: string;
  last_seen?: string;
};

type SessionPayload = {
  user: UserInfo;
  access_token: string;
  refresh_token: string;
  access_expires_at?: string;
  refresh_expires_at?: string;
};

type StoredSession = {
  user: UserInfo | null;
  accessToken: string;
  refreshToken: string;
  accessExpiresAt?: string;
  refreshExpiresAt?: string;
};

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as UserInfo | null,
    accessToken: '' as string,
    refreshToken: '' as string,
    accessExpiresAt: '' as string,
    refreshExpiresAt: '' as string,
    initialized: false
  }),
  getters: {
    isAuthenticated: (state) => !!state.accessToken && !!state.user,
    accessExpired: (state) =>
      !!state.accessExpiresAt && new Date(state.accessExpiresAt).getTime() < Date.now()
  },
  actions: {
    setSession(payload: SessionPayload) {
      this.user = payload.user;
      this.accessToken = payload.access_token;
      this.refreshToken = payload.refresh_token;
      this.accessExpiresAt = payload.access_expires_at || '';
      this.refreshExpiresAt = payload.refresh_expires_at || '';
      this.persist();
    },
    persist() {
      const stored: StoredSession = {
        user: this.user,
        accessToken: this.accessToken,
        refreshToken: this.refreshToken,
        accessExpiresAt: this.accessExpiresAt,
        refreshExpiresAt: this.refreshExpiresAt
      };
      setJSON(KEYS.AUTH_SESSION, stored);
    },
    loadFromStorage() {
      if (this.initialized) return;
      const stored = getJSON<StoredSession | null>(KEYS.AUTH_SESSION, null);
      if (stored) {
        this.user = stored.user;
        this.accessToken = stored.accessToken || '';
        this.refreshToken = stored.refreshToken || '';
        this.accessExpiresAt = stored.accessExpiresAt || '';
        this.refreshExpiresAt = stored.refreshExpiresAt || '';
      }
      this.initialized = true;
    },
    clear() {
      this.user = null;
      this.accessToken = '';
      this.refreshToken = '';
      this.accessExpiresAt = '';
      this.refreshExpiresAt = '';
      remove(KEYS.AUTH_SESSION);
    },
    logout() {
      this.clear();
    },
    async login(identifier: string, password: string) {
      const res = await http.post('/auth/login', { identifier, password });
      this.setSession(res.data as SessionPayload);
      return res.data;
    },
    async setup(username: string, password: string, email?: string) {
      const res = await http.post('/auth/setup', { username, password, email });
      this.setSession(res.data as SessionPayload);
      return res.data;
    },
    async refresh() {
      if (!this.refreshToken) return null;
      try {
        const res = await http.post(
          '/auth/refresh',
          { refresh_token: this.refreshToken },
          { _skipAuthRetry: true } as any
        );
        this.setSession(res.data as SessionPayload);
        return this.accessToken;
      } catch (err) {
        this.clear();
        return null;
      }
    },
    async fetchProfile() {
      try {
        const res = await http.get('/auth/me');
        this.user = res.data.user;
        this.persist();
        return res.data.user as UserInfo;
      } catch (err) {
        return null;
      }
    },
    async ensureSession() {
      this.loadFromStorage();
      if (this.accessExpired && this.refreshToken) {
        await this.refresh();
      }
      return this.isAuthenticated;
    },
    async updateProfile(payload: { username: string; email?: string; current_password?: string; new_password?: string }) {
      const res = await http.put('/auth/profile', payload);
      this.setSession(res.data as SessionPayload);
      return res.data;
    }
  }
});

// Token 存取：使用 localStorage 保存 access / refresh token。
const ACCESS_KEY = 'flymail_access'
const REFRESH_KEY = 'flymail_refresh'

export const auth = {
  get access(): string | null {
    return localStorage.getItem(ACCESS_KEY)
  },

  get refresh(): string | null {
    return localStorage.getItem(REFRESH_KEY)
  },

  /** 是否已登录（存在 access token）。 */
  isAuthenticated(): boolean {
    return !!localStorage.getItem(ACCESS_KEY)
  },

  /** 保存登录后返回的 token 对。 */
  set(accessToken: string, refreshToken: string): void {
    localStorage.setItem(ACCESS_KEY, accessToken)
    localStorage.setItem(REFRESH_KEY, refreshToken)
  },

  /** 清除所有 token（退出登录）。 */
  clear(): void {
    localStorage.removeItem(ACCESS_KEY)
    localStorage.removeItem(REFRESH_KEY)
  },
}

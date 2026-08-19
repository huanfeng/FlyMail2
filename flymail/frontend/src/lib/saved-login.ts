// 「记住密码」持久化：登录页勾选后保存用户名/密码，下次打开自动填充。
// 密码仅做 base64 混淆（防肩窥，非加密）——自托管/桌面本地场景的取舍；
// 取消勾选并登录后即清除。
export interface SavedLogin {
  username: string
  password: string
}

const KEY = 'flymail_saved_login'

/** UTF-8 安全的 base64 编码（密码可能含非 ASCII 字符）。 */
function encode(s: string): string {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  bytes.forEach((b) => {
    bin += String.fromCharCode(b)
  })
  return btoa(bin)
}

function decode(s: string): string {
  const bin = atob(s)
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

export const savedLogin = {
  /** 读取已保存的凭据；不存在或数据损坏时返回 null（损坏则顺带清除）。 */
  load(): SavedLogin | null {
    const raw = localStorage.getItem(KEY)
    if (!raw) return null
    try {
      const parsed = JSON.parse(raw) as { username?: unknown; password?: unknown }
      if (typeof parsed.username !== 'string' || typeof parsed.password !== 'string') {
        throw new Error('bad shape')
      }
      return { username: parsed.username, password: decode(parsed.password) }
    } catch {
      localStorage.removeItem(KEY)
      return null
    }
  },

  save(cred: SavedLogin): void {
    localStorage.setItem(
      KEY,
      JSON.stringify({ username: cred.username, password: encode(cred.password) }),
    )
  },

  clear(): void {
    localStorage.removeItem(KEY)
  },
}

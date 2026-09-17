/**
 * 登录重定向的「来路」参数。
 *
 * ── 为什么需要 ───────────────────────────────────────────────────────────────
 *
 * 通知里的「打开邮件」链接是给**别的设备**用的：在飞书里看到提醒，用手机浏览器
 * 或另一台电脑点开。那个浏览器多半没有登录态，于是第一跳必然撞上登录守卫。
 * 守卫要是直接 `Navigate to="/login"`，URL 上的 account/folder/message 就全没了，
 * 登录完落在默认收件箱——用户点了链接，却没打开那封邮件，而且没有任何提示。
 * 会话过期那条路径（401 → 刷新失败 → 整页跳登录）也一样。
 *
 * ⚠ 回跳目标必须只接受**站内相对路径**，否则就是一个开放重定向：
 * `/login?next=https://evil.example.com` 会让我们自己的域名把人送去钓鱼页，
 * 而用户看到的是从可信站点跳过去的。
 */

/** 把当前位置编码成 next 参数的值。 */
export function encodeNext(pathname: string, search: string): string {
  return encodeURIComponent(pathname + search)
}

/**
 * 校验并还原 next 参数；不可信则返回 null（调用方回落到首页）。
 */
export function safeNext(raw: string | null | undefined): string | null {
  if (raw == null || raw === '') return null

  let v: string
  try {
    v = decodeURIComponent(raw)
  } catch {
    // 单个 % 之类的畸形编码
    return null
  }

  // 必须是站内绝对路径。`https://evil.com`、`evil.com` 都在这里被挡掉。
  if (!v.startsWith('/')) return null
  // ⚠ `//evil.com` 是协议相对 URL，浏览器会当成跨站地址。它以 `/` 开头，
  // 光看第一个字符是挡不住的。`/\evil.com` 同理——多数浏览器把 `\` 当 `/`。
  if (v.startsWith('//') || v.startsWith('/\\')) return null
  // 控制字符（换行、制表、NUL）可能被用来绕过上面几条。
  // 这里按字符码判而不是写正则：正则里的控制字符要么写成转义序列（容易被
  // 工具链还原成真的控制字节，源文件会被 git 当二进制），要么就是裸字节。
  for (const ch of v) {
    const code = ch.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f) return null
  }
  // 回登录页本身没意义，会绕成一个圈
  if (v === '/login' || v.startsWith('/login?')) return null

  return v
}

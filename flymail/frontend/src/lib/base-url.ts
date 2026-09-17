/**
 * 「对外访问地址」的校验。
 *
 * 与后端 setting.NormalizeBaseURL 同一套判据，前端先拦一道只是为了少一次往返；
 * 真正作数的是后端那一遍（接口可以被直接调用）。
 *
 * ⚠ 不能只靠 `new URL()`：它是 WHATWG 解析器，对 http/https 这类特殊 scheme 会
 * **自动补全斜杠**，把 `http:/mail.example.com`（少打一个斜杠，很常见的手误）
 * 和 `http:mail.example.com` 都解析成 `http://mail.example.com/`。而后端的
 * Go url.Parse 认为这两个的 Host 是空的，会拒。分歧的方向是「前端放行、后端拒绝」，
 * 用户点保存会撞上一个自己看不懂的 400。所以 scheme 与 `//host` 这两段在这里
 * 自己判，只把剩下的交给 URL 构造函数。
 */
export type BaseUrlVerdict = 'ok' | 'scheme' | 'host' | 'invalid'

/** scheme 必须字母开头——所以 `192.168.5.11:8086` 里的 `192` 不算 scheme。 */
const SCHEME_RE = /^([a-zA-Z][a-zA-Z0-9+.-]*):/

export function checkBaseUrl(raw: string): BaseUrlVerdict {
  const v = raw.trim()
  // 空串合法：表示没配，通知就不带链接
  if (v === '') return 'ok'

  const m = SCHEME_RE.exec(v)
  // 没有 scheme 的 `mail.example.com` 看起来最像"对的"，拼出来却是死链
  if (m == null) return 'scheme'
  const scheme = m[1].toLowerCase()
  if (scheme !== 'http' && scheme !== 'https') return 'scheme'
  // 有 scheme 但缺 `//主机名`：http:// 后面空着、http:/host、https:///path
  if (!/^https?:\/\/[^/?#]/i.test(v)) return 'host'

  let u: URL
  try {
    u = new URL(v)
  } catch {
    return 'invalid'
  }
  if (u.hostname === '') return 'host'
  return 'ok'
}

// mailto: URI 解析（RFC 6068）。
//
// 用途：正文 iframe 里的 mailto 链接被注入脚本拦下并上报给父窗口，
// 父窗口据此打开应用自己的撰写器——而不是把地址甩给系统默认邮件程序
// （在一个邮件客户端里点收件人地址却弹出别的客户端，是明显的断裂）。

/** mailto URI 里能用上的字段；未出现的字段为空数组/空串，调用方不必判 undefined */
export interface MailtoFields {
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  body: string
}

/**
 * 百分号解码。
 *
 * ⚠ 不做 `+` → 空格 的替换：那是 application/x-www-form-urlencoded 的规矩，
 * RFC 6068 的 mailto 查询串不适用。按表单规矩解会把 `a+b@x.com` 拆成 `a b@x.com`，
 * 而带 `+` 的邮箱地址（Gmail 别名）在现实里到处都是。
 *
 * 非法百分号序列（`%zz`）会让 decodeURIComponent 抛异常，这时原样返回：
 * 一个没解码干净的主题，好过整条链接失效。
 */
function decode(s: string): string {
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

/**
 * 剥掉控制字符（C0 与 DEL）。
 *
 * ⚠ 这不是洁癖。mailto 的百分号编码可以还原出 CR 与 LF，而这里解析出来的地址与主题
 * 会一路走进撰写器、再进 POST /messages/send 的 JSON 里。一旦后端或中间某层把它们
 * 拼进 RFC 5322 头部，`a@x.com%0D%0ABcc:%20victim@y.com` 就是一次头部注入——
 * 一封由邮件正文里的链接决定收件人的信。在最外层入口剥掉，比指望下游每一层都记得防要可靠。
 *
 * @param keepNewlines 正文保留换行（那是 mailto body 的正当用法，渲染时会转成 <br>）
 */
function stripControlChars(s: string, keepNewlines = false): string {
  // 逐字符过滤而不是写正则：控制字符的正则字面量会触发 no-control-regex，
  // 而给它挂一行 eslint-disable 只是把「这里有意为之」藏进注释里，不如直接写清楚判断。
  let out = ''
  for (const ch of s) {
    const code = ch.codePointAt(0) ?? 0
    // 制表与换行在正文里是正当内容，其余 C0 控制字符与 DEL 一律剥掉
    if (code === 0x09 || code === 0x0a || code === 0x0d) {
      if (keepNewlines) out += ch
      continue
    }
    if (code < 0x20 || code === 0x7f) continue
    out += ch
  }
  return out
}

/** 逗号分隔的地址串拆成数组，顺带去掉空项与首尾空白 */
function splitAddresses(raw: string): string[] {
  return raw
    .split(',')
    .map((a) => stripControlChars(decode(a)).trim())
    .filter((a) => a.length > 0)
}

/**
 * 解析 mailto URI。
 *
 * 不是 mailto、或者收件人与全部字段都为空时返回 null——
 * 那种链接打开一个空白撰写器只会让人困惑，不如什么都不做。
 */
export function parseMailto(href: string): MailtoFields | null {
  if (typeof href !== 'string') return null
  const trimmed = href.trim()
  if (!/^mailto:/i.test(trimmed)) return null

  const rest = trimmed.slice('mailto:'.length)
  const qIdx = rest.indexOf('?')
  const path = qIdx >= 0 ? rest.slice(0, qIdx) : rest
  const query = qIdx >= 0 ? rest.slice(qIdx + 1) : ''

  const fields: MailtoFields = {
    to: splitAddresses(path),
    cc: [],
    bcc: [],
    subject: '',
    body: '',
  }

  for (const pair of query.split('&')) {
    if (!pair) continue
    const eq = pair.indexOf('=')
    const key = (eq >= 0 ? pair.slice(0, eq) : pair).toLowerCase()
    const value = eq >= 0 ? pair.slice(eq + 1) : ''
    switch (key) {
      // hfields 里的 to 与路径部分的收件人是叠加关系，不是覆盖
      case 'to':
        fields.to.push(...splitAddresses(value))
        break
      case 'cc':
        fields.cc.push(...splitAddresses(value))
        break
      case 'bcc':
        fields.bcc.push(...splitAddresses(value))
        break
      case 'subject':
        fields.subject = stripControlChars(decode(value))
        break
      case 'body':
        // 正文保留换行（mailto body 的正当用法），其余控制字符照剥
        fields.body = stripControlChars(decode(value), true)
        break
      default:
        // in-reply-to / references 等其余 hfield 一律忽略：
        // 让不可信的邮件正文往我们的撰写器里塞任意头部是没必要的攻击面
        break
    }
  }

  if (fields.to.length === 0 && fields.cc.length === 0 && !fields.subject && !fields.body) {
    return null
  }
  return fields
}

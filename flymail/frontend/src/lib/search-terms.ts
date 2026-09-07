// 从搜索串里提取「值得在列表行上高亮的词」。
//
// 后端把 q 解析成 FTS5 查询，支持 Gmail 风格限定符（from: / is: / before: …）。
// 前端不重复那套解析，只回答一个更窄的问题：哪些片段会以**文本**的形式
// 出现在 subject / from_name / snippet 里？
//   · from: / to: / subject: 的取值是文本 → 要高亮
//   · is:unread / has:attachment / before:2026-01 / in:inbox / account:… 是结构化条件，
//     它们的取值压根不出现在邮件文本里，高亮了只会在正文中误标 "unread"、"inbox" 这类词
//   · 未知限定符（foo:bar）后端会退化成普通文本，这里也照样当自由词处理
//
// 导出的 term 数组直接喂给 lib/highlight.ts 的切片函数。

/** 取值是文本、会出现在列表可见字段里的限定符 */
const TEXT_QUALIFIERS = new Set(['from', 'to', 'subject'])

/** 后端认识的全部限定符；不在此表内的 `x:y` 会被后端当普通文本 */
const KNOWN_QUALIFIERS = new Set([
  'from', 'to', 'subject', 'has', 'is', 'before', 'after', 'in', 'account',
])

/**
 * 读一个 token：以空白分隔，但引号内的空白不算分隔符。
 * 引号原样保留在 raw 里，由后续 unquote 统一剥离——因为要先判断冒号
 * 是否落在引号内（`"a:b"` 是短语，不是限定符）。
 */
function readToken(input: string, start: number): { raw: string; next: number } {
  let i = start
  let inQuote = false
  let raw = ''
  while (i < input.length) {
    const ch = input[i]
    if (ch === '"') {
      inQuote = !inQuote
      raw += ch
      i++
      continue
    }
    if (!inQuote && /\s/.test(ch)) break
    raw += ch
    i++
  }
  return { raw, next: i }
}

/** 找到第一个位于引号外的冒号；没有返回 -1 */
function outerColonIndex(raw: string): number {
  let inQuote = false
  for (let i = 0; i < raw.length; i++) {
    const ch = raw[i]
    if (ch === '"') inQuote = !inQuote
    else if (ch === ':' && !inQuote) return i
  }
  return -1
}

/** 剥掉双引号（短语的边界标记，不参与文本匹配） */
function unquote(s: string): string {
  return s.replace(/"/g, '').trim()
}

/**
 * 从查询串提取高亮词。
 *
 * 返回值已去重（大小写不敏感）、去空，并按长度降序排列——
 * 长词必须先匹配，否则 "发票" 会把 "电子发票" 切成两段，高亮看起来是碎的。
 */
export function extractHighlightTerms(q: string): string[] {
  if (!q) return []

  const terms: string[] = []
  let i = 0
  while (i < q.length) {
    // 跳过分隔空白
    if (/\s/.test(q[i])) { i++; continue }

    const { raw, next } = readToken(q, i)
    i = next
    if (!raw) continue

    const colon = outerColonIndex(raw)
    if (colon > 0) {
      const name = raw.slice(0, colon).toLowerCase()
      if (KNOWN_QUALIFIERS.has(name)) {
        // 结构化限定符：只有文本类的取值需要高亮，其余整条丢弃
        if (TEXT_QUALIFIERS.has(name)) terms.push(unquote(raw.slice(colon + 1)))
        continue
      }
      // 未知限定符：与后端一致，整体退化为自由文本（连冒号一起匹配）
    }
    terms.push(unquote(raw))
  }

  // 去重按小写键，保留首次出现的原始大小写（高亮本身大小写不敏感）
  const seen = new Set<string>()
  const unique: string[] = []
  for (const t of terms) {
    if (!t) continue
    const key = t.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    unique.push(t)
  }

  return unique.sort((a, b) => b.length - a.length)
}

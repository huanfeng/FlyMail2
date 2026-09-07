// 命中高亮的**纯切片逻辑**：把一段文本按命中词切成 [普通|命中] 段。
//
// 刻意与渲染分离（渲染见 components/ui/Highlight.tsx）：切片是纯字符串运算，
// 单测不必挂 DOM；React 那边只负责把段落映射成 <mark>。

/** 一段文本切片：hit 为 true 表示这段命中了搜索词 */
export interface HighlightSegment {
  text: string
  hit: boolean
}

/** 转义正则元字符——搜索词来自用户输入，`c++`、`a.b` 不能被当成模式 */
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * 按 terms 切分 text，大小写不敏感。
 *
 * · text 为空 → 返回空数组（调用方不必再判空）
 * · terms 为空或全无命中 → 返回单段未命中，调用方可据此跳过包装节点
 * · terms 需按长度降序传入（extractHighlightTerms 已保证）：正则 `|` 在同一位置
 *   取先写的分支，短词在前会把长词切碎
 * · 相邻命中段会合并，避免 "abab" 被拆成四个 <mark>
 */
export function splitHighlight(text: string, terms: string[]): HighlightSegment[] {
  if (!text) return []
  const valid = terms.filter((t) => t.length > 0)
  if (valid.length === 0) return [{ text, hit: false }]

  const re = new RegExp(valid.map(escapeRegExp).join('|'), 'gi')
  const out: HighlightSegment[] = []
  let last = 0

  const push = (chunk: string, hit: boolean) => {
    if (!chunk) return
    const prev = out[out.length - 1]
    if (prev && prev.hit === hit) prev.text += chunk
    else out.push({ text: chunk, hit })
  }

  for (let m = re.exec(text); m !== null; m = re.exec(text)) {
    // 零长匹配理论上不会出现（空词已过滤），但真出现会死循环，兜一手
    if (m[0].length === 0) { re.lastIndex++; continue }
    push(text.slice(last, m.index), false)
    push(m[0], true)
    last = m.index + m[0].length
  }
  push(text.slice(last), false)

  return out
}

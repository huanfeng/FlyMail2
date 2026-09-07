// 搜索命中高亮的渲染层：把 lib/highlight.ts 的切片映射成 <mark className="hl">。
// 切片逻辑不在这里——那边是纯函数，单测不需要 DOM。

import { Fragment, type ReactNode } from 'react'
import { splitHighlight } from '@/lib/highlight'

/**
 * 高亮 text 中命中 terms 的部分。
 *
 * 无词或无命中时**原样返回字符串**：列表每屏几十行、每行三处调用，
 * 非搜索态不该为此多出成千上万个 Fragment 节点。
 */
export function highlightText(text: string, terms: string[]): ReactNode {
  if (!text || terms.length === 0) return text

  const segments = splitHighlight(text, terms)
  if (!segments.some((s) => s.hit)) return text

  return segments.map((s, i) =>
    s.hit
      ? <mark key={i} className="hl">{s.text}</mark>
      : <Fragment key={i}>{s.text}</Fragment>,
  )
}

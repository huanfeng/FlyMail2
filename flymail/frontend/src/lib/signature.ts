// 签名：决定"插不插"和"插什么"，以及在一段正文 HTML 里精确定位签名块。
//
// 关键约束来自 docs/flymail/m13-composer.md：**切换发件人时整节点替换，不能误伤正文**。
// 所以签名在文档里不是一段普通 HTML，而是一个带 `data-signature` 标记的块——
// 有了标记才谈得上"找到旧的、换成新的"；靠字符串查找匹配签名内容，
// 用户但凡在正文里引用过自己的签名文字，就会被替换掉半句话。

import type { Signature } from '@/lib/types'

/** 签名块在 DOM / HTML 里的标记属性 */
export const SIGNATURE_ATTR = 'data-signature'

/** 撰写场景：新建信 / 回复或转发 */
export type ComposeScenario = 'new' | 'reply'

/**
 * 这个场景下该插入的签名 HTML；不该插入时返回空串。
 *
 * 空白签名（只有空标签）按"没配置"处理：用户把签名清空就是不想要它，
 * 而一个空的 `<div data-signature>` 留在正文里会在收件方那儿多出一个空行。
 */
export function signatureForScenario(
  sig: Signature | null | undefined,
  scenario: ComposeScenario,
): string {
  if (!sig) return ''
  const enabled = scenario === 'reply' ? sig.use_on_reply : sig.use_on_new
  if (!enabled) return ''
  return isBlankHtml(sig.body_html) ? '' : sig.body_html
}

/** HTML 去掉标签和空白后是不是空的 */
export function isBlankHtml(html: string | undefined | null): boolean {
  if (!html) return true
  const text = html
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/gi, ' ')
    .trim()
  // 只有图片也算有内容（有人的签名就是一张图）
  if (text.length > 0) return false
  return !/<img\b/i.test(html)
}

/** 包一层签名标记 */
export function wrapSignature(html: string): string {
  return `<div ${SIGNATURE_ATTR}="true">${html}</div>`
}

/**
 * 在一段正文 HTML 里把签名块换成新的；没有旧签名就追加到末尾。
 *
 * 传空串表示"去掉签名"。这是 HTML 字符串层面的实现，用于草稿等不经编辑器的路径；
 * 编辑器内的替换走 ProseMirror 事务（见 RichEditor.applySignature），
 * 那条路径能保住撤销栈和光标，这条不能——两条路径的判定逻辑必须一致，所以都以本文件为准。
 */
export function replaceSignatureHtml(bodyHtml: string, signatureHtml: string): string {
  if (typeof DOMParser === 'undefined') return bodyHtml
  const doc = new DOMParser().parseFromString(bodyHtml, 'text/html')
  const existing = doc.body.querySelector(`[${SIGNATURE_ATTR}]`)

  if (!signatureHtml) {
    existing?.parentNode?.removeChild(existing)
    return doc.body.innerHTML
  }

  const holder = doc.createElement('div')
  holder.setAttribute(SIGNATURE_ATTR, 'true')
  holder.innerHTML = signatureHtml

  if (existing) existing.parentNode?.replaceChild(holder, existing)
  else doc.body.appendChild(holder)
  return doc.body.innerHTML
}

/** 取出正文里现有的签名块内容；没有则返回 null */
export function extractSignatureHtml(bodyHtml: string): string | null {
  if (typeof DOMParser === 'undefined') return null
  const doc = new DOMParser().parseFromString(bodyHtml, 'text/html')
  const existing = doc.body.querySelector(`[${SIGNATURE_ATTR}]`)
  return existing ? existing.innerHTML : null
}

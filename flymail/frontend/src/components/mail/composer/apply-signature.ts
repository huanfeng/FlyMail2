// 在 ProseMirror 文档里换掉签名块。
//
// 走事务而不是 setContent(整篇 HTML)：后者会清空撤销栈、丢掉光标，而切换发件人
// 是写信写到一半才会发生的事——正文没了比签名没换更严重。
//
// 与 lib/signature.ts 的 replaceSignatureHtml 是同一套判定，只是作用在文档而非字符串上：
// 那条路径给不经编辑器的场合（草稿）兜底，这条是界面真正走的。

import type { Editor } from '@tiptap/core'
import { wrapSignature } from '@/lib/signature'

/**
 * 找到文档顶层某类节点的位置。
 *
 * 只扫顶层：签名和引用块按设计都是顶层块，往下递归只会找到用户自己粘进来的
 * 同名结构（比如把上一封带签名的信整段引用进来），换错地方比找不到更糟。
 */
export function findTopLevelNode(editor: Editor, name: string): { pos: number; size: number } | null {
  let found: { pos: number; size: number } | null = null
  editor.state.doc.forEach((node, offset) => {
    if (found === null && node.type.name === name) found = { pos: offset, size: node.nodeSize }
  })
  return found
}

/**
 * 用新签名整块替换旧签名；`signatureHtml` 为空表示去掉签名。
 *
 * 没有旧签名时插在引用块之前——回复里签名应当在引用原文的上方，
 * 追加到文末会把它埋在几十行历史往返的后面。
 */
export function applySignatureToEditor(editor: Editor, signatureHtml: string): void {
  const existing = findTopLevelNode(editor, 'signature')

  if (!signatureHtml) {
    if (existing) {
      editor.chain().deleteRange({ from: existing.pos, to: existing.pos + existing.size }).run()
    }
    return
  }

  const content = wrapSignature(signatureHtml)
  if (existing) {
    // 整块换掉：用户在正文里写的东西一个字都不碰
    editor
      .chain()
      .insertContentAt({ from: existing.pos, to: existing.pos + existing.size }, content, {
        updateSelection: false,
      })
      .run()
    return
  }

  const quote = findTopLevelNode(editor, 'quoteBlock')
  const at = quote ? quote.pos : editor.state.doc.content.size
  editor.chain().insertContentAt(at, content, { updateSelection: false }).run()
}

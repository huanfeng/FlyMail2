// 渲染层：给 schema 里的引用块挂上 React NodeView（折叠头）。
//
// schema 本身在 schema.ts，不含 React——那一层要能在 jsdom 里裸跑单测。
// 这里只做一件事：把同一个节点包上界面。

import { ReactNodeViewRenderer } from '@tiptap/react'
import type { AnyExtension } from '@tiptap/core'
import { QuoteBlockView } from '@/components/mail/composer/QuoteBlockView'
import { QuoteBlockBase, composerExtensions } from '@/components/mail/composer/schema'

/** 带折叠 NodeView 的引用块 */
export const QuoteBlock = QuoteBlockBase.extend({
  addNodeView() {
    return ReactNodeViewRenderer(QuoteBlockView)
  },
})

/** 撰写器实际使用的扩展列表 */
export const EDITOR_EXTENSIONS: AnyExtension[] = composerExtensions(QuoteBlock)

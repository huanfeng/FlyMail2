// 撰写器的 ProseMirror schema —— 不含任何 React。
//
// 拆出这一层的理由是可测：schema 决定了"从 Outlook 粘过来的字号会不会被吃掉"
// 这类最要命的行为，而它必须能在 jsdom 里被直接实例化验证。NodeView 是渲染层的事
// （见 extensions.ts），混在一起就只能靠人工点界面来验收。

import { Node, mergeAttributes } from '@tiptap/core'
import type { AnyExtension } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import Highlight from '@tiptap/extension-highlight'
import { TableKit } from '@tiptap/extension-table'
import { BackgroundColor, Color, FontFamily, FontSize, TextStyle } from '@tiptap/extension-text-style'
import { SIGNATURE_ATTR } from '@/lib/signature'

/** 引用块在 HTML 里的标记属性；回复/转发预填时由 compose-prefill 写上 */
export const QUOTE_BLOCK_ATTR = 'data-quote-block'

/** 引用块的行内样式，随邮件发出去，收件方看到的就是常见的左边线引用样式 */
const QUOTE_STYLE = 'border-left:2px solid #ccc;padding-left:10px;color:#666'

/**
 * 引用块（无 NodeView 版本）。
 *
 * parseHTML 的 priority 必须高于默认值：`blockquote[data-quote-block]` 同时也匹配
 * StarterKit 的 blockquote 规则，优先级相同的话谁先注册谁赢，那就成了玄学。
 */
export const QuoteBlockBase = Node.create({
  name: 'quoteBlock',
  group: 'block',
  content: 'block+',
  defining: true,

  parseHTML() {
    return [{ tag: `blockquote[${QUOTE_BLOCK_ATTR}]`, priority: 100 }]
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'blockquote',
      mergeAttributes(HTMLAttributes, { [QUOTE_BLOCK_ATTR]: 'true', style: QUOTE_STYLE }),
      0,
    ]
  },
})

/**
 * 签名块。
 *
 * 没有 NodeView——签名就是一段普通可编辑内容，用户想在里面改字随时可以改。
 * 它需要的只是一个"我是签名"的身份，好让切换发件人时能整块替换掉。
 */
export const SignatureBlock = Node.create({
  name: 'signature',
  group: 'block',
  content: 'block+',
  defining: true,

  parseHTML() {
    return [{ tag: `div[${SIGNATURE_ATTR}]`, priority: 100 }]
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'div',
      mergeAttributes(HTMLAttributes, { [SIGNATURE_ATTR]: 'true', class: 'mail-signature' }),
      0,
    ]
  },
})

/**
 * 撰写器的完整扩展列表。
 *
 * `quoteBlock` 由调用方传入：界面上用带 NodeView 的版本（能折叠），
 * 测试里用 QuoteBlockBase（不需要 React 渲染），两者 schema 完全一致。
 */
export function composerExtensions(quoteBlock: AnyExtension = QuoteBlockBase): AnyExtension[] {
  return [
    StarterKit.configure({
      // link 单独配置（要管协议白名单），underline / 其余保持 StarterKit 默认
      link: false,
      // 邮件正文里 h1 太重，只开到 h3
      heading: { levels: [1, 2, 3] },
    }),
    Link.configure({
      openOnClick: false,
      autolink: true,
      // 邮件里出现的链接协议就这几种；其余（javascript:、data: …）一律不认
      protocols: ['http', 'https', 'mailto'],
    }),
    Image.configure({ inline: false, allowBase64: true }),
    Highlight.configure({ multicolor: true }),
    TableKit.configure({ table: { resizable: true } }),
    // TextStyle 是载体，Color / FontSize / BackgroundColor / FontFamily 往它上面挂属性。
    // 少了 TextStyle，从 Outlook 粘过来的 span[style] 会被 schema 整个丢掉，
    // 表现就是"粘贴后字号和颜色全没了"。
    TextStyle,
    Color,
    FontSize,
    BackgroundColor,
    FontFamily,
    quoteBlock,
    SignatureBlock,
  ]
}

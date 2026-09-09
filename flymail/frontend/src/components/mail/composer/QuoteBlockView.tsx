// 引用块的 NodeView：折叠头 + 可编辑内容。
//
// 折叠状态刻意只放在 React 本地 state，不写进文档属性——展开/收起是"看"的动作，
// 不是"改"的动作。写进文档就会进撤销栈（Ctrl+Z 先撤销一次展开，很莫名其妙），
// 还会让"内容没改过"的判断失真。
//
// 收起时用 display:none 而不是把节点从文档里摘掉：`editor.getHTML()` 走的是
// renderHTML 而非 DOM，所以无论展开与否，发出去的正文都完整包含引用内容。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { NodeViewContent, NodeViewWrapper } from '@tiptap/react'
import { Icon } from '@/components/ui/Icon'

export function QuoteBlockView() {
  const { t } = useTranslation()
  const [expanded, setExpanded] = React.useState(false)

  return (
    <NodeViewWrapper className="quote-block" data-expanded={expanded ? 'true' : 'false'}>
      <button
        type="button"
        className="quote-toggle"
        contentEditable={false}
        // NodeView 里的交互元素必须挡住 mousedown，否则 ProseMirror 会先抢走选区
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setExpanded((v) => !v)}
        title={expanded ? t('compose.editor.quoteHide') : t('compose.editor.quoteShow')}
      >
        <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={12} />
        <span>{expanded ? t('compose.editor.quoteHide') : t('compose.editor.quoteShow')}</span>
      </button>
      <NodeViewContent
        className="quote-content"
        style={{ display: expanded ? 'block' : 'none' }}
      />
    </NodeViewWrapper>
  )
}

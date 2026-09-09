// 富文本工具栏。
//
// 激活态靠 useEditorState 订阅：直接读 editor.isActive() 不会触发重渲染
// （useEditor 关了 shouldRerenderOnTransaction，就是为了不让每次按键都重画整个撰写器），
// 所以按钮的高亮必须走 selector 显式声明依赖。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useEditorState } from '@tiptap/react'
import type { Editor } from '@tiptap/react'
import { Icon } from '@/components/ui/Icon'

/** 字号档位。留空 = 跟随默认，不写 font-size。 */
const FONT_SIZES = ['12px', '14px', '16px', '18px', '24px', '32px']

/** 高亮色：浅黄，深色主题下也还能看清底下的字 */
const HIGHLIGHT_COLOR = '#fff3a3'

interface EditorToolbarProps {
  editor: Editor
  disabled?: boolean
  /** 打开文件选择器插入图片 */
  onPickImage: () => void
}

interface ToolButtonProps {
  active?: boolean
  disabled?: boolean
  title: string
  onClick: () => void
  children: React.ReactNode
}

function ToolButton({ active, disabled, title, onClick, children }: ToolButtonProps) {
  return (
    <button
      type="button"
      className={'rt-btn' + (active ? ' active' : '')}
      title={title}
      aria-label={title}
      aria-pressed={active ? true : undefined}
      disabled={disabled}
      // 不让按钮抢走编辑器焦点，否则 toggle 完选区就没了
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      {children}
    </button>
  )
}

export function EditorToolbar({ editor, disabled, onPickImage }: EditorToolbarProps) {
  const { t } = useTranslation()

  const state = useEditorState({
    editor,
    selector: ({ editor: ed }) => ({
      bold: ed.isActive('bold'),
      italic: ed.isActive('italic'),
      underline: ed.isActive('underline'),
      strike: ed.isActive('strike'),
      bulletList: ed.isActive('bulletList'),
      orderedList: ed.isActive('orderedList'),
      link: ed.isActive('link'),
      highlight: ed.isActive('highlight'),
      inTable: ed.isActive('table'),
      fontSize: (ed.getAttributes('textStyle').fontSize as string | undefined) ?? '',
      color: (ed.getAttributes('textStyle').color as string | undefined) ?? '',
    }),
  })

  function setFontSize(value: string) {
    if (value) editor.chain().focus().setFontSize(value).run()
    else editor.chain().focus().unsetFontSize().run()
  }

  function handleLink() {
    if (state.link) {
      editor.chain().focus().unsetLink().run()
      return
    }
    const prev = (editor.getAttributes('link').href as string | undefined) ?? ''
    const url = window.prompt(t('compose.editor.linkPrompt'), prev)
    if (url === null) return
    const trimmed = url.trim()
    if (!trimmed) {
      editor.chain().focus().unsetLink().run()
      return
    }
    // 不带协议的输入按 https 处理；Link 扩展会拒绝 javascript: 之类的危险协议
    const href = /^[a-z][a-z0-9+.-]*:/i.test(trimmed) ? trimmed : `https://${trimmed}`
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run()
  }

  return (
    <div className="rich-toolbar" role="toolbar" aria-label={t('compose.editor.toolbar')}>
      <ToolButton
        title={t('compose.editor.bold')}
        active={state.bold}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleBold().run()}
      >
        <span style={{ fontWeight: 700 }}>B</span>
      </ToolButton>
      <ToolButton
        title={t('compose.editor.italic')}
        active={state.italic}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      >
        <span style={{ fontStyle: 'italic', fontFamily: 'serif' }}>I</span>
      </ToolButton>
      <ToolButton
        title={t('compose.editor.underline')}
        active={state.underline}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleUnderline().run()}
      >
        <span style={{ textDecoration: 'underline' }}>U</span>
      </ToolButton>
      <ToolButton
        title={t('compose.editor.strike')}
        active={state.strike}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleStrike().run()}
      >
        <span style={{ textDecoration: 'line-through' }}>S</span>
      </ToolButton>

      <span className="rt-sep" />

      {/* 字号 */}
      <select
        className="rt-select"
        value={state.fontSize}
        disabled={disabled}
        title={t('compose.editor.fontSize')}
        aria-label={t('compose.editor.fontSize')}
        onMouseDown={(e) => e.stopPropagation()}
        onChange={(e) => setFontSize(e.target.value)}
      >
        <option value="">{t('compose.editor.fontSizeDefault')}</option>
        {FONT_SIZES.map((s) => (
          <option key={s} value={s}>{s.replace('px', '')}</option>
        ))}
      </select>

      {/* 文字颜色：input[type=color] 没有"未设置"这个状态，所以旁边补一个清除按钮 */}
      <label className="rt-color" title={t('compose.editor.color')}>
        <span className="rt-color-swatch" style={{ background: state.color || 'var(--ink)' }} />
        <input
          type="color"
          value={state.color || '#000000'}
          disabled={disabled}
          aria-label={t('compose.editor.color')}
          onChange={(e) => editor.chain().focus().setColor(e.target.value).run()}
        />
      </label>
      <ToolButton
        title={t('compose.editor.colorClear')}
        disabled={disabled}
        onClick={() => editor.chain().focus().unsetColor().run()}
      >
        <Icon name="close" size={11} />
      </ToolButton>

      <ToolButton
        title={t('compose.editor.highlight')}
        active={state.highlight}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleHighlight({ color: HIGHLIGHT_COLOR }).run()}
      >
        <span className="rt-highlight-mark">A</span>
      </ToolButton>

      <span className="rt-sep" />

      <ToolButton
        title={t('compose.editor.bulletList')}
        active={state.bulletList}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleBulletList().run()}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
          <circle cx="3" cy="4" r="1" fill="currentColor" stroke="none" />
          <circle cx="3" cy="8" r="1" fill="currentColor" stroke="none" />
          <circle cx="3" cy="12" r="1" fill="currentColor" stroke="none" />
          <path d="M6 4h7M6 8h7M6 12h7" />
        </svg>
      </ToolButton>
      <ToolButton
        title={t('compose.editor.orderedList')}
        active={state.orderedList}
        disabled={disabled}
        onClick={() => editor.chain().focus().toggleOrderedList().run()}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
          <path d="M2 3h1v3M2 6h2" />
          <path d="M2 10h2v1l-2 1v1h2" />
          <path d="M6 4h7M6 8h7M6 12h7" />
        </svg>
      </ToolButton>

      <span className="rt-sep" />

      <ToolButton
        title={state.link ? t('compose.editor.linkRemove') : t('compose.editor.link')}
        active={state.link}
        disabled={disabled}
        onClick={handleLink}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
          <path d="M6.5 9.5a2.5 2.5 0 0 0 3.5 0l2-2a2.5 2.5 0 0 0-3.5-3.5l-.8.8" />
          <path d="M9.5 6.5a2.5 2.5 0 0 0-3.5 0l-2 2a2.5 2.5 0 0 0 3.5 3.5l.8-.8" />
        </svg>
      </ToolButton>

      <ToolButton
        title={t('compose.editor.imageInsert')}
        disabled={disabled}
        onClick={onPickImage}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round">
          <rect x="2" y="3" width="12" height="10" rx="1.5" />
          <path d="M2 10.5l3-3 2.5 2.5 2-2L14 11" />
          <circle cx="6" cy="6" r="1" />
        </svg>
      </ToolButton>

      <ToolButton
        title={t('compose.editor.tableInsert')}
        active={state.inTable}
        disabled={disabled}
        onClick={() => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4">
          <rect x="2" y="3" width="12" height="10" rx="1" />
          <path d="M2 6.5h12M2 10h12M6.5 3v10M10.5 3v10" />
        </svg>
      </ToolButton>

      {/* 表格增删行列：只在光标落在表格里时出现，平时不占地方 */}
      {state.inTable && (
        <>
          <span className="rt-sep" />
          <ToolButton
            title={t('compose.editor.tableAddRow')}
            disabled={disabled}
            onClick={() => editor.chain().focus().addRowAfter().run()}
          >
            <span className="rt-text">+{t('compose.editor.rowShort')}</span>
          </ToolButton>
          <ToolButton
            title={t('compose.editor.tableDeleteRow')}
            disabled={disabled}
            onClick={() => editor.chain().focus().deleteRow().run()}
          >
            <span className="rt-text">-{t('compose.editor.rowShort')}</span>
          </ToolButton>
          <ToolButton
            title={t('compose.editor.tableAddCol')}
            disabled={disabled}
            onClick={() => editor.chain().focus().addColumnAfter().run()}
          >
            <span className="rt-text">+{t('compose.editor.colShort')}</span>
          </ToolButton>
          <ToolButton
            title={t('compose.editor.tableDeleteCol')}
            disabled={disabled}
            onClick={() => editor.chain().focus().deleteColumn().run()}
          >
            <span className="rt-text">-{t('compose.editor.colShort')}</span>
          </ToolButton>
          <ToolButton
            title={t('compose.editor.tableDelete')}
            disabled={disabled}
            onClick={() => editor.chain().focus().deleteTable().run()}
          >
            <Icon name="trash" size={12} />
          </ToolButton>
        </>
      )}

      <span className="rt-sep" />

      <ToolButton
        title={t('compose.editor.clearFormat')}
        disabled={disabled}
        onClick={() => editor.chain().focus().unsetAllMarks().clearNodes().run()}
      >
        <span className="rt-text">Tx</span>
      </ToolButton>
    </div>
  )
}

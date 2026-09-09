// 撰写器的富文本内核（Tiptap / ProseMirror）。
//
// 与被它替换掉的 react-simple-wysiwyg 的根本区别：这里有 **schema**。
// contentEditable 薄包装只有一串 HTML 字符串，所以"折叠引用""换掉签名""把粘进来的
// Word 垃圾扔掉"这三件事都无从下手——你没法指到一段结构。有了 schema 才有节点，
// 有了节点才谈得上定位与替换。
//
// 这个组件对外是**命令式**的：正文不作为受控 state 往上抛。
// 撰写窗口每敲一个键就 setState 一次、再把整篇正文序列化一遍，在带十封引用的回复里
// 是实打实的卡顿；而调用方真正需要正文的时刻只有两个——发送、存草稿。
// 所以正文留在编辑器里，要用时通过 ref 取。

import * as React from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import type { Editor } from '@tiptap/react'
import { applySignatureToEditor } from '@/components/mail/composer/apply-signature'
import { EditorToolbar } from '@/components/mail/composer/EditorToolbar'
import { EDITOR_EXTENSIONS } from '@/components/mail/composer/extensions'
import { cleanPastedHtml } from '@/lib/paste-clean'

/** 调用方拿得到的能力：取正文、换签名、插图、聚焦 */
export interface RichEditorHandle {
  /** 当前正文 HTML（始终包含引用块的完整内容，与折叠与否无关） */
  getHTML: () => string
  /** 用新签名整块替换旧签名；传空串表示去掉签名 */
  applySignature: (signatureHtml: string) => void
  /** 把一批图片文件插到光标处 */
  insertImages: (files: File[]) => void
  focus: () => void
}

export interface RichEditorProps {
  ref?: React.Ref<RichEditorHandle>
  /** 初始正文；只在 resetKey 变化时重新灌入 */
  initialHtml: string
  /**
   * 一次撰写会话的标识。打开新草稿 / 新回复时换一个值，编辑器据此重灌正文；
   * 同一次撰写期间必须保持不变，否则用户打的字会被 initialHtml 冲掉。
   */
  resetKey: string
  editable?: boolean
  /**
   * 登记一张内联图，返回编辑器内用于预览的 blob URL。
   * 返回 null 表示拒绝（超限等），此时这张图不插入。
   */
  registerInlineImage: (file: File) => string | null
  minHeight?: number
}

export function RichEditor({
  ref,
  initialHtml,
  resetKey,
  editable = true,
  registerInlineImage,
  minHeight = 240,
}: RichEditorProps) {
  const editorRef = React.useRef<Editor | null>(null)
  const registerRef = React.useRef(registerInlineImage)
  const fileInputRef = React.useRef<HTMLInputElement>(null)

  React.useEffect(() => { registerRef.current = registerInlineImage }, [registerInlineImage])

  /** 把图片文件插进文档；at 为空则插在当前光标处 */
  const insertImageFiles = React.useCallback((files: File[], at?: number): boolean => {
    const ed = editorRef.current
    if (!ed) return false
    const images = files.filter((f) => f.type.startsWith('image/'))
    if (images.length === 0) return false

    if (at != null) ed.commands.setTextSelection(at)
    let inserted = false
    for (const f of images) {
      const url = registerRef.current(f)
      if (!url) continue
      ed.chain().focus().insertContent({ type: 'image', attrs: { src: url } }).run()
      inserted = true
    }
    return inserted
  }, [])

  const editor = useEditor({
    // 每次按键都重画整个撰写窗口没有意义；工具栏的激活态自己用 useEditorState 订阅。
    shouldRerenderOnTransaction: false,
    content: initialHtml,
    editable,
    extensions: EDITOR_EXTENSIONS,
    editorProps: {
      attributes: {
        class: 'rich-editor-content',
        style: `min-height:${minHeight}px`,
      },
      // schema 只挡不认识的节点，挡不住 <span style="mso-...">；私有样式在这一层清掉。
      transformPastedHTML: (html) => cleanPastedHtml(html),
      handlePaste: (_view, event) => {
        const files = Array.from(event.clipboardData?.files ?? [])
        if (files.length === 0) return false
        if (!insertImageFiles(files)) return false
        event.preventDefault()
        return true
      },
      handleDrop: (view, event, _slice, moved) => {
        // moved = 文档内部拖动已有节点，不是外部文件
        if (moved) return false
        const dragEvent = event as DragEvent
        const files = Array.from(dragEvent.dataTransfer?.files ?? [])
        if (files.length === 0) return false
        const coords = view.posAtCoords({ left: dragEvent.clientX, top: dragEvent.clientY })
        if (!insertImageFiles(files, coords?.pos)) return false
        event.preventDefault()
        return true
      },
    },
  }, [])

  React.useEffect(() => { editorRef.current = editor }, [editor])

  React.useEffect(() => {
    if (editor) editor.setEditable(editable)
  }, [editor, editable])

  // 换一次撰写会话才重灌正文。首挂载已经由 content 选项灌过，这里跳过。
  const loadedKey = React.useRef(resetKey)
  React.useEffect(() => {
    if (!editor) return
    if (loadedKey.current === resetKey) return
    loadedKey.current = resetKey
    editor.commands.setContent(initialHtml, { emitUpdate: false })
  }, [editor, resetKey, initialHtml])

  React.useImperativeHandle(ref, (): RichEditorHandle => ({
    getHTML: () => editorRef.current?.getHTML() ?? '',
    focus: () => editorRef.current?.commands.focus(),
    insertImages: (files) => { insertImageFiles(files) },
    applySignature: (signatureHtml) => {
      const ed = editorRef.current
      if (ed) applySignatureToEditor(ed, signatureHtml)
    },
  }), [insertImageFiles])

  function onPickImage() {
    fileInputRef.current?.click()
  }

  function onFilesPicked(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    e.target.value = '' // 允许再次选择同一张图
    insertImageFiles(files)
  }

  return (
    <div className="rich-editor">
      {editor && <EditorToolbar editor={editor} disabled={!editable} onPickImage={onPickImage} />}
      <EditorContent editor={editor} className="rich-editor-shell" />
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        multiple
        hidden
        onChange={onFilesPicked}
      />
    </div>
  )
}

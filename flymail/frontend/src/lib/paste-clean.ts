// 粘贴清洗：把 Outlook / Gmail / Word 复制出来的 HTML 收拾成 Tiptap schema 认得的样子。
//
// 为什么不能直接交给 ProseMirror：schema 只会丢弃它不认识的**节点**，但
// `<span style="mso-fareast-font-family:等线; font-size:14.0pt">` 这种节点它是认得的
// （span + style → TextStyle mark），于是整坨 Word 私有样式会原样进到我们要发出去的信里。
// 反过来，`<font size="3" color="#f00">` 这种老标签 schema 不认识，直接丢弃 ——
// 用户看到的就是"从 Outlook 粘过来颜色没了"。
//
// 所以这一层的职责只有两件事：
//   1. 把**有语义**的老写法（font 标签、align 属性）翻译成 span[style]，别让格式丢失；
//   2. 把**没语义**的私有垃圾（mso-*、<o:p>、条件注释、class）扔掉，别让它跟着邮件跑。
//
// 纯字符串进、纯字符串出，靠 DOMParser 解析——所以 jsdom 下可直接单测。

/** style 声明白名单。不在表里的属性一律丢弃，`mso-` 前缀因此天然出局。 */
const ALLOWED_STYLE_PROPS = new Set([
  'color',
  'background-color',
  'font-size',
  'font-family',
  'font-style',
  'font-weight',
  'text-align',
  'text-decoration',
  'text-decoration-line',
])

/** 属性白名单（除 style 外）。表格/图片的排版属性保留，其余交给 schema 决定。 */
const ALLOWED_ATTRS = new Set([
  'href',
  'src',
  'alt',
  'title',
  'colspan',
  'rowspan',
  'width',
  'height',
  'target',
  'rel',
  'data-cid',
  'data-quote-block',
  'data-signature',
])

/** 整棵子树都要删掉的标签（内容一并丢弃） */
const DROP_TREE_TAGS = new Set(['script', 'style', 'meta', 'link', 'title', 'xml', 'base', 'noscript'])

/** `<font size>` 的 1–7 → px。HTML4 的相对尺寸表，浏览器默认值就是这一组。 */
const FONT_SIZE_PX = ['10px', '13px', '16px', '18px', '24px', '32px', '48px']

/** 危险的样式取值：CSS 表达式与 javascript: URL */
function isDangerousStyleValue(value: string): boolean {
  const v = value.toLowerCase()
  return v.includes('expression(') || v.includes('javascript:')
}

/**
 * 过滤 style 属性，只留白名单里的声明。
 *
 * 导出是为了单测能单独打这一层：Outlook 一个 span 上能挂十几条 `mso-*`，
 * 这里错放一条，整封信就带着 Word 的私有样式发出去了。
 */
export function filterStyle(style: string): string {
  const kept: string[] = []
  for (const decl of style.split(';')) {
    const idx = decl.indexOf(':')
    if (idx <= 0) continue
    const prop = decl.slice(0, idx).trim().toLowerCase()
    const value = decl.slice(idx + 1).trim()
    if (!value) continue
    if (!ALLOWED_STYLE_PROPS.has(prop)) continue
    if (isDangerousStyleValue(value)) continue
    kept.push(`${prop}: ${value}`)
  }
  return kept.join('; ')
}

/** 把元素上的 `<font>` 属性翻译成等价的 style 声明 */
function fontAttrsToStyle(el: Element): string {
  const parts: string[] = []
  const color = el.getAttribute('color')
  if (color) parts.push(`color: ${color}`)
  const face = el.getAttribute('face')
  if (face) parts.push(`font-family: ${face}`)
  const size = el.getAttribute('size')
  if (size) {
    const n = Number.parseInt(size, 10)
    if (Number.isFinite(n) && n >= 1 && n <= 7) parts.push(`font-size: ${FONT_SIZE_PX[n - 1]}`)
  }
  return parts.join('; ')
}

/** 用子节点原地替换元素本身（保留内容，去掉这一层标签） */
function unwrap(el: Element): void {
  const parent = el.parentNode
  if (!parent) return
  while (el.firstChild) parent.insertBefore(el.firstChild, el)
  parent.removeChild(el)
}

/** 删掉文档里所有注释节点（Word 的条件注释块就藏在这里） */
function removeComments(root: Node, doc: Document): void {
  const walker = doc.createTreeWalker(root, 128 /* NodeFilter.SHOW_COMMENT */)
  const comments: Node[] = []
  let current = walker.nextNode()
  while (current) {
    comments.push(current)
    current = walker.nextNode()
  }
  for (const c of comments) c.parentNode?.removeChild(c)
}

/**
 * 清洗粘贴进来的 HTML。
 *
 * 处理顺序是有讲究的：先删注释和整棵丢弃的子树（否则 `<style>` 里的文本会被
 * 当成正文留下来），再翻译 font 标签（这一步会新增 style 属性），最后才统一过滤属性。
 */
export function cleanPastedHtml(html: string): string {
  if (!html) return ''
  if (typeof DOMParser === 'undefined') return html

  const doc = new DOMParser().parseFromString(html, 'text/html')
  const body = doc.body
  if (!body) return html

  removeComments(body, doc)

  // 1. 整棵删除 / 拆掉命名空间标签（<o:p>、<w:sdt>、<v:shapetype> …）
  //    命名空间标签本身没有语义，但里面可能有真正的文字，所以是 unwrap 不是 remove。
  for (const el of Array.from(body.querySelectorAll('*'))) {
    const tag = el.tagName.toLowerCase()
    if (DROP_TREE_TAGS.has(tag)) {
      el.parentNode?.removeChild(el)
      continue
    }
    if (tag.includes(':')) {
      unwrap(el)
    }
  }

  // 2. <font> → <span style>，并把 align 属性翻译成 text-align
  for (const el of Array.from(body.querySelectorAll('font'))) {
    const span = doc.createElement('span')
    const style = fontAttrsToStyle(el)
    if (style) span.setAttribute('style', style)
    while (el.firstChild) span.appendChild(el.firstChild)
    el.parentNode?.replaceChild(span, el)
  }
  for (const el of Array.from(body.querySelectorAll('[align]'))) {
    const align = el.getAttribute('align')
    if (align && ['left', 'right', 'center', 'justify'].includes(align.toLowerCase())) {
      const prev = el.getAttribute('style') ?? ''
      el.setAttribute('style', `${prev}${prev ? '; ' : ''}text-align: ${align.toLowerCase()}`)
    }
  }

  // 3. 属性白名单 + style 过滤
  for (const el of Array.from(body.querySelectorAll('*'))) {
    for (const attr of Array.from(el.attributes)) {
      const name = attr.name.toLowerCase()
      if (name === 'style') {
        const filtered = filterStyle(attr.value)
        if (filtered) el.setAttribute('style', filtered)
        else el.removeAttribute('style')
        continue
      }
      if (!ALLOWED_ATTRS.has(name)) el.removeAttribute(attr.name)
    }
    // href/src 里的 javascript: 一律去掉
    for (const urlAttr of ['href', 'src']) {
      const v = el.getAttribute(urlAttr)
      if (v && /^\s*javascript:/i.test(v)) el.removeAttribute(urlAttr)
    }
  }

  // 4. 拆掉不再携带任何信息的 span（Word 里这种壳子能套五六层）
  for (const el of Array.from(body.querySelectorAll('span'))) {
    if (el.attributes.length === 0) unwrap(el)
  }

  return body.innerHTML
}

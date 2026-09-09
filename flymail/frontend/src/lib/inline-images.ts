// 内联图片：同一张图在三个地方有三副面孔，这个文件负责三者之间的转换。
//
//   编辑器内   `blob:…`   —— 浏览器本地对象 URL，只有当前页面认；便宜、可撤销
//   草稿里     `data:…`   —— base64 内嵌，草稿自包含，不需要服务端暂存区
//   发出去的信 `cid:…`    —— MIME multipart/related 里的 Content-ID 引用
//
// 三态之间只能单向省事、不能想当然：blob URL 一旦 revoke 或页面刷新就失效，
// 所以**存草稿必须转 data:**；而 data: URI 塞进邮件正文会被大量客户端拦截，
// 所以**发送必须转 cid:**。中间任何一步偷懒，用户看到的都是"图片裂了"。

/** 后端对 Content-ID 的校验（见 docs/flymail/m13-composer.md）。CRLF 进头就是头注入。 */
export const CID_PATTERN = /^[A-Za-z0-9._-]{1,128}$/

/** 草稿正文总大小上限，超过则丢弃内联图并提示用户 */
export const DRAFT_MAX_BYTES = 5 * 1024 * 1024

/**
 * 生成一个新的 Content-ID。
 *
 * 前缀 `ii_` 是 Gmail 的写法，随后 16 位十六进制随机数。整体必然满足 CID_PATTERN——
 * cid 是要写进 `Content-ID:` 邮件头的，格式不是洁癖问题，是安全边界。
 */
export function newCid(): string {
  const bytes = new Uint8Array(8)
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes)
  } else {
    for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256)
  }
  let hex = ''
  for (const b of bytes) hex += b.toString(16).padStart(2, '0')
  return `ii_${hex}`
}

/** 是否为内联图的本地来源（还没变成 cid: 的那两种） */
export function isLocalImageSrc(src: string): boolean {
  return src.startsWith('blob:') || src.startsWith('data:')
}

function parseHtml(html: string): Document | null {
  if (typeof DOMParser === 'undefined') return null
  return new DOMParser().parseFromString(html, 'text/html')
}

/** 按文档顺序列出所有 `<img>` 的 src */
export function collectImageSrcs(html: string): string[] {
  const doc = parseHtml(html)
  if (!doc) return []
  return Array.from(doc.body.querySelectorAll('img')).map((img) => img.getAttribute('src') ?? '')
}

/**
 * 按文档顺序改写每个 `<img src>`。
 *
 * `map` 返回 null 表示这一张不动（例如远程图片 `https://…`，那是发件人自己写的外链，
 * 不该被我们打包成内联附件）。
 */
export function mapImageSrc(html: string, map: (src: string, index: number) => string | null): string {
  const doc = parseHtml(html)
  if (!doc) return html
  const imgs = Array.from(doc.body.querySelectorAll('img'))
  imgs.forEach((img, i) => {
    const src = img.getAttribute('src') ?? ''
    const next = map(src, i)
    if (next !== null) img.setAttribute('src', next)
  })
  return doc.body.innerHTML
}

/** 删掉所有内联图（blob:/data:），远程图保留。用于草稿超限时的截断。 */
export function stripLocalImages(html: string): { html: string; removed: number } {
  const doc = parseHtml(html)
  if (!doc) return { html, removed: 0 }
  let removed = 0
  for (const img of Array.from(doc.body.querySelectorAll('img'))) {
    if (isLocalImageSrc(img.getAttribute('src') ?? '')) {
      img.parentNode?.removeChild(img)
      removed++
    }
  }
  return { html: doc.body.innerHTML, removed }
}

/** UTF-8 字节数（草稿限额按字节算，中文正文按字符算会差三倍） */
export function htmlByteSize(html: string): number {
  if (typeof TextEncoder !== 'undefined') return new TextEncoder().encode(html).length
  return html.length
}

// ─────────────────────────────────────────────────────────────────────────────
// data: URI
// ─────────────────────────────────────────────────────────────────────────────

export interface ParsedDataUri {
  mime: string
  base64: string
}

/** 解析 `data:image/png;base64,xxxx`；非 base64 形式一律拒绝（我们只自己产出这一种） */
export function parseDataUri(uri: string): ParsedDataUri | null {
  const m = /^data:([^;,]+);base64,([\s\S]*)$/.exec(uri)
  if (!m) return null
  return { mime: m[1], base64: m[2] }
}

/** 从 mime 猜一个文件扩展名，纯粹为了收件方看到的附件名好看一点 */
function extForMime(mime: string): string {
  const sub = mime.split('/')[1] ?? 'bin'
  if (sub === 'jpeg') return 'jpg'
  if (sub === 'svg+xml') return 'svg'
  return sub.replace(/[^a-z0-9]/gi, '') || 'bin'
}

/**
 * data: URI → File。草稿恢复时把内嵌图变回可上传的文件。
 *
 * 用 `cid` 当文件名，收件方在附件列表里看到的就是这个名字；
 * 更重要的是发送时 `inline_cids` 与文件字段按序对应，名字对得上便于排查。
 */
export function dataUriToFile(uri: string, name: string): File | null {
  const parsed = parseDataUri(uri)
  if (!parsed) return null
  let binary: string
  try {
    binary = atob(parsed.base64)
  } catch {
    return null
  }
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return new File([bytes], `${name}.${extForMime(parsed.mime)}`, { type: parsed.mime })
}

/** File → data: URI（存草稿时用） */
export function fileToDataUri(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.readAsDataURL(file)
  })
}

// ─────────────────────────────────────────────────────────────────────────────
// 发送前的三态收敛
// ─────────────────────────────────────────────────────────────────────────────

/** 一张待发送的内联图：cid 与文件必须同时存在，且在两个数组里下标一致 */
export interface InlineAsset {
  cid: string
  file: File
}

export interface PreparedInline {
  /** src 已改写为 `cid:…` 的正文 */
  html: string
  /** 与 files 严格一一对应的 cid 列表（后端按下标取 Content-ID） */
  cids: string[]
  files: File[]
}

/**
 * 把正文里的 blob:/data: 图收敛成 cid: 引用，并给出与之对应的文件列表。
 *
 * `lookup` 负责把一个本地 src 变成 `{cid, file}`：blob URL 由编辑器登记的表直接查到，
 * data: URI（来自刚打开的草稿）现场解码成 File。返回 null 的图会被原样留下——
 * 留一张裂图，好过把不认识的东西当附件发出去。
 *
 * ⚠ cids 与 files 必须同序 push，这是与后端唯一的对齐约定。
 */
export function prepareInlineForSend(
  html: string,
  lookup: (src: string) => InlineAsset | null,
): PreparedInline {
  const cids: string[] = []
  const files: File[] = []
  // 同一张图可能被引用多次（复制粘贴），只上传一份，第二次复用同一个 cid。
  const seen = new Map<string, string>()

  const out = mapImageSrc(html, (src) => {
    if (!isLocalImageSrc(src)) return null
    const already = seen.get(src)
    if (already) return `cid:${already}`
    const asset = lookup(src)
    if (!asset || !CID_PATTERN.test(asset.cid)) return null
    seen.set(src, asset.cid)
    cids.push(asset.cid)
    files.push(asset.file)
    return `cid:${asset.cid}`
  })

  return { html: out, cids, files }
}

/**
 * 存草稿前把 blob: 图换成 data: URI，超过限额则整体丢弃内联图。
 *
 * `toDataUri` 由调用方提供（需要读 File，是异步 IO，不能放进纯函数里）。
 */
export async function prepareInlineForDraft(
  html: string,
  toDataUri: (src: string) => Promise<string | null>,
): Promise<{ html: string; truncated: boolean }> {
  const srcs = collectImageSrcs(html).filter((s) => s.startsWith('blob:'))
  const resolved = new Map<string, string>()
  for (const src of Array.from(new Set(srcs))) {
    const uri = await toDataUri(src)
    if (uri) resolved.set(src, uri)
  }

  let out = mapImageSrc(html, (src) => resolved.get(src) ?? null)
  if (htmlByteSize(out) <= DRAFT_MAX_BYTES) return { html: out, truncated: false }

  const stripped = stripLocalImages(out)
  out = stripped.html
  return { html: out, truncated: true }
}

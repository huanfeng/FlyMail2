import { auth } from '@/lib/auth'
import api from '@/lib/api'
import type { Attachment } from '@/lib/types'

/** 可预览的 MIME 类型前缀（图片 / PDF）。 */
const PREVIEWABLE = /^(image\/|application\/pdf)/

/** 判断附件是否可在浏览器中直接预览。 */
export function isPreviewable(a: Attachment): boolean {
  return PREVIEWABLE.test(a.content_type || '')
}

/**
 * 返回带 token 的附件访问 URL（用于 img/iframe/新标签预览）。
 * token 走 query 参数而非请求头，因为 img src / a href 无法附带自定义请求头。
 */
export function attachmentUrl(
  messageId: number,
  idx: number,
  opts?: { download?: boolean },
): string {
  const t = auth.access ?? ''
  const dl = opts?.download ? '&dl=1' : ''
  return `/api/v1/messages/${messageId}/attachments/${idx}?access_token=${encodeURIComponent(t)}${dl}`
}

/**
 * 通过 axios（Bearer 头）以 blob 方式下载附件并触发浏览器另存为。
 * 不将 token 暴露到 URL 中。
 */
export async function downloadAttachment(
  messageId: number,
  idx: number,
  filename: string,
): Promise<void> {
  const res = await api.get(`/messages/${messageId}/attachments/${idx}?dl=1`, {
    responseType: 'blob',
  })
  const url = URL.createObjectURL(res.data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'attachment'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/**
 * 将正文 HTML 中的 `cid:` 引用改写为附件接口 URL，
 * 使内联图片（如签名图、嵌入图）能在 iframe 中正常渲染。
 *
 * content_id 比对忽略大小写，同时兼容带尖括号 `<cid>` 的形式（仅匹配括号内的值）。
 */
export function rewriteCidLinks(
  html: string,
  messageId: number,
  attachments: Attachment[],
): string {
  return html.replace(/(["'(])cid:([^"')\s]+)/gi, (m, pre, cid) => {
    // 去掉可能的 < > 包裹
    const cidClean = String(cid).replace(/^<|>$/g, '').toLowerCase()
    const idx = attachments.findIndex((a) => {
      if (!a.content_id) return false
      return a.content_id.replace(/^<|>$/g, '').toLowerCase() === cidClean
    })
    if (idx < 0) return m
    return pre + attachmentUrl(messageId, idx)
  })
}

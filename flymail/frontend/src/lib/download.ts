/**
 * 把一个对象作为 JSON 文件交给浏览器下载。
 *
 * 与 attachments.ts 里的 downloadAttachment 同一套做法（createObjectURL +
 * 临时 <a download>），区别是数据来自内存而不是接口响应。
 *
 * ⚠ 延迟 revoke 不能省：`a.click()` 触发的下载是**异步**发起的，
 * 立即释放在部分浏览器（Firefox、大文件）会下到一个空文件。
 */
export function downloadJson(data: unknown, filename: string): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

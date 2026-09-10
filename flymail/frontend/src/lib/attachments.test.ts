import { describe, it, expect, vi } from 'vitest'
import type { Attachment } from '@/lib/types'

// ── mock @/lib/auth ──
// attachments.ts 现在**不该**再 import auth：这个 mock 是留给回归的哨兵——
// 谁要是把「没有 attachment_token 就退回 access token」的兜底加回来，
// URL 里就会冒出 TOK123，下面的断言当场炸掉。
vi.mock('@/lib/auth', () => ({
  auth: { access: 'TOK123' },
}))

// ── mock @/lib/api（attachments.ts import 了 api，但测试不需要真实请求） ──
vi.mock('@/lib/api', () => ({
  default: {},
}))

import { isPreviewable, attachmentUrl, rewriteCidLinks } from '@/lib/attachments'

// 构造最小 Attachment 对象
function makeAttachment(overrides: Partial<Attachment> = {}): Attachment {
  return {
    filename: 'file.bin',
    content_type: 'application/octet-stream',
    size: 0,
    is_inline: false,
    ...overrides,
  }
}

// ─────────────────────────────────────────────
// isPreviewable
// ─────────────────────────────────────────────
describe('isPreviewable', () => {
  it('image/png → true', () => {
    expect(isPreviewable(makeAttachment({ content_type: 'image/png' }))).toBe(true)
  })

  it('application/pdf → true', () => {
    expect(isPreviewable(makeAttachment({ content_type: 'application/pdf' }))).toBe(true)
  })

  it('text/plain → false', () => {
    expect(isPreviewable(makeAttachment({ content_type: 'text/plain' }))).toBe(false)
  })

  it('空字符串 → false', () => {
    expect(isPreviewable(makeAttachment({ content_type: '' }))).toBe(false)
  })
})

// ─────────────────────────────────────────────
// attachmentUrl
// ─────────────────────────────────────────────
describe('attachmentUrl', () => {
  it('凭据写在 ticket 参数上', () => {
    const url = attachmentUrl(5, 2, 'ATT-TOKEN')
    expect(url).toBe('/api/v1/messages/5/attachments/2?ticket=ATT-TOKEN')
  })

  it('download=true 时末尾含 &dl=1', () => {
    const url = attachmentUrl(5, 2, 'ATT-TOKEN', { download: true })
    expect(url).toBe('/api/v1/messages/5/attachments/2?ticket=ATT-TOKEN&dl=1')
  })

  it('token 缺失时不得退回 access token', () => {
    // 这条是安全约束，不是容错：写进邮件文档的 URL 只能带限定单封的 attachment_token。
    // 一旦允许退回 access token，邮件自带的 <style> 就能用属性前缀选择器把它逐字符外泄。
    // 宁可这一张图裂掉（后端 401），也不能把能开整个账号的凭据交给发件人控制的文档。
    for (const t of [undefined, '']) {
      const url = attachmentUrl(5, 2, t)
      expect(url).toBe('/api/v1/messages/5/attachments/2?ticket=')
      expect(url).not.toContain('TOK123')
      expect(url).not.toContain('access_token')
    }
  })

  it('token 中的特殊字符被转义', () => {
    expect(attachmentUrl(5, 2, 'a b&c')).toBe('/api/v1/messages/5/attachments/2?ticket=a%20b%26c')
  })
})

// ─────────────────────────────────────────────
// rewriteCidLinks
// ─────────────────────────────────────────────
describe('rewriteCidLinks', () => {
  const MSG_ID = 10

  it('将 cid:img1 改写为附件 URL', () => {
    const html = '<img src="cid:img1">'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).not.toContain('cid:')
    expect(result).toContain('/api/v1/messages/10/attachments/0?ticket=ATT-TOKEN')
  })

  it('改写结果不含 access token', () => {
    const html = '<img src="cid:img1">'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).toContain('ticket=ATT-TOKEN')
    expect(result).not.toContain('TOK123')
    expect(result).not.toContain('access_token')
  })

  it('同一封的多张内联图共用同一张 attachment_token（不是一次性票据）', () => {
    // 十几张 cid: 图会由浏览器并发请求；用完即废的票只有第一张能加载出来。
    const html = '<img src="cid:a"><img src="cid:b"><img src="cid:c">'
    const attachments = [
      makeAttachment({ content_id: 'a', content_type: 'image/png' }),
      makeAttachment({ content_id: 'b', content_type: 'image/png' }),
      makeAttachment({ content_id: 'c', content_type: 'image/png' }),
    ]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result.match(/ticket=ATT-TOKEN/g)).toHaveLength(3)
  })

  it('大小写：cid:IMG1 对 content_id img1 → 命中改写', () => {
    const html = '<img src="cid:IMG1">'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).not.toContain('cid:IMG1')
    expect(result).toContain('/api/v1/messages/10/attachments/0')
  })

  it('未命中 content_id → 原样保留 cid:xxx', () => {
    const html = '<img src="cid:unknown">'
    const attachments = [makeAttachment({ content_id: 'img1' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).toContain('cid:unknown')
  })

  it('无引号属性 src=cid:img1 → 也被改写', () => {
    const html = '<img src=cid:img1>'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).not.toContain('cid:img1')
    expect(result).toContain('/api/v1/messages/10/attachments/0')
  })

  it('多个 cid → 全部改写', () => {
    const html = '<img src="cid:img1"><img src="cid:img2">'
    const attachments = [
      makeAttachment({ content_id: 'img1', content_type: 'image/png' }),
      makeAttachment({ content_id: 'img2', content_type: 'image/jpeg' }),
    ]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).not.toContain('cid:img1')
    expect(result).not.toContain('cid:img2')
    expect(result).toContain('/attachments/0')
    expect(result).toContain('/attachments/1')
  })

  it('content_id 带尖括号 <img1> → 也能命中', () => {
    const html = '<img src="cid:img1">'
    // 服务端有时返回 <img1> 包裹形式
    const attachments = [makeAttachment({ content_id: '<img1>', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments, 'ATT-TOKEN')
    expect(result).not.toContain('cid:img1')
    expect(result).toContain('/attachments/0')
  })
})

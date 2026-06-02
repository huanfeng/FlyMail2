import { describe, it, expect, vi } from 'vitest'
import type { Attachment } from '@/lib/types'

// ── mock @/lib/auth（必须在 import 被测模块之前） ──
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
  it('返回正确的带 token URL', () => {
    const url = attachmentUrl(5, 2)
    expect(url).toBe('/api/v1/messages/5/attachments/2?access_token=TOK123')
  })

  it('download=true 时末尾含 &dl=1', () => {
    const url = attachmentUrl(5, 2, { download: true })
    expect(url).toContain('&dl=1')
    expect(url).toBe('/api/v1/messages/5/attachments/2?access_token=TOK123&dl=1')
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
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).not.toContain('cid:')
    expect(result).toContain('/api/v1/messages/10/attachments/0?access_token=TOK123')
  })

  it('大小写：cid:IMG1 对 content_id img1 → 命中改写', () => {
    const html = '<img src="cid:IMG1">'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).not.toContain('cid:IMG1')
    expect(result).toContain('/api/v1/messages/10/attachments/0')
  })

  it('未命中 content_id → 原样保留 cid:xxx', () => {
    const html = '<img src="cid:unknown">'
    const attachments = [makeAttachment({ content_id: 'img1' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).toContain('cid:unknown')
  })

  it('无引号属性 src=cid:img1 → 也被改写', () => {
    const html = '<img src=cid:img1>'
    const attachments = [makeAttachment({ content_id: 'img1', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).not.toContain('cid:img1')
    expect(result).toContain('/api/v1/messages/10/attachments/0')
  })

  it('多个 cid → 全部改写', () => {
    const html = '<img src="cid:img1"><img src="cid:img2">'
    const attachments = [
      makeAttachment({ content_id: 'img1', content_type: 'image/png' }),
      makeAttachment({ content_id: 'img2', content_type: 'image/jpeg' }),
    ]
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).not.toContain('cid:img1')
    expect(result).not.toContain('cid:img2')
    expect(result).toContain('/attachments/0')
    expect(result).toContain('/attachments/1')
  })

  it('content_id 带尖括号 <img1> → 也能命中', () => {
    const html = '<img src="cid:img1">'
    // 服务端有时返回 <img1> 包裹形式
    const attachments = [makeAttachment({ content_id: '<img1>', content_type: 'image/png' })]
    const result = rewriteCidLinks(html, MSG_ID, attachments)
    expect(result).not.toContain('cid:img1')
    expect(result).toContain('/attachments/0')
  })
})

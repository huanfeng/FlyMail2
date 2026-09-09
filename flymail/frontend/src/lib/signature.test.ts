import { describe, it, expect } from 'vitest'
import {
  SIGNATURE_ATTR,
  extractSignatureHtml,
  isBlankHtml,
  replaceSignatureHtml,
  signatureForScenario,
  wrapSignature,
} from '@/lib/signature'
import type { Signature } from '@/lib/types'

function sig(over: Partial<Signature> = {}): Signature {
  return { body_html: '<p>—— 小明</p>', use_on_new: true, use_on_reply: true, ...over }
}

describe('signatureForScenario', () => {
  it('新建场景看 use_on_new', () => {
    expect(signatureForScenario(sig({ use_on_new: true }), 'new')).toContain('小明')
    expect(signatureForScenario(sig({ use_on_new: false }), 'new')).toBe('')
  })

  it('回复场景看 use_on_reply', () => {
    expect(signatureForScenario(sig({ use_on_reply: false }), 'reply')).toBe('')
    expect(signatureForScenario(sig({ use_on_new: false, use_on_reply: true }), 'reply')).toContain('小明')
  })

  it('未配置签名返回空串', () => {
    expect(signatureForScenario(undefined, 'new')).toBe('')
    expect(signatureForScenario(null, 'reply')).toBe('')
  })

  it('空白签名按没配置处理（避免收件方多出一个空行）', () => {
    expect(signatureForScenario(sig({ body_html: '<p></p><p>  </p>' }), 'new')).toBe('')
    expect(signatureForScenario(sig({ body_html: '' }), 'new')).toBe('')
  })

  it('只有图片的签名算有内容', () => {
    expect(signatureForScenario(sig({ body_html: '<p><img src="data:image/png;base64,AA"></p>' }), 'new'))
      .toContain('<img')
  })
})

describe('isBlankHtml', () => {
  it('空标签、&nbsp;、空白都算空', () => {
    expect(isBlankHtml('')).toBe(true)
    expect(isBlankHtml('<p></p>')).toBe(true)
    expect(isBlankHtml('<p>&nbsp;</p>')).toBe(true)
    expect(isBlankHtml(undefined)).toBe(true)
  })

  it('有文字或图片就不算空', () => {
    expect(isBlankHtml('<p>x</p>')).toBe(false)
    expect(isBlankHtml('<img src="x">')).toBe(false)
  })
})

describe('replaceSignatureHtml', () => {
  const body = `<p>正文第一段</p>${wrapSignature('<p>旧签名</p>')}`

  it('整块换掉签名，正文一个字不动', () => {
    const out = replaceSignatureHtml(body, '<p>新签名</p>')
    expect(out).toContain('正文第一段')
    expect(out).toContain('新签名')
    expect(out).not.toContain('旧签名')
  })

  it('替换后仍然只有一个签名块', () => {
    const out = replaceSignatureHtml(body, '<p>新签名</p>')
    expect(out.match(new RegExp(SIGNATURE_ATTR, 'g'))).toHaveLength(1)
  })

  it('原本没有签名时追加到末尾', () => {
    const out = replaceSignatureHtml('<p>只有正文</p>', '<p>签名</p>')
    expect(out).toContain('只有正文')
    expect(out).toContain(SIGNATURE_ATTR)
    expect(out.indexOf('只有正文')).toBeLessThan(out.indexOf('签名'))
  })

  it('传空串表示去掉签名，正文保留', () => {
    const out = replaceSignatureHtml(body, '')
    expect(out).toContain('正文第一段')
    expect(out).not.toContain(SIGNATURE_ATTR)
  })

  it('正文里出现与签名相同的文字不会被误伤', () => {
    // 关键场景：用户在正文里引用了自己的签名文字，靠字符串匹配的实现会在这里改错地方
    const tricky = `<p>我的签名是「旧签名」</p>${wrapSignature('<p>旧签名</p>')}`
    const out = replaceSignatureHtml(tricky, '<p>新签名</p>')
    expect(out).toContain('我的签名是「旧签名」')
    expect(out).toContain('新签名')
  })
})

describe('extractSignatureHtml', () => {
  it('取出签名块内容', () => {
    expect(extractSignatureHtml(`<p>x</p>${wrapSignature('<p>S</p>')}`)).toBe('<p>S</p>')
  })

  it('没有签名块返回 null', () => {
    expect(extractSignatureHtml('<p>x</p>')).toBeNull()
  })
})

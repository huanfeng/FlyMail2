import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  MAIL_FRAME_SANDBOX,
  MAX_FRAME_HEIGHT,
  buildFrameDocument,
  cspMeta,
  isFrameEvent,
  parseFrameMessage,
  secureRandomHex,
} from '@/lib/mail-frame'

// 这一组测试盯的是 M12 的安全边界。它们不验证渲染效果，只验证
// 「沙箱属性没被人加回 allow-same-origin」「CSP 只放行带 nonce 的脚本」
// 「来自不可信文档的消息形状被逐项校验」——这三件事一旦回退，界面上看不出任何异样。

function doc(overrides: Partial<Parameters<typeof buildFrameDocument>[0]> = {}) {
  return buildFrameDocument({
    html: '<p>hello</p>',
    allowRemote: false,
    foldQuote: false,
    quoteHideCss: '[data-fm-quote]{display:none}',
    nonce: 'n0nc3' as string | null,
    token: 'tok3n' as string | null,
    ...overrides,
  })
}

describe('MAIL_FRAME_SANDBOX', () => {
  it('绝不包含 allow-same-origin', () => {
    // 与 allow-scripts 同时出现就等于没有沙箱：邮件脚本可读 parent.document
    expect(MAIL_FRAME_SANDBOX).not.toContain('allow-same-origin')
  })

  it('保留脚本与弹窗权限（注入脚本与外链兜底都依赖它们）', () => {
    expect(MAIL_FRAME_SANDBOX).toContain('allow-scripts')
    expect(MAIL_FRAME_SANDBOX).toContain('allow-popups')
    expect(MAIL_FRAME_SANDBOX).toContain('allow-popups-to-escape-sandbox')
  })
})

describe('secureRandomHex', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('产出指定字节数的十六进制串', () => {
    expect(secureRandomHex(16)).toMatch(/^[0-9a-f]{32}$/)
    expect(secureRandomHex(12)).toMatch(/^[0-9a-f]{24}$/)
  })

  it('两次调用不相同', () => {
    expect(secureRandomHex()).not.toBe(secureRandomHex())
  })

  it('没有 getRandomValues 时返回 null，绝不退回 Math.random', () => {
    // 可预测的 nonce 等于让邮件正文自己猜出来、写一个带 nonce 的 <script>
    vi.stubGlobal('crypto', {})
    expect(secureRandomHex(16)).toBeNull()
  })
})

describe('cspMeta', () => {
  it('默认不放行任何远程来源', () => {
    const meta = cspMeta(false, 'abc')
    expect(meta).toContain("default-src 'none'")
    expect(meta).not.toContain('https:')
    expect(meta).toContain("connect-src 'none'")
    expect(meta).toContain("object-src 'none'")
    expect(meta).toContain("form-action 'none'")
  })

  it('允许远程时只放开图片 / 媒体 / 字体，不放开 connect 与 frame', () => {
    const meta = cspMeta(true, 'abc')
    expect(meta).toMatch(/img-src[^;]*https:/)
    expect(meta).toMatch(/media-src[^;]*https:/)
    expect(meta).toContain("connect-src 'none'")
    expect(meta).toContain("frame-src 'none'")
  })

  it('style-src 恒定只放行内联样式，与 allowRemote 无关', () => {
    // 远程样式表要么是 <link> 要么是 @import，两者都已在服务端剥掉，
    // 放开 https: 换不来任何能渲染出来的东西，只是多一条向外发请求的路
    for (const allow of [true, false]) {
      expect(cspMeta(allow, 'abc')).toMatch(/style-src 'unsafe-inline'(;|$)/)
    }
  })

  it('media-src 与 img-src 同一份来源清单（cid 内联音视频才放得出来）', () => {
    const meta = cspMeta(false, 'abc')
    const img = /img-src ([^;]+)/.exec(meta)?.[1]
    const media = /media-src ([^;]+)/.exec(meta)?.[1]
    expect(media).toBe(img)
  })

  it('nonce 为 null 时收紧成 script-src none', () => {
    expect(cspMeta(false, null)).toContain("script-src 'none'")
  })

  it('script-src 只认给定 nonce，不含 unsafe-inline', () => {
    const meta = cspMeta(false, 'abc123')
    expect(meta).toContain("script-src 'nonce-abc123'")
    expect(meta).not.toContain("script-src 'unsafe-inline'")
  })

  it('img-src 带上父页面 origin —— 不透明源下 self 匹配不到内联附件地址', () => {
    // jsdom 的 location.origin 是 http://localhost:3000
    expect(cspMeta(false, 'abc')).toContain(window.location.origin)
  })
})

describe('buildFrameDocument', () => {
  it('注入脚本带着与 CSP 相同的 nonce，并写入 token', () => {
    const html = doc()
    expect(html).toContain('<script nonce="n0nc3">')
    expect(html).toContain("script-src 'nonce-n0nc3'")
    expect(html).toContain('"tok3n"')
  })

  it('引用折叠靠 srcDoc 里的样式完成，不依赖同源', () => {
    expect(doc({ foldQuote: true })).toContain('[data-fm-quote]{display:none}')
    expect(doc({ foldQuote: false })).not.toContain('[data-fm-quote]{display:none}')
  })

  it('外链兜底：base target=_blank 始终在', () => {
    expect(doc()).toContain('<base target="_blank">')
  })

  it('nonce / token 缺失时不注入脚本，CSP 收紧成 script-src none', () => {
    for (const bad of [{ nonce: null }, { token: null }]) {
      const html = doc(bad)
      expect(html).not.toContain('<script')
      expect(html).toContain("script-src 'none'")
    }
  })

  it('注入脚本同时监听 click 与 auxclick（中键不触发 click）', () => {
    const html = doc()
    expect(html).toContain("addEventListener('click', onLinkActivate, true)")
    expect(html).toContain("addEventListener('auxclick', onLinkActivate, true)")
  })

  it('邮件正文排在脚本之后，正文内容原样保留', () => {
    const html = doc({ html: '<p>marker-body</p>' })
    expect(html.indexOf('<script')).toBeLessThan(html.indexOf('marker-body'))
  })
})

describe('isFrameEvent', () => {
  const win = {} as Window

  it('只接受来自那个 iframe 窗口的事件', () => {
    expect(isFrameEvent({ source: win }, win)).toBe(true)
  })

  it('别的窗口、null 来源、iframe 未挂载一律拒绝', () => {
    expect(isFrameEvent({ source: {} as Window }, win)).toBe(false)
    expect(isFrameEvent({ source: null }, win)).toBe(false)
    expect(isFrameEvent({ source: win }, null)).toBe(false)
  })
})

describe('parseFrameMessage', () => {
  const TOKEN = 'tok3n'

  it('token 不匹配一律拒绝', () => {
    expect(parseFrameMessage({ type: 'fm:height', token: 'other', height: 100 }, TOKEN)).toBeNull()
    expect(parseFrameMessage({ type: 'fm:height', height: 100 }, TOKEN)).toBeNull()
  })

  it('非对象、空 token 一律拒绝', () => {
    expect(parseFrameMessage('fm:height', TOKEN)).toBeNull()
    expect(parseFrameMessage(null, TOKEN)).toBeNull()
    expect(parseFrameMessage({ type: 'fm:height', token: '', height: 10 }, '')).toBeNull()
  })

  it('token 为 null（本次未注入脚本）时不存在合法消息', () => {
    expect(parseFrameMessage({ type: 'fm:height', token: 'x', height: 10 }, null)).toBeNull()
  })

  it('高度：接受正常值，reason=load 表示可重设基准', () => {
    expect(parseFrameMessage({ type: 'fm:height', token: TOKEN, height: 320 }, TOKEN)).toEqual({
      kind: 'height',
      height: 320,
      rebase: false,
    })
    expect(
      parseFrameMessage({ type: 'fm:height', token: TOKEN, height: 320, reason: 'load' }, TOKEN),
    ).toEqual({ kind: 'height', height: 320, rebase: true })
  })

  it('高度：非数字 / 非正 / 越界一律拒绝', () => {
    const bad = [0, -5, NaN, Infinity, MAX_FRAME_HEIGHT + 1, '400']
    for (const height of bad) {
      expect(parseFrameMessage({ type: 'fm:height', token: TOKEN, height }, TOKEN)).toBeNull()
    }
  })

  it('mailto：只认 mailto: 协议', () => {
    expect(
      parseFrameMessage({ type: 'fm:mailto', token: TOKEN, href: 'mailto:a@b.com' }, TOKEN),
    ).toEqual({ kind: 'mailto', href: 'mailto:a@b.com' })
    expect(
      parseFrameMessage({ type: 'fm:mailto', token: TOKEN, href: 'https://x.com' }, TOKEN),
    ).toBeNull()
  })

  it('外链：只认 http / https，javascript: 与 file: 一律拒绝', () => {
    expect(parseFrameMessage({ type: 'fm:open', token: TOKEN, href: 'https://x.com' }, TOKEN)).toEqual(
      { kind: 'open', href: 'https://x.com' },
    )
    for (const href of ['javascript:alert(1)', 'file:///etc/passwd', 'data:text/html,x', '']) {
      expect(parseFrameMessage({ type: 'fm:open', token: TOKEN, href }, TOKEN)).toBeNull()
    }
  })

  it('未知消息类型忽略', () => {
    expect(parseFrameMessage({ type: 'fm:evil', token: TOKEN }, TOKEN)).toBeNull()
  })
})

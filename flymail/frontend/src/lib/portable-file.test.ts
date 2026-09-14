import { describe, it, expect } from 'vitest'
import { SUPPORTED_VERSION, validateBundle, parseBundle } from '@/lib/portable-file'
import type { PortableBundle } from '@/lib/types'

function bundle(over: Partial<PortableBundle> = {}): unknown {
  return {
    version: SUPPORTED_VERSION,
    encrypted: false,
    exported_at: '2026-09-14T12:00:00Z',
    accounts: [
      {
        name: 'alice',
        email: 'alice@example.com',
        auth_type: 'password',
        imap_host: 'imap.example.com',
        imap_port: 993,
        imap_security: 'ssl',
        smtp_host: 'smtp.example.com',
        smtp_port: 465,
        smtp_security: 'ssl',
        enabled: true,
      },
    ],
    ...over,
  }
}

/**
 * 导入文件的前端校验。
 *
 * 后端当然会再验一遍。这里的价值是**时机**：用户选完文件的那一刻就知道
 * 「这个文件不对」，而不是勾完账户、点了导入才收到一句 400。
 * 最常见的情形是随手点中了另一个 JSON——那时告诉他，比三步之后告诉他有用得多。
 *
 * 每条错误都要**各自**有原因键。共用一句「文件无效」等于什么都没说：
 * 用户分不清是选错了文件、文件版本太新、还是文件本身坏了，三者的处置完全不同。
 */
describe('validateBundle', () => {
  it('正常文件通过', () => {
    const r = validateBundle(bundle())
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.bundle.accounts).toHaveLength(1)
  })

  it('选错文件（不是导出物）→ 专门的原因', () => {
    // 这是最常见的一种错，必须能被单独说清楚
    expect(validateBundle({ foo: 'bar' })).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errNotBundle',
    })
    expect(validateBundle([1, 2, 3]).ok).toBe(false)
    expect(validateBundle(null).ok).toBe(false)
    expect(validateBundle('a string').ok).toBe(false)
  })

  it('accounts 不是数组也算"不是导出物"', () => {
    // 判 accounts 在判 version 之前：一个根本不是导出物的文件里，
    // 「缺少版本号」这句话没有任何诊断价值，只会把用户引向错误的方向。
    const r = validateBundle({ version: 1, accounts: 'oops' })
    expect(r).toEqual({ ok: false, reasonKey: 'settings.portable.errNotBundle' })
  })

  it('缺版本号 → 单独的原因', () => {
    const b = bundle() as Record<string, unknown>
    delete b.version
    expect(validateBundle(b)).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errNoVersion',
    })
  })

  it('版本比本程序新 → 拒绝，不猜', () => {
    // 字段语义可能已经变了。照读会得到一堆静默错配的账户——
    // 那比直接拒绝坏得多，因为用户根本不会发现。
    expect(validateBundle(bundle({ version: SUPPORTED_VERSION + 1 }))).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errNewerVersion',
    })
  })

  it('旧版本仍然可读', () => {
    // 向后兼容是这个格式存在的意义之一：用户去年导的文件今年还要能用。
    expect(validateBundle(bundle({ version: 0 })).ok).toBe(true)
  })

  it('加密文件 → 明确拒绝，而不是把密文当明文导进去', () => {
    // 那会得到一个"密码是一串乱码"的账户，用户完全看不出哪里错了。
    expect(validateBundle(bundle({ encrypted: true }))).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errEncrypted',
    })
  })

  it('空账户列表 → 单独提示', () => {
    expect(validateBundle(bundle({ accounts: [] }))).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errEmpty',
    })
  })

  it('账户缺邮箱 → 拒绝', () => {
    // 邮箱是导入侧的主键：冲突判定、挑选导入都按它。缺了就没法处理。
    const b = bundle() as { accounts: { email: string }[] }
    b.accounts[0].email = '   '
    expect(validateBundle(b)).toEqual({
      ok: false,
      reasonKey: 'settings.portable.errNoEmail',
    })
  })
})

describe('parseBundle', () => {
  function file(text: string, size?: number): File {
    // jsdom 的 File 支持 .text()；size 需要覆盖时直接改属性
    const f = new File([text], 'x.json', { type: 'application/json' })
    if (size != null) Object.defineProperty(f, 'size', { value: size })
    return f
  }

  it('读得出正常文件', async () => {
    const r = await parseBundle(file(JSON.stringify(bundle())))
    expect(r.ok).toBe(true)
  })

  it('不是 JSON → 说的是"不是 JSON"，不是"不像导出物"', async () => {
    // 两者的处置不同：前者多半是选中了一个二进制文件或文件损坏，
    // 后者是选中了另一个程序的 JSON。混成一句话，用户无从下手。
    const r = await parseBundle(file('这不是 JSON {{{'))
    expect(r).toEqual({ ok: false, reasonKey: 'settings.portable.errNotJson' })
  })

  it('文件过大直接拒，不去读它', async () => {
    // 账户配置就几 KB。几十 MB 的文件几乎肯定是选错了，
    // 而把它整个读进内存再 JSON.parse 会让页面卡住几秒。
    const r = await parseBundle(file('{}', 50 * 1024 * 1024))
    expect(r).toEqual({ ok: false, reasonKey: 'settings.portable.errTooLarge' })
  })

  it('校验结果与 validateBundle 一致', async () => {
    const r = await parseBundle(file(JSON.stringify(bundle({ encrypted: true }))))
    expect(r).toEqual({ ok: false, reasonKey: 'settings.portable.errEncrypted' })
  })
})

/**
 * 原因键必须真的存在于语言包里。
 *
 * 这些键是**字符串字面量**，拼错了 TypeScript 一个字都不会说——
 * 界面上直接显示 `settings.portable.errNotJson` 这串原文给用户看。
 * 而这条路径只有在"用户选错文件"时才走到，本地开发几乎不会遇到。
 */
describe('原因键在两种语言里都有对应文案', () => {
  it('zh / en 都能查到', async () => {
    const zh = (await import('@/locales/zh.json')).default as Record<string, unknown>
    const en = (await import('@/locales/en.json')).default as Record<string, unknown>

    const keys = [
      'errNotBundle', 'errNotJson', 'errNoVersion', 'errNewerVersion',
      'errEncrypted', 'errEmpty', 'errNoEmail', 'errTooLarge', 'errUnreadable',
    ]
    for (const locale of [zh, en]) {
      const settings = locale.settings as Record<string, unknown>
      const portable = settings.portable as Record<string, unknown>
      for (const k of keys) {
        expect(typeof portable[k], `缺少文案 settings.portable.${k}`).toBe('string')
      }
    }
  })
})

import type { PortableBundle } from '@/lib/types'

/**
 * 解析用户选的导出文件。
 *
 * ── 为什么在前端也校验一遍 ──────────────────────────────────────────────────
 *
 * 后端当然会再验。但这里拦下来的错误能**在用户按「导入」之前**说清楚，
 * 而不是让他选完文件、勾完账户、点了导入才收到一句 400。
 * 尤其是"选错文件"这种最常见的情形——用户随手点中了一个别的 JSON，
 * 此刻告诉他比三步之后告诉他有用得多。
 *
 * 返回判别式联合而不是抛异常：调用方要把原因**显示出来**，
 * 而 try/catch 里拿到的 Error 消息没法直接进 i18n。
 */
export type ParseResult =
  | { ok: true; bundle: PortableBundle }
  | { ok: false; reasonKey: string }

/** 本程序能读的最高格式版本。与后端 PortableVersion 保持一致。 */
export const SUPPORTED_VERSION = 1

/** 单个文件的大小上限。账户配置就几 KB，超过说明多半选错了文件。 */
const MAX_BYTES = 2 * 1024 * 1024

export function validateBundle(raw: unknown): ParseResult {
  if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) {
    return { ok: false, reasonKey: 'settings.portable.errNotBundle' }
  }
  const b = raw as Partial<PortableBundle>

  // accounts 是这个格式的标志性字段。先看它，因为"选错了文件"是最常见的错误，
  // 而版本号缺失在一个根本不是导出物的文件里没有任何诊断价值。
  if (!Array.isArray(b.accounts)) {
    return { ok: false, reasonKey: 'settings.portable.errNotBundle' }
  }
  if (typeof b.version !== 'number') {
    return { ok: false, reasonKey: 'settings.portable.errNoVersion' }
  }
  if (b.version > SUPPORTED_VERSION) {
    // 比本程序新的格式：字段语义可能已经变了，照读会得到一堆静默错配的账户
    return { ok: false, reasonKey: 'settings.portable.errNewerVersion' }
  }
  if (b.encrypted === true) {
    // 当前版本不会解密。必须明说，而不是把密文当明文导进去——
    // 那会得到一个"密码是一串乱码"的账户，用户完全看不出哪里错了。
    return { ok: false, reasonKey: 'settings.portable.errEncrypted' }
  }
  if (b.accounts.length === 0) {
    return { ok: false, reasonKey: 'settings.portable.errEmpty' }
  }
  // 邮箱地址是导入侧的主键（冲突判定、挑选导入都按它）。缺了就没法处理。
  if (b.accounts.some((a) => typeof a?.email !== 'string' || a.email.trim() === '')) {
    return { ok: false, reasonKey: 'settings.portable.errNoEmail' }
  }
  return { ok: true, bundle: b as PortableBundle }
}

/** 读取并解析一个 File。 */
export async function parseBundle(file: File): Promise<ParseResult> {
  if (file.size > MAX_BYTES) {
    return { ok: false, reasonKey: 'settings.portable.errTooLarge' }
  }
  let text: string
  try {
    text = await file.text()
  } catch {
    return { ok: false, reasonKey: 'settings.portable.errUnreadable' }
  }
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return { ok: false, reasonKey: 'settings.portable.errNotJson' }
  }
  return validateBundle(raw)
}

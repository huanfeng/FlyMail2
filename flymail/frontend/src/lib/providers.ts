export type Security = 'ssl' | 'starttls' | 'none'

export interface ServerPreset {
  host: string
  port: number
  security: Security
}

export interface ProviderPreset {
  id: string
  name: string
  domains: string[]
  imap: ServerPreset
  smtp: ServerPreset
  note?: string
}

// 常见邮箱服务商预设（IMAP+SMTP）。国内邮箱 SMTP 统一 ssl/465；保存前可"测试连接"校验。
export const PROVIDER_PRESETS: ProviderPreset[] = [
  { id: 'gmail', name: 'Gmail', domains: ['gmail.com', 'googlemail.com'],
    imap: { host: 'imap.gmail.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.gmail.com', port: 465, security: 'ssl' },
    note: '需应用专用密码或 OAuth' },
  { id: 'outlook', name: 'Outlook', domains: ['outlook.com', 'hotmail.com', 'live.com', 'office365.com'],
    imap: { host: 'outlook.office365.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.office365.com', port: 587, security: 'starttls' } },
  { id: 'yahoo', name: 'Yahoo', domains: ['yahoo.com', 'ymail.com'],
    imap: { host: 'imap.mail.yahoo.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.mail.yahoo.com', port: 465, security: 'ssl' },
    note: '需应用专用密码' },
  { id: '163', name: '网易 163', domains: ['163.com'],
    imap: { host: 'imap.163.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.163.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码），并在邮箱设置开启 IMAP/SMTP' },
  { id: '126', name: '网易 126', domains: ['126.com'],
    imap: { host: 'imap.126.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.126.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码）' },
  { id: 'yeah', name: 'Yeah', domains: ['yeah.net'],
    imap: { host: 'imap.yeah.net', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.yeah.net', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码）' },
  { id: 'qq', name: 'QQ 邮箱', domains: ['qq.com', 'foxmail.com'],
    imap: { host: 'imap.qq.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.qq.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码），并在设置开启 IMAP/SMTP' },
  { id: 'sina', name: '新浪邮箱', domains: ['sina.com', 'sina.com.cn'],
    imap: { host: 'imap.sina.com.cn', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.sina.com.cn', port: 465, security: 'ssl' } },
  { id: 'sohu', name: '搜狐邮箱', domains: ['sohu.com'],
    imap: { host: 'imap.sohu.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.sohu.com', port: 465, security: 'ssl' } },
]

/** 按邮箱域名匹配预设，未命中返回 null。 */
export function presetForEmail(email: string): ProviderPreset | null {
  const at = email.lastIndexOf('@')
  if (at < 0) return null
  const domain = email.slice(at + 1).trim().toLowerCase()
  if (!domain) return null
  return PROVIDER_PRESETS.find((p) => p.domains.includes(domain)) ?? null
}

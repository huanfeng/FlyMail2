import { AtSign, Globe } from 'lucide-vue-next'

// 常用邮箱服务提供商配置
export interface EmailProvider {
  name: string
  domains: string[]
  icon?: any
  imap: {
    server: string
    port: number
    ssl: boolean
  }
  smtp: {
    server: string
    port: number
    ssl: boolean
  }
  authType: 'password' | 'oauth' | 'app_password'
  description?: string
  setupNote?: string
  helpUrl?: string
}

export const EMAIL_PROVIDERS: EmailProvider[] = [
  {
    name: 'Gmail',
    domains: ['gmail.com', 'googlemail.com'],
    imap: {
      server: 'imap.gmail.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.gmail.com',
      port: 465,
      ssl: true
    },
    authType: 'app_password',
    description: 'Google Gmail 邮箱',
    setupNote: '需要开启两步验证并使用应用专用密码，不能使用账户密码',
    helpUrl: 'https://support.google.com/accounts/answer/185833'
  },
  {
    name: 'Outlook/Hotmail',
    domains: ['outlook.com', 'hotmail.com', 'live.com', 'msn.com'],
    imap: {
      server: 'outlook.office365.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp-mail.outlook.com',
      port: 587,
      ssl: true
    },
    authType: 'app_password',
    description: 'Microsoft Outlook 邮箱',
    setupNote: '需要开启两步验证并使用应用专用密码',
    helpUrl: 'https://support.microsoft.com/zh-cn/account-billing/using-app-passwords-with-apps-that-don-t-support-two-step-verification-5896ed9b-4263-e681-128a-a6f2979a7944'
  },
  {
    name: '网易邮箱',
    domains: ['163.com', '126.com', 'yeah.net'],
    imap: {
      server: 'imap.163.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.163.com',
      port: 465,
      ssl: true
    },
    authType: 'app_password',
    description: '网易163/126邮箱',
    setupNote: '需要在邮箱设置中开启IMAP/SMTP服务并使用授权码',
    helpUrl: 'https://help.163.com/09/1223/14/5R7P3QI100753VB8.html'
  },
  {
    name: 'QQ邮箱',
    domains: ['qq.com', 'foxmail.com'],
    imap: {
      server: 'imap.qq.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.qq.com',
      port: 587,
      ssl: true
    },
    authType: 'app_password',
    description: 'QQ邮箱',
    setupNote: '需要在邮箱设置中开启IMAP/SMTP服务并使用授权码',
    helpUrl: 'https://service.mail.qq.com/cgi-bin/help?subtype=1&no=1001256&id=28'
  },
  {
    name: '新浪邮箱',
    domains: ['sina.com', 'sina.cn'],
    imap: {
      server: 'imap.sina.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.sina.com',
      port: 587,
      ssl: true
    },
    authType: 'password',
    description: '新浪邮箱'
  },
  {
    name: '搜狐邮箱',
    domains: ['sohu.com'],
    imap: {
      server: 'imap.sohu.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.sohu.com',
      port: 587,
      ssl: true
    },
    authType: 'password',
    description: '搜狐邮箱'
  },
  {
    name: 'Yahoo邮箱',
    domains: ['yahoo.com', 'yahoo.cn'],
    imap: {
      server: 'imap.mail.yahoo.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.mail.yahoo.com',
      port: 587,
      ssl: true
    },
    authType: 'app_password',
    description: 'Yahoo 邮箱',
    setupNote: '需要开启两步验证并使用应用专用密码',
    helpUrl: 'https://help.yahoo.com/kb/generate-third-party-passwords-sln15241.html'
  },
  {
    name: 'iCloud',
    domains: ['icloud.com', 'me.com', 'mac.com'],
    imap: {
      server: 'imap.mail.me.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.mail.me.com',
      port: 587,
      ssl: true
    },
    authType: 'app_password',
    description: 'Apple iCloud 邮箱',
    setupNote: '需要在Apple ID设置中生成应用专用密码',
    helpUrl: 'https://support.apple.com/zh-cn/102654'
  },
  {
    name: '阿里云邮箱',
    domains: ['aliyun.com'],
    imap: {
      server: 'imap.aliyun.com',
      port: 993,
      ssl: true
    },
    smtp: {
      server: 'smtp.aliyun.com',
      port: 465,
      ssl: true
    },
    authType: 'password',
    description: '阿里云企业邮箱'
  }
]

/**
 * 根据邮箱地址自动检测邮箱服务提供商
 */
export function detectEmailProvider(email: string): EmailProvider | null {
  const domain = email.split('@')[1]?.toLowerCase()
  if (!domain) return null

  return EMAIL_PROVIDERS.find(provider =>
    provider.domains.some(d => d === domain)
  ) || null
}

/**
 * 获取邮箱提供商的设置建议
 */
export function getProviderSetupNote(provider: EmailProvider): string {
  if (provider.setupNote) {
    return provider.setupNote
  }

  switch (provider.authType) {
    case 'app_password':
      return '此邮箱需要使用应用专用密码，请在邮箱设置中生成'
    case 'oauth':
      return '此邮箱支持OAuth认证，更加安全'
    default:
      return '可以直接使用邮箱密码'
  }
}

/**
 * 根据邮箱地址获取对应的图标
 */
export function getEmailProviderIcon(email: string): any {
  if (!email) return AtSign

  const provider = detectEmailProvider(email)
  if (provider?.icon) {
    return provider.icon
  }

  // 如果没有找到对应的提供商，根据域名返回默认图标
  const domain = email.split('@')[1]
  if (!domain) return AtSign

  // 对于企业邮箱或自定义域名，使用 Globe 图标
  return Globe
}

/**
 * 根据邮箱地址获取提供商名称
 */
export function getEmailProviderName(email: string): string {
  const provider = detectEmailProvider(email)
  return provider?.name || 'Email'
}
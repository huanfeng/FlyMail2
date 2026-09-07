export interface Account {
  id: number
  name: string
  email: string
  username?: string
  auth_type: string
  imap_host: string
  imap_port: number
  imap_security: string
  smtp_host: string
  smtp_port: number
  smtp_security: string
  status: string
  last_sync_at?: string
  enabled: boolean
}

/** 正文预取模式：仅新邮件 / 最近 N 天 / 全部历史 */
export type BodySyncMode = 'new' | 'recent' | 'all'

export interface AppSettings {
  sync_depth: number
  sync_poll_interval: number
  /** 同步时把哪些邮件的正文也下载到本地 */
  body_sync_mode: BodySyncMode
  /** recent 模式的天数窗口 */
  body_sync_recent_days: number
}

/** 管理员资料 */
export interface Profile {
  username: string
  display_name: string
  email: string
  created_at: string
  last_login_at?: string
}

/** SSE 实时推送事件结构 */
export interface RealtimeEvent {
  type: 'new_mail'
  account_id: number
  folder_id: number
  new_count: number
}

export interface AccountStats {
  message_count: number
  folder_count: number
}

/** 收件人自动补全候选项（来自历史往来地址） */
export interface Contact {
  name: string
  email: string
}

/** 站内通知事件类型 */
export type NotifyEventType = 'mail_new' | 'sync_failed' | 'account_status'

/** 站内通知记录 */
export interface Notification {
  id: number
  type: NotifyEventType | string
  account_id: number
  /** 单封新邮件通知携带的消息 ID（用于精准跳转），其余情况缺省/0 */
  message_id?: number
  title: string
  body: string
  read: boolean
  created_at: string
}

/** 外发推送渠道 */
export interface NotifyChannel {
  id: number
  name: string
  kind: 'webhook' | 'feishu' | string
  url: string
  has_secret: boolean
  events: string[]
  enabled: boolean
  created_at: string
}

/** 渠道创建/更新入参 */
export interface NotifyChannelInput {
  name: string
  kind: string
  url: string
  secret?: string
  events: string[]
  enabled?: boolean
}

/** 系统监控概览 */
export interface MonitoringOverview {
  accounts: number
  folders: number
  messages: number
  unread: number
  active_workers: number
  poll_interval_sec: number
  uptime_sec: number
  version: string
  db_size_bytes: number
  pending_writeback: number
}

/** 单账户健康 */
export interface AccountHealth {
  id: number
  name: string
  email: string
  enabled: boolean
  status: string
  last_sync_at?: string
  message_count: number
  folder_count: number
  has_worker: boolean
  sync_phase: string
  sync_error?: string
  breaker_open: boolean
  queue_depth: number
  mode: string // 当前 runner 模式：idle/polling/disconnected/...
}

/** 一条运行时诊断事件 */
export interface DiagEvent {
  at: string
  type: string
  detail?: string
}

/** 单账户 runner 运行时诊断 */
export interface RunnerDiag {
  account_id: number
  mode: string
  mode_since: string
  mode_seconds: number
  idle_capable: boolean
  idle_allowed: boolean
  idle_active: boolean
  connected: boolean
  breaker_open: boolean
  breaker_failures: number
  queue_depth: number
  last_sync_at?: string
  last_error?: string
  last_error_at?: string
  events: DiagEvent[]
}

/** 诊断接口响应：running=false 表示账户停用或 runner 未拉起 */
export interface DiagnosticsResponse {
  running: boolean
  diagnostics?: RunnerDiag
}

/** 外发投递日志 */
export interface NotifyLog {
  id: number
  channel_id: number
  channel_name: string
  type: string
  status: 'ok' | 'failed' | string
  error?: string
  created_at: string
}

export interface ProxyInput {
  type: string
  host: string
  port: number
  username?: string
  password?: string
}

export interface AccountInput {
  name: string
  email: string
  username?: string
  password?: string
  imap_host: string
  imap_port: number
  imap_security: string
  smtp_host: string
  smtp_port: number
  smtp_security: string
  proxy?: ProxyInput
}

export interface ConnectionTestResult {
  imap: boolean
  smtp: boolean
  supports_idle: boolean
  capabilities?: string[]
  security_mode?: string
  imap_error?: string
  smtp_error?: string
}

export interface Folder {
  id: number
  account_id: number
  path: string
  display_name: string
  type: string
  selectable: boolean
  total_count: number
  unread_count: number
  sort_order: number
}

export interface Address {
  name: string
  email: string
}

export interface MessageListItem {
  id: number
  account_id: number
  folder_id: number
  uid: number
  subject: string
  from_name: string
  from_addr: string
  to: Address[]
  date: string
  size: number
  seen: boolean
  flagged: boolean
  has_attachment: boolean
  snippet: string
}

export interface Attachment {
  filename: string
  content_type: string
  size: number
  content_id?: string
  is_inline: boolean
}

export interface MessageDetail extends MessageListItem {
  cc?: Address[]
  text_body: string
  html_body: string
  attachments: Attachment[]
  body_synced: boolean
  message_id?: string
  in_reply_to?: string
  references?: string
}

export interface SendRequest {
  account_id: number
  to: string[]
  cc?: string[]
  bcc?: string[]
  subject: string
  body_html: string
  in_reply_to?: string
  references?: string
}

export interface Draft {
  id: number
  account_id: number
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  body_html: string
  in_reply_to: string
  references: string
}

export interface DraftRequest {
  account_id: number
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  body_html: string
  in_reply_to: string
  references: string
}

export type SyncPhase = 'none' | 'folders' | 'messages' | 'done' | 'error'

export interface SyncStatus {
  account_id?: number
  phase: SyncPhase
  total?: number
  processed?: number
  error?: string
}

/**
 * 服务端兜底搜索的结果（POST /search/remote）。
 * 后端把同一查询翻译成 IMAP SEARCH 发给所有启用账户，把服务器命中但本地没有的
 * 邮件补抓入库——所以这里没有邮件列表，前端重跑一次本地搜索即可看到新命中。
 */
export interface RemoteSearchResult {
  /** 补抓入库的邮件数 */
  fetched: number
  /** 服务器端命中数（含本地已有的） */
  matched: number
  /** 参与搜索的账户数 */
  accounts: number
  /** 参与搜索的文件夹数 */
  folders: number
  /** 失败账户的错误信息；部分账户失败不影响整体成功 */
  errors?: string[]
}

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

export interface AppSettings {
  sync_depth: number
  sync_poll_interval: number
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

// API Response Types based on OpenAPI specification

// FolderType constants - matches backend API
export const FolderType = {
  UNKNOWN: 0,
  INBOX: 1,
  SENT: 2,
  DRAFTS: 3,
  TRASH: 4,
  JUNK: 5,
  ARCHIVE: 6,
  CUSTOM: 7
} as const

export type FolderType = typeof FolderType[keyof typeof FolderType]

// Base response structure
export interface BaseResponse<T = any> {
  code: number
  message: string
  data: T | null
  error?: ErrorInfo | null
}

export interface ErrorInfo {
  details?: string
  field?: string
  reason?: string
  suggestion?: string
  error_code?: string
  metadata?: Record<string, any>
}

export interface PageData<T = any> {
  list: T[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

// User and Authentication Types
export interface User {
  user_id: number
  username: string
  email?: string
  is_admin: boolean
  created_at?: string
  updated_at?: string
  last_login?: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface RefreshRequest {
  refresh_token: string
}

export interface UpdateCredentialsRequest {
  username?: string
  email?: string
  password?: string
  old_password?: string
}

export interface LoginResponse {
  access_token: string
  refresh_token: string
  user: User
}

// Email Account Types
export interface EmailAccount {
  account_id: number
  user_id: number
  name: string
  email: string
  type: 'smtp' | 'imap' | 'oauth' | 'personal' | 'work'
  imap_server?: string
  imap_port?: number
  imap_ssl?: boolean
  smtp_server?: string
  smtp_port?: number
  smtp_ssl?: boolean
  username?: string
  is_active: boolean
  last_sync?: string
  supports_idle?: boolean | null
  capabilities?: string
  last_capability_check?: string | null
  signature?: string
  auto_reply?: boolean
  auto_reply_message?: string
  created_at?: string
  updated_at?: string
}

export interface CreateAccountRequest {
  name: string
  email: string
  type: 'smtp' | 'imap' | 'oauth'
  imap_server?: string
  imap_port?: number
  imap_ssl?: boolean
  smtp_server?: string
  smtp_port?: number
  smtp_ssl?: boolean
  username?: string
  password?: string
  signature?: string
  auto_reply?: boolean
  auto_reply_message?: string
}

export interface AccountTestResult {
  account_id: number
  tested_at: string
  imap: boolean
  smtp: boolean
  supports_idle: boolean
  capabilities: string[]
}

// Folder Types
export interface Folder {
  folder_id: number
  account_id: number
  name: string
  raw_name: string
  type: FolderType
  delimiter: string
  parent_name?: string
  flags?: string
  email_count: number
  unread_count: number
  last_sync_at?: string | null
  sort_order?: number
  created_at?: string
  updated_at?: string
}

// Email Types
export interface Email {
  email_id: number
  account_id: number
  message_id: string
  uid: number
  subject: string
  from: string
  to: string
  cc?: string
  bcc?: string
  is_read: boolean
  is_starred: boolean
  date: string
  size: number
  folder_name: string
  folder_id?: number | null
  folder_type: FolderType
  created_at?: string
}

export interface EmailDetail extends Email {
  body?: string
  body_html?: string
  attachments?: Attachment[]
}

export interface Attachment {
  attachment_id: number
  email_id: number
  filename: string
  content_type: string
  size: number
}

export interface SendEmailRequest {
  to: string[]
  cc?: string[]
  bcc?: string[]
  subject: string
  body: string
  content_type?: 'text/plain' | 'text/html'
}

export interface EmailFlags {
  is_read: boolean
  is_starred: boolean
}

export interface UpdateEmailFlagsRequest {
  is_read?: boolean
  is_starred?: boolean
}

// 批量操作类型
export interface BatchUpdateFlagsRequest {
  email_ids: number[]
  is_read?: boolean
  is_starred?: boolean
}

export interface BatchDeleteRequest {
  email_ids: number[]
}

export interface BatchOperationResult {
  successful: number
  failed: number
  failed_ids: number[]
  errors: string[]
}

// 虚拟文件夹类型
export type VirtualFolder = 'all-inbox' | 'all-starred' | 'all-unread' | 'all-sent' | 'all-drafts' | 'all-trash'

// Task Types
export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
export type TaskPriority = 'low' | 'normal' | 'high'
export type TaskType = 'loop' | 'scheduled' | 'once'

export interface Task {
  task_id: string
  type: string
  status: TaskStatus
  created_at: string
}

export interface TaskDetail {
  task_id: string
  type: string
  status: TaskStatus
  progress: number
  current_step?: string
  total_count?: number
  current_count?: number
  error_message?: string | null
  result?: any | null
  started_at?: string | null
  completed_at?: string | null
  created_at: string
}

export interface CreateTaskRequest {
  type: string
  params?: any
  priority?: TaskPriority
}

export interface SyncTaskRequest {
  limit?: number
  force?: boolean
  folders?: string[]
  priority?: TaskPriority
}

export interface TaskPageData {
  list: TaskDetail[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

// Task Configuration Types
export interface TaskConfig {
  id: number
  user_id: number
  task_type: TaskType
  task_name: string
  task_id: string
  name: string
  description?: string
  is_active: boolean
  priority: TaskPriority
  loop_interval?: number
  cron_expression?: string
  extra_config?: Record<string, any>
  created_at?: string
  updated_at?: string
  last_run?: string
  next_run?: string
  run_count?: number
  error_count?: number
  last_error?: string
}

export interface TaskConfigPageData {
  list: TaskConfig[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface CreateTaskConfigRequest {
  task_type: TaskType
  task_name: string
  name: string
  description?: string
  priority?: TaskPriority
  loop_interval?: number
  cron_expression?: string
  extra_config?: Record<string, any>
}

export interface UpdateTaskConfigRequest {
  name?: string
  description?: string
  is_active?: boolean
  priority?: TaskPriority
  loop_interval?: number
  cron_expression?: string
  extra_config?: Record<string, any>
}

export interface TaskLog {
  id: number
  task_config_id: number
  status: TaskStatus
  started_at: string
  completed_at?: string
  duration?: number
  error_message?: string
  details?: string
  created_at: string
}

// Settings Types
export interface AppSettings {
  email_sync_interval?: number
  email_sync_enabled?: boolean
  max_emails_per_sync?: number
  delete_after_days?: number
  enable_metrics?: boolean
  enable_debug_log?: boolean
  session_timeout?: number
  max_upload_size?: number
  theme?: 'light' | 'dark' | 'auto'
  language?: string
  timezone?: string
  date_format?: string
  enable_two_factor?: boolean
  password_min_length?: number
  password_require_upper?: boolean
  password_require_lower?: boolean
  password_require_number?: boolean
  password_require_special?: boolean
}

export interface EmailMonitorSettings {
  enabled: boolean
  enable_idle: boolean
  check_interval: number
  day_time_start: number
  day_time_end: number
  day_time_poll_interval: string
  night_time_poll_interval: string
  retry_interval: string
  max_retries: number
}

export interface MonitorStatus {
  is_active: boolean
  is_idle_supported: boolean
  mode: string
  last_check: string
  last_error?: string
  error_count: number
  emails_received: number
}

// Scheduled Task Types (CronTask in OpenAPI)
export interface CronTask {
  cron_id: number
  name: string
  type: string
  schedule: string
  is_active: boolean
  last_run?: string
  next_run?: string
  run_count?: number
  error_count?: number
  last_error?: string
  created_at?: string
  updated_at?: string
}

// Alias for backward compatibility
export type ScheduledTask = CronTask

export interface CreateScheduledTaskRequest {
  name: string
  type: 'email_sync' | 'database_backup'
  schedule: string
  is_active?: boolean
}

// Monitoring Types
export interface SystemMetrics {
  cpu_usage: number
  memory_usage: number
  memory_alloc_mb: number
  goroutine_count: number
  db_connections: number
  uptime_seconds: number
}

export interface BusinessMetrics {
  total_users: number
  total_accounts: number
  total_emails: number
  emails_sent_today: number
  emails_received_today: number
  active_sessions: number
  failed_operations: Record<string, number>
}

export interface RealtimeStatus {
  active_connections: number
  active_sse_clients: number
  queued_tasks: number
  running_tasks: number
  last_sync_by_account: Record<string, number>
  service_health: Record<string, string>
}

export interface MonitorStatus {
  timestamp: string
  system: SystemMetrics
  business: BusinessMetrics
  realtime: RealtimeStatus
}

export interface HealthStatus {
  status: string
  services: Record<string, string>
}

// SSE Event Types
export interface SSEEvent {
  type: string
  data: any
}

export interface NewEmailsEvent {
  account_id: number
  count: number
  emails: Email[]
}

export interface TaskProgressEvent {
  task_id: string
  progress: number
  message?: string
  current_count?: number
  total_count?: number
}

export interface TaskCountsEvent {
  pending: number
  running: number
  completed: number
  failed: number
}

// 通知通道类型
export type NotifyChannelType = 'webhook' | 'feishu' | 'wecom' | 'telegram' | 'email' | 'dingtalk' | 'slack'

// 通知严重级别
export type NotifySeverity = 'low' | 'medium' | 'high' | 'critical'

// 通知时间范围
export interface NotifyTimeRange {
  id?: number
  start_time: string  // HH:MM
  end_time: string    // HH:MM
  weekdays: number[]  // 0=周日, 1=周一, ..., 6=周六
}

// 通知通道事件
export interface NotifyChannelEvent {
  id?: number
  event_type: string
  min_severity: NotifySeverity
}

// 通知通道配置（根据不同类型有不同的配置）
export interface NotifyChannelConfig {
  // Webhook 通用配置
  webhook_url?: string
  
  // 飞书配置
  app_id?: string
  app_secret?: string
  
  // 企业微信配置
  corp_id?: string
  agent_id?: string
  secret?: string
  
  // Telegram 配置
  bot_token?: string
  chat_id?: string
  
  // 邮件配置
  smtp_host?: string
  smtp_port?: number
  smtp_user?: string
  smtp_password?: string
  from_email?: string
  to_emails?: string[]
  
  // 钉钉配置
  access_token?: string
  sign_secret?: string
  
  // Slack 配置
  // bot_token 已经在 Telegram 配置中定义，使用相同的字段
  channel?: string
  
  // 其他自定义配置
  [key: string]: any
}

// 通知通道
export interface NotifyChannel {
  id: string
  name: string
  type: NotifyChannelType
  enabled: boolean
  config: NotifyChannelConfig
  max_retries?: number
  retry_interval?: number
  time_ranges?: NotifyTimeRange[]
  events?: NotifyChannelEvent[]
  created_at: string
  updated_at: string
}

// 创建通知通道请求
export interface CreateNotifyChannelRequest {
  name: string
  type: NotifyChannelType
  enabled?: boolean
  config: NotifyChannelConfig
  max_retries?: number
  retry_interval?: number
  time_ranges?: NotifyTimeRange[]
  events?: NotifyChannelEvent[]
}

// 更新通知通道请求
export interface UpdateNotifyChannelRequest {
  name?: string
  enabled?: boolean
  config?: NotifyChannelConfig
  max_retries?: number
  retry_interval?: number
  time_ranges?: NotifyTimeRange[]
  events?: NotifyChannelEvent[]
}

// 通知日志状态
export type NotifyLogStatus = 'pending' | 'success' | 'failed'

// 通知日志
export interface NotifyLog {
  id: number
  channel_id: string
  channel?: NotifyChannel
  event_type: string
  severity: NotifySeverity
  status: NotifyLogStatus
  retry_count: number
  error_message?: string
  event_data?: Record<string, any>
  created_at: string
}

// 事件定义
export interface EventDefinition {
  event_type: string
  name: string
  description: string
  severity: NotifySeverity
  template: string
  variables: Array<{
    name: string
    type: string
    required: boolean
    description: string
  }>
}

// 测试通知请求
export interface TestNotificationRequest {
  event_type?: string
  severity?: NotifySeverity
  data?: Record<string, any>
}
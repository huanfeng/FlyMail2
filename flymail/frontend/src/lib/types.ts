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
export type NotifyEventType = 'mail_new' | 'sync_failed' | 'account_status' | 'mail_rule'

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

/**
 * 会话行（M10 线程视图的列表条目）。
 *
 * 口径见 docs/flymail/m10-threads.md：封数/未读/星标/附件/参与者按**账户内整条会话**统计
 * （跨文件夹去重），而主题/摘要/日期/latest_* 取的是**当前范围内**最新的那一封——
 * 于是「收件箱 3 封 + 已发送 2 封」显示 5 封，与展开手风琴看到的条数一致。
 */
export interface ThreadListItem {
  /** `{account_id}:{root message-id}`，按账户隔离；含 : @ <> 等字符，进 URL 必须编码 */
  thread_id: string
  account_id: number
  /** 整条会话的封数（去重） */
  count: number
  /** 整条会话的未读封数 */
  unread: number
  /** 任一成员被标星 */
  flagged: boolean
  /** 任一成员带附件 */
  has_attachment: boolean
  /** 范围内最新一封的主题 */
  subject: string
  /** 范围内最新一封的摘要 */
  snippet: string
  /** 范围内最新一封的日期（RFC3339） */
  date: string
  /** 范围内最新一封的邮件 id（手风琴默认展开这一封） */
  latest_id: number
  latest_folder_id: number
  /** 按首次出现顺序去重（按邮箱）的参与者，后端最多给 8 个 */
  participants: Address[]
}

/** 会话列表翻页游标（不透明，由后端回传，前端原样传回） */
export interface ThreadCursor {
  before_date: string
  before_thread: string
}

/** 三条会话列表链路（文件夹 / 聚合 / 搜索）共用的分页形状 */
export interface ThreadPage {
  threads: ThreadListItem[]
  next_cursor: ThreadCursor | null
  /** 会话总数，后端只在第一页给出（翻页时结果不变，重复分组纯属浪费） */
  total?: number
}

export interface Attachment {
  filename: string
  content_type: string
  size: number
  content_id?: string
  is_inline: boolean
}

export interface MessageDetail extends MessageListItem {
  /** 所属会话 id；会话视图下由通知跳转等入口据此定位到哪条会话 */
  thread_id?: string
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

// ── M11 规则引擎 + 黑名单 ─────────────────────────────────────────────────────

/** 条件字段。has_attachment 是布尔字段，只配 equals + 'true'/'false' */
export type RuleField =
  | 'from'
  | 'to'
  | 'cc'
  | 'subject'
  | 'body'
  | 'attachment_name'
  | 'has_attachment'

/** 条件运算符。regex 交给 Go RE2 执行，前端保存前用 new RegExp 先粗筛一遍语法 */
export type RuleOp =
  | 'contains'
  | 'not_contains'
  | 'equals'
  | 'regex'
  | 'starts_with'
  | 'ends_with'

/** 动作类型。move 之外的类型 value 恒为空串（后端按类型忽略） */
export type RuleActionType = 'move' | 'mark_read' | 'star' | 'delete' | 'notify'

export interface RuleCondition {
  field: RuleField
  op: RuleOp
  value: string
}

export interface RuleAction {
  type: RuleActionType
  /** move 时为目标文件夹的 display_name（跨账户按名字解析），其余类型为空串 */
  value: string
}

/** 规则创建/更新入参（id / priority / 时间戳由后端维护） */
export interface RuleInput {
  name: string
  enabled: boolean
  /** 0 = 对全部账户生效 */
  account_id: number
  /** all = 所有条件都要满足，any = 任一满足 */
  match: 'all' | 'any'
  conditions: RuleCondition[]
  actions: RuleAction[]
  /** 命中后不再看后续规则 */
  stop_processing: boolean
}

export interface Rule extends RuleInput {
  id: number
  /** 执行顺序，升序；前端通过 /rules/reorder 重排而非直接改这个值 */
  priority: number
  created_at: string
  updated_at: string
}

/** 试运行结果：只读求值，不产生任何副作用 */
export interface RuleTestResult {
  matched: MessageListItem[]
  /** 参与求值的邮件数 */
  scanned: number
  /** 其中正文尚未同步的封数——正文/附件名条件对这些邮件按「不命中」处理 */
  without_body: number
  /** 命中数超过后端固定的 50 条回传上限，matched 只是前 50 条 */
  truncated: boolean
}

/** 规则执行日志（诊断用）。rule_id = 0 表示黑名单命中 */
export interface RuleRun {
  id: number
  account_id: number
  message_key: string
  rule_id: number
  rule_name: string
  action: string
  created_at: string
}

/** 黑名单条目：完整地址或域名，后端已归一化为小写 */
export interface BlockEntry {
  id: number
  pattern: string
  note: string
  created_at: string
}

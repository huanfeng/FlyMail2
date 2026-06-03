import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Account, AccountHealth, AccountInput, AccountStats, AppSettings, ConnectionTestResult, Contact, Draft, DraftRequest, Folder, MessageDetail, MessageListItem, MonitoringOverview, Notification, NotifyChannel, NotifyChannelInput, NotifyLog, SendRequest, SyncStatus } from '@/lib/types'

export function useAccounts() {
  return useQuery({
    queryKey: ['accounts'],
    queryFn: async (): Promise<Account[]> => {
      // /accounts 返回裸数组（与 folders/messages 的包裹形状不同）；兼容两种形状。
      const { data } = await api.get<Account[] | { accounts: Account[] }>('/accounts')
      return Array.isArray(data) ? data : (data.accounts ?? [])
    },
  })
}

export function useFolders(accountId: number | null) {
  return useQuery({
    queryKey: ['folders', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<Folder[]> => {
      const { data } = await api.get<{ folders: Folder[] }>(`/accounts/${accountId}/folders`)
      return data.folders ?? []
    },
  })
}

export function useMessages(folderId: number | null) {
  return useQuery({
    queryKey: ['messages', folderId],
    enabled: folderId != null,
    queryFn: async (): Promise<MessageListItem[]> => {
      const { data } = await api.get<{ messages: MessageListItem[] }>(`/folders/${folderId}/messages?limit=50`)
      return data.messages ?? []
    },
  })
}

/**
 * 无限加载版邮件查询，复用 query key ['messages', folderId] 使现有 invalidate 生效。
 * before_uid=0 表示不限制，返回最新 50 封；翻页时传入上一页最后一封的 uid。
 */
export function useInfiniteMessages(folderId: number | null) {
  return useInfiniteQuery({
    queryKey: ['messages', folderId],
    enabled: folderId != null,
    initialPageParam: 0,
    queryFn: async ({ pageParam }): Promise<MessageListItem[]> => {
      const { data } = await api.get<{ messages: MessageListItem[] }>(
        `/folders/${folderId}/messages?limit=50&before_uid=${pageParam ?? 0}`,
      )
      return data.messages ?? []
    },
    getNextPageParam: (lastPage) => {
      const last = lastPage.at(-1)
      return lastPage.length < 50 || !last ? undefined : last.uid
    },
  })
}

/** 聚合视图：所有收件箱 / 所有未读 / 星标（跨所有账户） */
export type AggregateView = 'inbox' | 'unread' | 'starred'

/** 聚合列表翻页游标（不透明，由后端回传，前端原样传回） */
interface AggCursor {
  before_date: string
  before_id: number
}

interface AggregatePage {
  messages: MessageListItem[]
  next_cursor: AggCursor | null
}

/**
 * 跨账户聚合邮件列表（无限加载）。
 * 游标采用后端回传的 (date, id) keyset，规避跨文件夹 UID 不唯一与日期截断问题。
 * query key 以 'messages' 开头，使现有 invalidateQueries(['messages']) 一并刷新。
 */
export function useInfiniteAggregate(view: AggregateView | null) {
  return useInfiniteQuery({
    queryKey: ['messages', 'aggregate', view],
    enabled: view != null,
    initialPageParam: null as AggCursor | null,
    queryFn: async ({ pageParam }): Promise<AggregatePage> => {
      const params = new URLSearchParams({ view: view as string, limit: '50' })
      if (pageParam) {
        params.set('before_date', pageParam.before_date)
        params.set('before_id', String(pageParam.before_id))
      }
      const { data } = await api.get<AggregatePage>(`/aggregate/messages?${params.toString()}`)
      return { messages: data.messages ?? [], next_cursor: data.next_cursor ?? null }
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/** 聚合入口徽标计数：{ inbox, unread, starred } */
export function useAggregateCounts() {
  return useQuery({
    queryKey: ['aggregate-counts'],
    queryFn: async (): Promise<Record<AggregateView, number>> => {
      const { data } = await api.get<{ counts: Record<string, number> }>('/aggregate/counts')
      const c = data.counts ?? {}
      return { inbox: c.inbox ?? 0, unread: c.unread ?? 0, starred: c.starred ?? 0 }
    },
  })
}

/**
 * 跨账户全文搜索（无限加载）。q 为空时禁用。
 * 与聚合同款 (date,id) keyset 游标；query key 以 'messages' 开头便于统一失效。
 */
export function useInfiniteSearch(q: string) {
  const query = q.trim()
  return useInfiniteQuery({
    queryKey: ['messages', 'search', query],
    enabled: query.length > 0,
    initialPageParam: null as AggCursor | null,
    queryFn: async ({ pageParam }): Promise<AggregatePage> => {
      const params = new URLSearchParams({ q: query, limit: '50' })
      if (pageParam) {
        params.set('before_date', pageParam.before_date)
        params.set('before_id', String(pageParam.before_id))
      }
      const { data } = await api.get<AggregatePage>(`/search/messages?${params.toString()}`)
      return { messages: data.messages ?? [], next_cursor: data.next_cursor ?? null }
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  })
}

/** 删除邮件（移到回收站；已在回收站则永久删除，由后端判定）。 */
export function useDeleteMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/messages/${id}/delete`)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
    },
  })
}

/** 移动邮件到同账户的另一个文件夹。 */
export function useMoveMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, folderId }: { id: number; folderId: number }) => {
      await api.post(`/messages/${id}/move`, { folder_id: folderId })
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
    },
  })
}

/** 批量操作统一的缓存失效（邮件列表/文件夹/聚合计数/邮件详情）。 */
function invalidateMailCaches(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ['messages'] })
  void qc.invalidateQueries({ queryKey: ['folders'] })
  void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
}

/** 批量删除（移到回收站；已在回收站则永久删除）。 */
export function useBatchDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (ids: number[]) => { await api.post('/batch/delete', { ids }) },
    onSuccess: () => invalidateMailCaches(qc),
  })
}

/** 批量移动到同账户的目标文件夹。 */
export function useBatchMove() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, folderId }: { ids: number[]; folderId: number }) => {
      await api.post('/batch/move', { ids, folder_id: folderId })
    },
    onSuccess: () => invalidateMailCaches(qc),
  })
}

/** 批量标记已读/未读。 */
export function useBatchRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, read }: { ids: number[]; read: boolean }) => {
      await api.post('/batch/read', { ids, read })
    },
    onSuccess: () => invalidateMailCaches(qc),
  })
}

/** 批量加/取消星标。 */
export function useBatchFlag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ ids, flagged }: { ids: number[]; flagged: boolean }) => {
      await api.post('/batch/flag', { ids, flagged })
    },
    onSuccess: () => invalidateMailCaches(qc),
  })
}

/** 收件人自动补全：按输入片段检索历史往来联系人（按频率降序）。 */
export function useContacts(q: string, enabled: boolean) {
  return useQuery({
    queryKey: ['contacts', q],
    enabled,
    staleTime: 60_000,
    queryFn: async (): Promise<Contact[]> => {
      const { data } = await api.get<{ contacts: Contact[] }>(
        `/contacts?q=${encodeURIComponent(q)}&limit=8`,
      )
      return data.contacts ?? []
    },
  })
}

// ── 通知中心 ──────────────────────────────────────────────────────────────────

interface NotificationsPage {
  notifications: Notification[]
  unread_count: number
}

/** 站内通知 feed（无限加载，before_id 游标）。 */
export function useNotifications() {
  return useInfiniteQuery({
    queryKey: ['notifications'],
    initialPageParam: 0,
    queryFn: async ({ pageParam }): Promise<NotificationsPage> => {
      const { data } = await api.get<NotificationsPage>(`/notifications?limit=30&before_id=${pageParam ?? 0}`)
      return { notifications: data.notifications ?? [], unread_count: data.unread_count ?? 0 }
    },
    getNextPageParam: (lastPage) => {
      const arr = lastPage.notifications
      return arr.length < 30 ? undefined : arr[arr.length - 1].id
    },
  })
}

/** 轻量未读计数（铃铛角标用，定时刷新）。 */
export function useNotificationUnread() {
  return useQuery({
    queryKey: ['notifications-unread'],
    refetchInterval: 30_000,
    queryFn: async (): Promise<number> => {
      const { data } = await api.get<NotificationsPage>('/notifications?limit=1')
      return data.unread_count ?? 0
    },
  })
}

function invalidateNotifs(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ['notifications'] })
  void qc.invalidateQueries({ queryKey: ['notifications-unread'] })
}

export function useMarkNotificationRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.post('/notifications/read', { id }) },
    onSuccess: () => invalidateNotifs(qc),
  })
}

export function useMarkAllNotificationsRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => { await api.post('/notifications/read-all') },
    onSuccess: () => invalidateNotifs(qc),
  })
}

export function useClearNotifications() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => { await api.post('/notifications/clear') },
    onSuccess: () => invalidateNotifs(qc),
  })
}

// ── 外发渠道 ──────────────────────────────────────────────────────────────────

export function useNotifyChannels() {
  return useQuery({
    queryKey: ['notify-channels'],
    queryFn: async (): Promise<NotifyChannel[]> => {
      const { data } = await api.get<{ channels: NotifyChannel[] }>('/notify/channels')
      return data.channels ?? []
    },
  })
}

export function useCreateNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: NotifyChannelInput) => { await api.post('/notify/channels', input) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useUpdateNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: NotifyChannelInput }) => {
      await api.put(`/notify/channels/${id}`, input)
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useDeleteNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.delete(`/notify/channels/${id}`) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-channels'] }) },
  })
}

export function useTestNotifyChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => { await api.post(`/notify/channels/${id}/test`) },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['notify-logs'] }) },
  })
}

export function useNotifyLogs() {
  return useQuery({
    queryKey: ['notify-logs'],
    queryFn: async (): Promise<NotifyLog[]> => {
      const { data } = await api.get<{ logs: NotifyLog[] }>('/notify/logs?limit=50')
      return data.logs ?? []
    },
  })
}

// ── 系统监控 ──────────────────────────────────────────────────────────────────

/** 系统概览（打开监控面板时自动刷新）。enabled 控制仅在面板可见时轮询。 */
export function useMonitoringOverview(enabled: boolean) {
  return useQuery({
    queryKey: ['monitoring-overview'],
    enabled,
    refetchInterval: enabled ? 5000 : false,
    queryFn: async (): Promise<MonitoringOverview> => {
      const { data } = await api.get<MonitoringOverview>('/monitoring/overview')
      return data
    },
  })
}

/** 各账户健康。 */
export function useMonitoringAccounts(enabled: boolean) {
  return useQuery({
    queryKey: ['monitoring-accounts'],
    enabled,
    refetchInterval: enabled ? 5000 : false,
    queryFn: async (): Promise<AccountHealth[]> => {
      const { data } = await api.get<{ accounts: AccountHealth[] }>('/monitoring/accounts')
      return data.accounts ?? []
    },
  })
}

export function useSyncStatus(accountId: number | null, enabled: boolean) {
  return useQuery({
    queryKey: ['sync-status', accountId],
    enabled: accountId != null && enabled,
    refetchInterval: enabled ? 1000 : false,
    queryFn: async (): Promise<SyncStatus> => {
      const { data } = await api.get<SyncStatus>(`/accounts/${accountId}/sync/status`)
      return data
    },
  })
}

export function useTriggerSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      await api.post(`/accounts/${accountId}/sync`)
    },
    onSuccess: (_data, accountId) => {
      void qc.invalidateQueries({ queryKey: ['sync-status', accountId] })
    },
  })
}

export function useCreateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<Account> => {
      const { data } = await api.post<Account>('/accounts', input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useUpdateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: AccountInput }): Promise<Account> => {
      const { data } = await api.put<Account>(`/accounts/${id}`, input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useDeleteAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number): Promise<void> => {
      await api.delete(`/accounts/${id}`)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['accounts'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
    },
  })
}

export function useTestConnection() {
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<ConnectionTestResult> => {
      const { data } = await api.post<ConnectionTestResult>('/accounts/test', input)
      return data
    },
  })
}

export function useMessageDetail(messageId: number | null) {
  return useQuery({
    queryKey: ['message', messageId],
    enabled: messageId != null,
    queryFn: async (): Promise<MessageDetail> => {
      const { data } = await api.get<MessageDetail>(`/messages/${messageId}`)
      return data
    },
  })
}

export function useMarkRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, read }: { id: number; read: boolean }) => {
      await api.post(`/messages/${id}/read`, { read })
    },
    onSuccess: (_d, { id }) => {
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
      void qc.invalidateQueries({ queryKey: ['message', id] })
    },
  })
}

export function useToggleFlag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, flagged }: { id: number; flagged: boolean }) => {
      await api.post(`/messages/${id}/flag`, { flagged })
    },
    onSuccess: (_d, { id }) => {
      void qc.invalidateQueries({ queryKey: ['messages'] })
      void qc.invalidateQueries({ queryKey: ['aggregate-counts'] })
      void qc.invalidateQueries({ queryKey: ['message', id] })
    },
  })
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: async (): Promise<AppSettings> => {
      const { data } = await api.get<{ settings: Record<string, string> }>('/settings')
      return {
        sync_depth: Number(data.settings?.sync_depth ?? 1000) || 1000,
        sync_poll_interval: Number(data.settings?.sync_poll_interval ?? 180) || 180,
      }
    },
  })
}

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (settings: Record<string, string>) => {
      await api.put('/settings', { settings })
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['settings'] }) },
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: async ({ oldPassword, newPassword }: { oldPassword: string; newPassword: string }) => {
      await api.post('/auth/change-password', { old_password: oldPassword, new_password: newPassword })
    },
  })
}

export function useSetAccountEnabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) => {
      await api.post(`/accounts/${id}/enabled`, { enabled })
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useAccountStats(accountId: number | null) {
  return useQuery({
    queryKey: ['account-stats', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<AccountStats> => {
      const { data } = await api.get<AccountStats>(`/accounts/${accountId}/stats`)
      return data
    },
  })
}

export function useSend() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ req, files }: { req: SendRequest; files?: File[] }) => {
      if (files && files.length > 0) {
        // 有附件：用 multipart/form-data，payload 为 JSON 字段，文件挂在 attachments 下。
        const fd = new FormData()
        fd.append('payload', JSON.stringify(req))
        for (const f of files) fd.append('attachments', f, f.name)
        // 显式置空 Content-Type，让浏览器/axios 自动补全带 boundary 的 multipart 头。
        await api.post('/send', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      } else {
        await api.post('/send', req)
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
    },
  })
}

export function useDrafts(accountId: number | null) {
  return useQuery({
    queryKey: ['drafts', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<Draft[]> => {
      const { data } = await api.get<{ drafts: Draft[] } | Draft[]>(`/accounts/${accountId}/drafts`)
      // 后端可能返回 {drafts:[]} 或裸数组，兼容两种形式
      return Array.isArray(data) ? data : (data.drafts ?? [])
    },
  })
}

export function useCreateDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: DraftRequest): Promise<Draft> => {
      const { data } = await api.post<Draft>('/drafts', req)
      return data
    },
    onSuccess: (_d, req) => { void qc.invalidateQueries({ queryKey: ['drafts', req.account_id] }) },
  })
}

export function useUpdateDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, req }: { id: number; req: DraftRequest }): Promise<Draft> => {
      const { data } = await api.put<Draft>(`/drafts/${id}`, req)
      return data
    },
    onSuccess: (_d, { req }) => { void qc.invalidateQueries({ queryKey: ['drafts', req.account_id] }) },
  })
}

export function useDeleteDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; accountId: number }) => { await api.delete(`/drafts/${id}`) },
    onSuccess: (_d, { accountId }) => { void qc.invalidateQueries({ queryKey: ['drafts', accountId] }) },
  })
}

export function useSendDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; accountId: number }) => { await api.post(`/drafts/${id}/send`) },
    onSuccess: (_d, { accountId }) => {
      void qc.invalidateQueries({ queryKey: ['drafts', accountId] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
      void qc.invalidateQueries({ queryKey: ['messages'] })
    },
  })
}

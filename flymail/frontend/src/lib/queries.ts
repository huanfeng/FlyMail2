import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Account, AccountInput, AccountStats, AppSettings, ConnectionTestResult, Folder, MessageDetail, MessageListItem, SyncStatus } from '@/lib/types'

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
      void qc.invalidateQueries({ queryKey: ['message', id] })
    },
  })
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: async (): Promise<AppSettings> => {
      const { data } = await api.get<{ settings: Record<string, string> }>('/settings')
      return { sync_depth: Number(data.settings?.sync_depth ?? 1000) || 1000 }
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

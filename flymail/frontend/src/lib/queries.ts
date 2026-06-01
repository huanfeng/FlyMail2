import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Account, AccountInput, ConnectionTestResult, Folder, MessageListItem, SyncStatus } from '@/lib/types'

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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Account, Folder, MessageListItem, SyncStatus } from '@/lib/types'

export function useAccounts() {
  return useQuery({
    queryKey: ['accounts'],
    queryFn: async (): Promise<Account[]> => {
      const { data } = await api.get<{ accounts: Account[] }>('/accounts')
      return data.accounts ?? []
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

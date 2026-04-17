import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { useAccounts } from '@/composables/useAccounts'
import { useFolders } from '@/composables/useFolders'
import { useEmails } from '@/composables/useEmails'
import { sseService, foldersService, type SSEHandlers } from '@/api'
import { FolderType } from '@/utils/folderType'

import { useAuthStore } from './auth'

export const useMailApiStore = defineStore('mailApi', () => {
  // Auth store
  const authStore = useAuthStore()

  // Use composables
  const {
    accounts,
    activeAccounts,
    selectedAccountId,
    selectedAccount,
    isLoading: isLoadingAccounts,
    error: accountsError,
    fetchAccounts,
    selectAccount,
    syncAccount,
    testAccount,
    clearError: clearAccountsError
  } = useAccounts()

  const {
    folders,
    folderTree,
    selectedFolderId,
    selectedFolder,
    isLoading: isLoadingFolders,
    error: foldersError,
    fetchFolders,
    syncFolders,
    selectFolder: selectFolderBase,
    getFolderIcon,
    getLocalizedName,
    clearError: clearFoldersError
  } = useFolders(selectedAccountId)

  // Search state
  const searchQuery = ref('')

  // Virtual folder state
  const selectedVirtualFolder = ref<string | null>(null)

  // Email filters
  const emailFilters = computed(() => ({
    accountId: selectedAccountId.value || undefined,
    folderId: selectedFolderId.value || undefined,
    virtualFolder: selectedVirtualFolder.value || undefined,
    search: searchQuery.value
  }))

  const {
    emails,
    currentEmail,
    selectedEmailIds,
    selectedEmails,
    isLoading: isLoadingEmails,
    isLoadingDetail,
    error: emailsError,
    currentPage,
    totalEmails,
    totalPages,
    hasMore,
    fetchEmails,
    loadMore,
    fetchEmailDetail,
    toggleStar,
    markAsRead,
    deleteEmail,
    batchDelete,
    batchMarkAsRead,
    selectEmail: selectEmailBase,
    selectAll,
    clearSelection,
    clearError: clearEmailsError
  } = useEmails(emailFilters)

  // Additional state
  const isInitialized = ref(false)
  const isSyncing = ref(false)
  const syncProgress = ref(0)
  const syncMessage = ref('')


  // Combined loading and error states
  const isLoading = computed(() =>
    isLoadingAccounts.value || isLoadingFolders.value || isLoadingEmails.value
  )

  const error = computed(() =>
    accountsError.value || foldersError.value || emailsError.value
  )

  // Unread count
  const unreadCount = computed(() => {
    if (!selectedFolder.value) return 0
    return selectedFolder.value.unread_count || 0
  })

  // Initialize store
  async function initialize() {
    if (isInitialized.value) return

    // Prevent concurrent initialization
    if (isLoading.value) return

    // Mark initialization as started

    try {
      // 在初始化时跳过自动选择，让URL驱动状态管理器来处理
      await fetchAccounts(true)

      // Fetch folders for all accounts
      await fetchAllAccountsFolders()

      // Connect to SSE for real-time updates
      // Wrap in try-catch to handle SSE connection failures gracefully
      try {
        connectSSE()
      } catch (sseError) {
        console.warn('SSE connection failed, continuing without real-time updates:', sseError)
      }

      isInitialized.value = true
    } catch (err) {
      console.error('Failed to initialize mail store:', err)
    }
  }

  // Track ongoing folder fetch operations
  const folderFetchPromises = new Map<number, Promise<any>>()

  // Fetch folders for all accounts
  async function fetchAllAccountsFolders() {
    // Filter out invalid accounts before mapping
    const validAccounts = accounts.value.filter(account =>
      account && account.account_id && typeof account.account_id === 'number'
    )

    const accountFolderPromises = validAccounts.map(account =>
      fetchAccountFolders(account.account_id)
    )

    try {
      await Promise.all(accountFolderPromises)
    } catch (err) {
      console.error('Failed to fetch folders for some accounts:', err)
    }
  }

  // Fetch folders for a specific account
  async function fetchAccountFolders(accountId: number) {
    // Skip if accountId is invalid
    if (!accountId || typeof accountId !== 'number') {
      console.warn('Invalid accountId provided to fetchAccountFolders:', accountId)
      return []
    }

    // Check if there's already an ongoing fetch for this account
    const existingPromise = folderFetchPromises.get(accountId)
    if (existingPromise) {
      return existingPromise
    }

    // Create new fetch promise
    const fetchPromise = (async () => {
      try {
        const accountFolders = await foldersService.getFolders(accountId)

        // Update the folders array by removing old folders for this account
        // and adding the new ones
        const otherAccountFolders = folders.value.filter(f => f.account_id !== accountId)
        folders.value = [...otherAccountFolders, ...accountFolders]

        return accountFolders
      } catch (err) {
        console.error(`Failed to fetch folders for account ${accountId}:`, err)
        throw err
      } finally {
        // Remove promise from map when done
        folderFetchPromises.delete(accountId)
      }
    })()

    // Store promise in map
    folderFetchPromises.set(accountId, fetchPromise)

    return fetchPromise
  }


  // Select email and fetch detail
  async function selectEmail(emailId: number) {
    selectEmailBase(emailId)
    await fetchEmailDetail(emailId)
  }

  // Alias for compatibility
  const loadEmailDetail = fetchEmailDetail

  // Sync current account
  async function syncCurrentAccount() {
    if (!selectedAccountId.value) return

    isSyncing.value = true
    syncProgress.value = 0
    syncMessage.value = '开始同步邮件...'

    try {
      const task = await syncAccount(selectedAccountId.value)

      // Task will be monitored via SSE
      return task
    } catch (err) {
      isSyncing.value = false
      throw err
    }
  }

  // SSE handlers
  const sseHandlers: SSEHandlers = {
    onConnected: (data) => {
      console.log('SSE connected:', data)
    },

    onNewEmails: async (data) => {
      // Refresh emails if it's for current account
      if (data.account_id === selectedAccountId.value) {
        await fetchEmails(1)

        // Update folder unread counts incrementally
        await updateFolderCounts(data)
      }
    },

    onTaskProgress: (data: any) => {
      if (isSyncing.value) {
        syncProgress.value = data.progress
        syncMessage.value = data.message || `同步进度: ${data.progress}%`
      }
    },

    onTaskCompleted: async (task: any) => {
      if (isSyncing.value) {
        isSyncing.value = false
        syncProgress.value = 100
        syncMessage.value = '同步完成'

        // Only refresh if we're still on the same account
        if (task.account_id === selectedAccountId.value) {
          await fetchEmails(1)
          // Folder refresh is already handled by fetchAccountFolders
        }

        setTimeout(() => {
          syncProgress.value = 0
          syncMessage.value = ''
        }, 2000)
      }
    },

    onTaskFailed: (_task) => {
      if (isSyncing.value) {
        isSyncing.value = false
        syncProgress.value = 0
        syncMessage.value = '同步失败'
      }
    },

    onError: (error) => {
      console.error('SSE error:', error)
      // Don't stop the application, just log the error
    },

    onReconnect: () => {
      console.log('SSE reconnecting...')
    }
  }

  // Connect to SSE
  function connectSSE() {
    sseService.connect(sseHandlers)
  }

  // Disconnect SSE when store is disposed
  function dispose() {
    sseService.disconnect()
    isInitialized.value = false
  }

  // Watch auth state and cleanup when user logs out
  watch(() => authStore.isAuthenticated, (authenticated) => {
    if (!authenticated && isInitialized.value) {
      // User logged out, cleanup SSE connection
      dispose()
    }
  })

  // Search emails
  function setSearchQuery(query: string) {
    searchQuery.value = query
  }

  async function searchEmails(query: string) {
    searchQuery.value = query
    // Don't manually fetch emails here, the watch in useEmails will handle it
  }

  // Go to a specific page
  async function goToPage(page: number) {
    if (page < 1 || page > totalPages.value) return
    await fetchEmails(page)
  }

  // Update folder counts incrementally
  async function updateFolderCounts(data: any) {
    // If we have folder_id in the event data, update specific folder
    if (data.folder_id) {
      const folder = folders.value.find(f => f.folder_id === data.folder_id)
      if (folder && data.unread_count !== undefined) {
        folder.unread_count = data.unread_count
      }
    } else {
      // Otherwise refresh all folders for the account
      await fetchFolders()
    }
  }

  // Optimized folder selection with caching
  async function selectFolderOptimized(folderId: number) {
    selectFolderBase(folderId)

    // Clear virtual folder when selecting normal folder
    selectedVirtualFolder.value = null

    // Clear search when changing folders
    searchQuery.value = ''

    // Don't manually fetch emails here, the watch in useEmails will handle it
  }

  // Select virtual folder
  function selectVirtualFolder(virtualFolderId: string) {
    selectedVirtualFolder.value = virtualFolderId
    selectedAccountId.value = null // 虚拟文件夹清除账户选择
    selectedFolderId.value = null  // 清除普通文件夹选择

    // 清除搜索
    searchQuery.value = ''

    // 不在这里直接调用 fetchEmails，让 emailFilters 的 watch 自动处理
  }

  // Navigate to previous email
  async function navigateToPreviousEmail(): Promise<boolean> {
    if (!currentEmail.value || emails.value.length === 0) return false
    
    const currentIndex = emails.value.findIndex(e => e.email_id === currentEmail.value!.email_id)
    
    if (currentIndex > 0) {
      // Current page has previous email
      const previousEmail = emails.value[currentIndex - 1]
      await selectEmail(previousEmail.email_id)
      return true
    } else if (currentPage.value > 1) {
      // Need to load previous page
      await goToPage(currentPage.value - 1)
      // Select last email of previous page
      if (emails.value.length > 0) {
        const lastEmail = emails.value[emails.value.length - 1]
        await selectEmail(lastEmail.email_id)
        return true
      }
    }
    
    return false
  }

  // Navigate to next email
  async function navigateToNextEmail(): Promise<boolean> {
    if (!currentEmail.value || emails.value.length === 0) return false
    
    const currentIndex = emails.value.findIndex(e => e.email_id === currentEmail.value!.email_id)
    
    if (currentIndex >= 0 && currentIndex < emails.value.length - 1) {
      // Current page has next email
      const nextEmail = emails.value[currentIndex + 1]
      await selectEmail(nextEmail.email_id)
      return true
    } else if (hasMore.value) {
      // Need to load next page
      await loadMore()
      // After loading more, check if we have new emails
      const newIndex = emails.value.findIndex(e => e.email_id === currentEmail.value!.email_id)
      if (newIndex >= 0 && newIndex < emails.value.length - 1) {
        const nextEmail = emails.value[newIndex + 1]
        await selectEmail(nextEmail.email_id)
        return true
      }
    }
    
    return false
  }

  // Clear all errors
  function clearError() {
    clearAccountsError()
    clearFoldersError()
    clearEmailsError()
  }

  // Enhanced account selection with proper cleanup
  async function selectAccountEnhanced(accountId: number, skipAutoSelectFolder = false) {
    // Validate accountId
    if (!accountId || typeof accountId !== 'number') {
      console.warn('Invalid accountId provided to selectAccountEnhanced:', accountId)
      return
    }

    // Only proceed if different account
    if (selectedAccountId.value === accountId) {
      // If same account and not skipping folder selection, ensure we have selected a folder (but not in virtual folder state)
      if (!skipAutoSelectFolder && !selectedFolderId.value && !selectedVirtualFolder.value) {
        const inboxFolder = folders.value.find(f => f.type === FolderType.INBOX && f.account_id === accountId)
        if (inboxFolder) {
          await selectFolderOptimized(inboxFolder.folder_id)
        }
      }
      return
    }

    // Select the account first
    selectAccount(accountId)

    // Check if we have folders for this account, if not fetch them
    const accountFolders = folders.value.filter(f => f.account_id === accountId)
    if (accountFolders.length === 0) {
      try {
        await fetchAccountFolders(accountId)
      } catch (err) {
        console.error('Failed to fetch folders for account:', err)
      }
    }

    // Auto-select inbox folder if available and not skipping and not in virtual folder state
    if (!skipAutoSelectFolder && !selectedVirtualFolder.value) {
      const inboxFolder = folders.value.find(f => f.type === FolderType.INBOX && f.account_id === accountId)
      if (inboxFolder) {
        await selectFolderOptimized(inboxFolder.folder_id)
      }
    }
  }

  return {
    // Account state
    accounts,
    activeAccounts,
    selectedAccountId,
    selectedAccount,

    // Folder state
    folders,
    folderTree,
    selectedFolderId,
    selectedFolder,
    selectedVirtualFolder,

    // Email state
    emails,
    currentEmail,
    selectedEmailIds,
    selectedEmails,
    currentPage,
    totalEmails,
    totalPages,
    hasMore,
    searchQuery,

    // UI state
    isInitialized,
    isLoading,
    isLoadingEmails,
    isLoadingDetail,
    error,
    isSyncing,
    syncProgress,
    syncMessage,
    unreadCount,

    // Actions
    initialize,
    fetchAccounts,
    selectAccount: selectAccountEnhanced,
    fetchFolders,
    syncFolders,
    selectFolder: selectFolderOptimized,
    selectVirtualFolder,
    fetchEmails,
    loadMore,
    selectEmail,
    fetchEmailDetail,
    loadEmailDetail,
    toggleStar,
    markAsRead,
    deleteEmail,
    batchDelete,
    batchMarkAsRead,
    selectAll,
    clearSelection,
    syncCurrentAccount,
    testAccount,
    getFolderIcon,
    getLocalizedName,
    setSearchQuery,
    searchEmails,
    goToPage,
    fetchAccountFolders,
    fetchAllAccountsFolders,
    navigateToPreviousEmail,
    navigateToNextEmail,
    dispose,
    clearError
  }
})
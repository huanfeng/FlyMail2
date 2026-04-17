import { ref, computed, watch, type Ref } from 'vue'
import { emailsService, type Email, type EmailDetail } from '@/api'
import { getErrorMessage } from '@/utils/error'
import { useI18n } from '@/locales'

interface EmailFilters {
  accountId?: number
  folderId?: number
  virtualFolder?: string
  isRead?: boolean
  isStarred?: boolean
  search?: string
}

export function useEmails(filters: Ref<EmailFilters>) {
  const { t } = useI18n()
  const emails = ref<Email[]>([])
  const currentEmail = ref<EmailDetail | null>(null)
  const selectedEmailIds = ref<Set<number>>(new Set())
  const isLoading = ref(false)
  const isLoadingDetail = ref(false)
  const error = ref<string | null>(null)

  // Pagination
  const currentPage = ref(1)
  const pageSize = ref(50)
  const totalEmails = ref(0)
  const totalPages = ref(0)

  const hasMore = computed(() => currentPage.value < totalPages.value)
  const selectedEmails = computed(() =>
    emails.value.filter(email => selectedEmailIds.value.has(email.email_id))
  )

  // Track ongoing fetch requests to prevent duplicates
  const ongoingFetchKey = ref<string | null>(null)

  async function fetchEmails(page = 1) {
    // Generate a unique key for this request
    const fetchKey = `${page}-${filters.value.accountId}-${filters.value.folderId}-${filters.value.virtualFolder || ''}-${filters.value.search || ''}`

    // Skip if we're already fetching the same request
    if (ongoingFetchKey.value === fetchKey) {
      console.log('Skipping duplicate email fetch request:', fetchKey)
      return
    }

    ongoingFetchKey.value = fetchKey
    isLoading.value = true
    error.value = null

    try {
      const result = await emailsService.getEmails({
        page,
        page_size: pageSize.value,
        account_id: filters.value.accountId,
        folder_id: filters.value.folderId,
        virtual_folder: filters.value.virtualFolder as any,
        is_read: filters.value.isRead,
        is_starred: filters.value.isStarred,
        search: filters.value.search
      })

      if (page === 1) {
        emails.value = result.list
      } else {
        emails.value.push(...result.list)
      }

      currentPage.value = result.page
      totalEmails.value = result.total
      totalPages.value = result.total_pages
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.email.fetchListFailed')
      console.error('Failed to fetch emails:', err)
    } finally {
      isLoading.value = false
      ongoingFetchKey.value = null
    }
  }

  async function loadMore() {
    if (!hasMore.value || isLoading.value) return
    await fetchEmails(currentPage.value + 1)
  }

  async function fetchEmailDetail(emailId: number) {
    // Check if already showing this email
    if (currentEmail.value?.email_id === emailId) {
      return
    }

    isLoadingDetail.value = true
    error.value = null

    try {
      currentEmail.value = await emailsService.getEmail(emailId)

      // Mark as read automatically
      if (currentEmail.value && !currentEmail.value.is_read) {
        await emailsService.markAsRead(emailId)
        // Update in list
        const email = emails.value.find(e => e.email_id === emailId)
        if (email) {
          email.is_read = true
        }
      }
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.email.fetchDetailFailed')
      console.error('Failed to fetch email detail:', err)
      // 重新抛出错误以便调用方处理
      throw err
    } finally {
      isLoadingDetail.value = false
    }
  }

  async function toggleStar(emailId: number) {
    const email = emails.value.find(e => e.email_id === emailId)
    if (!email) return

    const newStarred = !email.is_starred

    try {
      await emailsService.toggleStar(emailId, newStarred)
      email.is_starred = newStarred

      if (currentEmail.value?.email_id === emailId) {
        currentEmail.value.is_starred = newStarred
      }
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.email.updateStarFailed')
      console.error('Failed to toggle star:', err)
    }
  }

  async function markAsRead(emailId: number, isRead = true) {
    const email = emails.value.find(e => e.email_id === emailId)
    if (!email) return

    try {
      if (isRead) {
        await emailsService.markAsRead(emailId)
      } else {
        await emailsService.markAsUnread(emailId)
      }

      email.is_read = isRead

      if (currentEmail.value?.email_id === emailId) {
        currentEmail.value.is_read = isRead
      }
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.email.updateReadStatusFailed')
      console.error('Failed to mark as read:', err)
    }
  }

  async function deleteEmail(emailId: number) {
    try {
      await emailsService.deleteEmail(emailId)

      // Remove from list
      const index = emails.value.findIndex(e => e.email_id === emailId)
      if (index > -1) {
        emails.value.splice(index, 1)
      }

      // Clear current if it was deleted
      if (currentEmail.value?.email_id === emailId) {
        currentEmail.value = null
      }

      // Remove from selection
      selectedEmailIds.value.delete(emailId)
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.email.deleteFailed')
      throw err
    }
  }

  async function batchDelete() {
    if (selectedEmailIds.value.size === 0) return

    const ids = Array.from(selectedEmailIds.value)
    const result = await emailsService.batchDelete(ids)

    if (result.successful > 0) {
      // Remove deleted emails from list
      emails.value = emails.value.filter(e => !selectedEmailIds.value.has(e.email_id))

      // Clear current if it was deleted
      if (currentEmail.value && selectedEmailIds.value.has(currentEmail.value.email_id)) {
        currentEmail.value = null
      }

      // Clear selection
      selectedEmailIds.value.clear()
    }

    return result
  }

  async function batchMarkAsRead(isRead = true) {
    if (selectedEmailIds.value.size === 0) return

    const ids = Array.from(selectedEmailIds.value)
    const result = await emailsService.batchUpdateFlags(ids, { is_read: isRead })

    if (result.successful > 0) {
      // Update emails in list
      emails.value.forEach(email => {
        if (selectedEmailIds.value.has(email.email_id)) {
          email.is_read = isRead
        }
      })

      // Update current email if selected
      if (currentEmail.value && selectedEmailIds.value.has(currentEmail.value.email_id)) {
        currentEmail.value.is_read = isRead
      }
    }

    return result
  }

  function selectEmail(emailId: number, multi = false) {
    if (multi) {
      if (selectedEmailIds.value.has(emailId)) {
        selectedEmailIds.value.delete(emailId)
      } else {
        selectedEmailIds.value.add(emailId)
      }
    } else {
      selectedEmailIds.value.clear()
      selectedEmailIds.value.add(emailId)
    }
  }

  function selectAll() {
    if (selectedEmailIds.value.size === emails.value.length) {
      selectedEmailIds.value.clear()
    } else {
      emails.value.forEach(email => {
        selectedEmailIds.value.add(email.email_id)
      })
    }
  }

  function clearSelection() {
    selectedEmailIds.value.clear()
  }

  function clearError() {
    error.value = null
  }

  // Track if we're currently fetching to prevent duplicate requests
  let fetchPromise: Promise<void> | null = null

  // Watch for filter changes
  watch(filters, async (newFilters, oldFilters) => {
    // Skip if filters haven't actually changed
    if (JSON.stringify(newFilters) === JSON.stringify(oldFilters)) {
      return
    }

    // Check if this is a cross-account switch that should override ongoing requests
    const accountChanged = oldFilters && newFilters.accountId !== oldFilters.accountId

    // Cancel any ongoing fetch if we need to override
    if (fetchPromise) {
      if (accountChanged) {
        console.log('🔄 [UseEmails]', t('msg.email.crossAccountSwitch'))
        // Cancel ongoing fetch by resetting the key
        ongoingFetchKey.value = null
        fetchPromise = null
      } else {
        return
      }
    }

    currentPage.value = 1
    fetchPromise = fetchEmails(1).finally(() => {
      fetchPromise = null
    })
  }, { deep: true })

  return {
    emails,
    currentEmail,
    selectedEmailIds,
    selectedEmails,
    isLoading,
    isLoadingDetail,
    error,
    currentPage,
    pageSize,
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
    selectEmail,
    selectAll,
    clearSelection,
    clearError
  }
}
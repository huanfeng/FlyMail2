import { ref, computed } from 'vue'
import { accountsService, type EmailAccount } from '@/api'
import { getErrorMessage } from '@/utils/error'
import { useI18n } from '@/locales'

export function useAccounts() {
  const accounts = ref<EmailAccount[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const selectedAccountId = ref<number | null>(null)
  const { t } = useI18n()

  const selectedAccount = computed(() =>
    accounts.value.find(acc => acc.account_id === selectedAccountId.value) || null
  )

  const activeAccounts = computed(() =>
    accounts.value.filter(acc => acc.is_active)
  )

  async function fetchAccounts(skipAutoSelect = false) {
    isLoading.value = true
    error.value = null

    try {
      const response = await accountsService.getAccounts()
      accounts.value = response.accounts

      // Auto-select first account if none selected (除非明确跳过)
      if (!skipAutoSelect && !selectedAccountId.value && accounts.value.length > 0) {
        selectedAccountId.value = accounts.value[0].account_id
      }
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.account.fetchFailed')
      console.error('Failed to fetch accounts:', err)
    } finally {
      isLoading.value = false
    }
  }

  function selectAccount(accountId: number) {
    selectedAccountId.value = accountId
  }

  async function syncAccount(accountId: number) {
    try {
      const task = await accountsService.syncAccount(accountId, {
        limit: 100,
        force: false
      })
      return task
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.account.syncFailed')
      throw err
    }
  }

  async function testAccount(accountId: number) {
    try {
      const result = await accountsService.testAccount(accountId)
      return result
    } catch (err) {
      error.value = getErrorMessage(err) || t('msg.account.testFailed')
      throw err
    }
  }

  function clearError() {
    error.value = null
  }

  return {
    accounts,
    activeAccounts,
    selectedAccountId,
    selectedAccount,
    isLoading,
    error,
    fetchAccounts,
    selectAccount,
    syncAccount,
    testAccount,
    clearError
  }
}
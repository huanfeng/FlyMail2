import { ref, computed, watch, type Ref } from 'vue'
import { foldersService, type Folder } from '@/api'
import { getErrorMessage } from '@/utils/error'
import { FolderType, getFolderTypeIcon } from '@/utils/folderType'
import { useI18n } from '@/locales'

export function useFolders(accountId: Ref<number | null>) {
  const { t } = useI18n()
  const folders = ref<Folder[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const selectedFolderId = ref<number | null>(null)

  const folderTree = computed(() => {
    const accountFolders = folders.value.filter(f => f.account_id === accountId.value)
    return foldersService.buildFolderTree(accountFolders)
  })

  const selectedFolder = computed(() =>
    folders.value.find(f => f.folder_id === selectedFolderId.value) || null
  )

  const inboxFolder = computed(() =>
    folders.value.find(f => f.type === FolderType.INBOX && f.account_id === accountId.value) || null
  )

  async function fetchFolders() {
    if (!accountId.value) {
      return
    }

    isLoading.value = true
    error.value = null

    try {
      const accountFolders = await foldersService.getFolders(accountId.value)

      // Remove old folders for this account and add new ones
      const otherAccountFolders = folders.value.filter(f => f.account_id !== accountId.value)
      folders.value = [...otherAccountFolders, ...accountFolders]

      // Auto-select inbox if no folder selected and account is valid (not virtual folder state)
      // 确保不在虚拟文件夹状态下自动选择文件夹
      if (!selectedFolderId.value && inboxFolder.value && accountId.value !== null && accountId.value > 0) {
        selectedFolderId.value = inboxFolder.value.folder_id
      }
    } catch (err) {
      error.value = getErrorMessage(err) || t('folders.fetchFailed')
      console.error('Failed to fetch folders:', err)
    } finally {
      isLoading.value = false
    }
  }

  async function syncFolders() {
    if (!accountId.value) return

    isLoading.value = true
    error.value = null

    try {
      const result = await foldersService.syncFolders(accountId.value)
      folders.value = result.folders
      return result
    } catch (err) {
      error.value = getErrorMessage(err) || t('folders.syncFailed')
      throw err
    } finally {
      isLoading.value = false
    }
  }

  function selectFolder(folderId: number) {
    // Only select folder if it belongs to the current account
    const folder = folders.value.find(f => f.folder_id === folderId)

    if (folder && folder.account_id === accountId.value) {
      selectedFolderId.value = folderId
    } else {
      console.warn('⚠️ [UseFolders] 文件夹选择失败:', {
        folderId,
        reason: !folder ? t('folders.folderNotFound') : t('folders.accountMismatch'),
        expectedAccountId: accountId.value,
        folderAccountId: folder?.account_id
      })
    }
  }

  function getFolderIcon(folder: Folder) {
    return getFolderTypeIcon(folder.type)
  }

  function getLocalizedName(folder: Folder) {
    return foldersService.getLocalizedFolderName(folder)
  }

  function clearError() {
    error.value = null
  }

  // Watch for account changes
  watch(accountId, () => {
    selectedFolderId.value = null
    fetchFolders()
  })

  return {
    folders,
    folderTree,
    selectedFolderId,
    selectedFolder,
    inboxFolder,
    isLoading,
    error,
    fetchFolders,
    syncFolders,
    selectFolder,
    getFolderIcon,
    getLocalizedName,
    clearError
  }
}
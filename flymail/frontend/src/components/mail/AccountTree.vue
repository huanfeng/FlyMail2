<script lang="ts" setup>
import { computed, ref, onMounted, watch, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronRight, Inbox, Star, FileText, Send, Trash2, Archive, AlertCircle, RefreshCw, PenSquare, MoreVertical, Copy, Download, FolderPlus, FolderSync, Settings, Edit } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import { useMailApiStore } from '@/stores/mailApi'
import { foldersService, type FolderTreeNode } from '@/api'
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'
import { FolderType } from '@/utils/folderType'
import { getEmailProviderIcon } from '@/utils/emailProviders'

interface AccountTreeProps {
  isCollapsed: boolean
}

defineProps<AccountTreeProps>()
const mailStore = useMailApiStore()
const stateManager = inject<UrlDrivenStateManager>('stateManager')!
const { t } = useI18n()

const expandedAccounts = ref<Set<number>>(new Set())

// Initialize on mount
onMounted(async () => {
  await mailStore.initialize()

  // Auto-expand all accounts
  mailStore.accounts.forEach(account => {
    expandedAccounts.value.add(account.account_id)
  })
})

// Virtual folders computed from actual data
const virtualFolders = computed(() => {
  const totalUnread = mailStore.folders.reduce((sum, folder) => sum + (folder.unread_count || 0), 0)
  const totalStarred = 0 // TODO: 从API获取星标邮件数量

  return [
    {
      id: 'all-inbox',
      name: t('folders.allInbox'),
      icon: Inbox,
      count: totalUnread,
      urlParam: 'inbox'
    },
    {
      id: 'all-unread',
      name: t('folders.allUnread'),
      icon: AlertCircle,
      count: totalUnread,
      urlParam: 'unread'
    },
    {
      id: 'all-starred',
      name: t('folders.allStarred'),
      icon: Star,
      count: totalStarred,
      urlParam: 'star'
    },
  ]
})

function toggleAccount(accountId: number) {
  if (expandedAccounts.value.has(accountId)) {
    expandedAccounts.value.delete(accountId)
  } else {
    expandedAccounts.value.add(accountId)
  }
}

function selectFolder(folderId: number, accountId?: number) {
  // 如果需要切换账户，使用原子性操作同时更新账户和文件夹
  if (accountId !== undefined && accountId !== mailStore.selectedAccountId) {
    console.log('🎯 [AccountTree] 跨账户选择文件夹:', { accountId, folderId })
    stateManager.selectAccountAndFolder(accountId, folderId)
  } else {
    // 如果账户相同，只选择文件夹（selectFolder会自动推断账户ID）
    console.log('📁 [AccountTree] 选择文件夹:', folderId)
    stateManager.selectFolder(folderId)
  }
}

function selectVirtualFolder(virtualFolderId: string) {
  console.log('✨ [AccountTree] 选择虚拟文件夹:', virtualFolderId)
  stateManager.selectVirtualFolder(virtualFolderId)
}

async function syncAccount(accountId: number) {
  try {
    // Select account if not already selected
    if (mailStore.selectedAccountId !== accountId) {
      await mailStore.selectAccount(accountId)
    }

    // Refresh folders for this account first
    await mailStore.fetchAccountFolders(accountId)

    // Then sync emails
    await mailStore.syncCurrentAccount()
  } catch (error) {
    console.error('Failed to sync account:', error)
  }
}

// Watch for account changes to update expanded state
watch(() => mailStore.selectedAccountId, (newId) => {
  if (newId && !expandedAccounts.value.has(newId)) {
    expandedAccounts.value.add(newId)
  }
})

function getFolderIcon(type: FolderType) {
  const icons: Record<FolderType, any> = {
    [FolderType.INBOX]: Inbox,
    [FolderType.SENT]: Send,
    [FolderType.DRAFTS]: FileText,
    [FolderType.TRASH]: Trash2,
    [FolderType.JUNK]: AlertCircle,
    [FolderType.ARCHIVE]: Archive,
    [FolderType.CUSTOM]: Archive,
    [FolderType.UNKNOWN]: Archive,
  }
  return icons[type] || Archive
}

// Get folders for specific account
function getAccountFolders(accountId: number): FolderTreeNode[] {
  // Filter folders to only include those for this account
  const accountFolders = mailStore.folders.filter(f => f.account_id === accountId)

  // Build tree from filtered folders
  return buildFolderTree(accountFolders)
}

// Simple tree builder
function buildFolderTree(folders: any[]): FolderTreeNode[] {
  // Use the folderService's buildFolderTree method instead
  return foldersService.buildFolderTree(folders)
}

interface FolderRenderItem {
  folder: FolderTreeNode
  accountId: number
  level: number
}

// Render folder tree recursively
function renderFolderTree(folders: FolderTreeNode[], accountId: number, level = 0): FolderRenderItem[] {
  const result: FolderRenderItem[] = []

  folders.forEach(folder => {
    result.push({
      folder,
      accountId,
      level
    })

    // Recursively add children
    if (folder.children && folder.children.length > 0) {
      result.push(...renderFolderTree(folder.children, accountId, level + 1))
    }
  })

  return result
}

// Format last sync time - commented out as not currently used
// function formatLastSync(account: any): string {
//   if (!account.last_sync) return t('account.neverSynced')
//
//   const lastSync = new Date(account.last_sync)
//   const now = new Date()
//   const diffMs = now.getTime() - lastSync.getTime()
//   const diffMins = Math.floor(diffMs / 60000)
//
//   if (diffMins < 1) return t('account.justNow')
//   if (diffMins < 60) return t('account.minutesAgo', { n: diffMins })
//
//   const diffHours = Math.floor(diffMins / 60)
//   if (diffHours < 24) return t('account.hoursAgo', { n: diffHours })
//
//   const diffDays = Math.floor(diffHours / 24)
//   return t('account.daysAgo', { n: diffDays })
// }

// Get folder count for an account (for debugging) - commented out as not currently used
// function getAccountFolderCount(accountId: number): number {
//   return mailStore.folders.filter(f => f.account_id === accountId).length
// }

// Account menu actions
async function copyEmailAddress(account: any) {
  try {
    await navigator.clipboard.writeText(account.email_address)
    // TODO: Show toast notification
  } catch (error) {
    console.error('Failed to copy email address:', error)
  }
}

async function fetchNewEmails(accountId: number) {
  await syncAccount(accountId)
}

async function fetchHistoricalEmails(accountId: number) {
  // TODO: Implement historical email fetch
  console.log('Fetching historical emails for account:', accountId)
}

async function syncFolderList(accountId: number) {
  try {
    await mailStore.fetchAccountFolders(accountId)
  } catch (error) {
    console.error('Failed to sync folder list:', error)
  }
}

async function createNewFolder(accountId: number) {
  // TODO: Implement new folder creation
  console.log('Creating new folder for account:', accountId)
}

// Folder menu actions
async function renameFolder(folderId: number) {
  // TODO: Implement folder rename
  console.log('Renaming folder:', folderId)
}

async function folderSettings(folderId: number) {
  // TODO: Implement folder settings
  console.log('Opening folder settings:', folderId)
}
</script>

<template>
  <ScrollArea class="h-full">
    <div class="py-2">
      <!-- Loading State -->
      <div v-if="mailStore.isLoading && !mailStore.isInitialized" class="px-2 space-y-2">
        <Skeleton class="h-8 w-full" />
        <Skeleton class="h-8 w-full" />
        <Skeleton class="h-8 w-full" />
      </div>


      <template v-else>
        <!-- Compose Button -->
        <div class="px-2 pb-2">
          <Button
            variant="outline"
            :class="cn('w-full justify-center', isCollapsed && 'px-0')"
            size="sm"
            @click="stateManager.openCompose('new')"
          >
            <PenSquare class="h-4 w-4" :class="!isCollapsed && 'mr-2'" />
            <span v-if="!isCollapsed">{{ t('account.composeEmail') }}</span>
          </Button>
        </div>

        <Separator class="mb-2" />

        <!-- Virtual Folders -->
        <div v-if="!isCollapsed">
          <nav class="grid gap-1 px-2">
            <a
              v-for="folder in virtualFolders"
              :key="folder.id"
              href="#"
              :class="cn(
                buttonVariants({
                  variant: mailStore.selectedVirtualFolder === folder.id ? 'default' : 'ghost',
                  size: 'sm'
                }),
                'justify-start'
              )"
              @click.prevent="selectVirtualFolder(folder.id)"
            >
              <component :is="folder.icon" class="mr-2 h-4 w-4" />
              <span class="flex-1 text-left">{{ folder.name }}</span>
              <span v-if="folder.count" class="ml-auto">
                {{ folder.count }}
              </span>
            </a>
          </nav>
        </div>

        <Separator v-if="!isCollapsed && mailStore.accounts.length > 0" class="my-2" />

        <!-- Account List -->
        <div v-if="!isCollapsed">
          <nav class="grid gap-1 px-2">
            <div v-for="account in mailStore.accounts" :key="account.account_id">
              <Collapsible :open="expandedAccounts.has(account.account_id)">
                <CollapsibleTrigger as-child>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="w-full justify-start"
                    @click="toggleAccount(account.account_id)"
                  >
                    <ChevronRight v-if="!expandedAccounts.has(account.account_id)" class="mr-1 h-4 w-4" />
                    <ChevronDown v-else class="mr-1 h-4 w-4" />
                    <component :is="getEmailProviderIcon(account.email)" class="mr-1 h-4 w-4" />
                    <span class="font-medium flex-1 text-left">{{ account.name }}</span>
                    <DropdownMenu>
                      <DropdownMenuTrigger as-child>
                        <Button
                          variant="ghost"
                          size="icon"
                          class="h-6 w-6 mr-1"
                          @click.stop
                        >
                          <MoreVertical class="h-3 w-3" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem @click="copyEmailAddress(account)">
                          <Copy class="mr-2 h-4 w-4" />
                          {{ t('account.copyEmail') }}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem @click="fetchNewEmails(account.account_id)" :disabled="mailStore.isSyncing">
                          <RefreshCw :class="cn('mr-2 h-4 w-4', mailStore.isSyncing && 'animate-spin')" />
                          {{ t('account.fetchNewEmails') }}
                        </DropdownMenuItem>
                        <DropdownMenuItem @click="fetchHistoricalEmails(account.account_id)">
                          <Download class="mr-2 h-4 w-4" />
                          {{ t('account.fetchHistoricalEmails') }}
                        </DropdownMenuItem>
                        <DropdownMenuItem @click="syncFolderList(account.account_id)">
                          <FolderSync class="mr-2 h-4 w-4" />
                          {{ t('account.syncFolderList') }}
                        </DropdownMenuItem>
                        <DropdownMenuItem @click="createNewFolder(account.account_id)">
                          <FolderPlus class="mr-2 h-4 w-4" />
                          {{ t('account.createNewFolder') }}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent class="mt-1 ml-4 grid gap-1">
                  <template v-for="{ folder, accountId, level } in renderFolderTree(getAccountFolders(account.account_id), account.account_id)" :key="`${account.account_id}-${folder.folder_id}`">
                    <div
                      :class="cn(
                        buttonVariants({
                          variant: (!mailStore.selectedVirtualFolder && mailStore.selectedFolderId === folder.folder_id) ? 'default' : 'ghost',
                          size: 'sm'
                        }),
                        'justify-start flex items-center group',
                        level > 0 && `ml-${level * 4}`
                      )"
                    >
                      <a
                        href="#"
                        class="flex-1 flex items-center"
                        @click.prevent="selectFolder(folder.folder_id, accountId)"
                      >
                        <component :is="getFolderIcon(folder.type)" class="mr-2 h-4 w-4" />
                        <span class="flex-1 text-left">{{ mailStore.getLocalizedName(folder) }}</span>
                        <span v-if="folder.unread_count" class="ml-auto mr-2">
                          {{ folder.unread_count }}
                        </span>
                      </a>
                      <DropdownMenu>
                        <DropdownMenuTrigger as-child>
                          <Button
                            variant="ghost"
                            size="icon"
                            class="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity"
                            @click.stop
                          >
                            <MoreVertical class="h-3 w-3" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem @click="renameFolder(folder.folder_id)">
                            <Edit class="mr-2 h-4 w-4" />
                            {{ t('folder.rename') }}
                          </DropdownMenuItem>
                          <DropdownMenuItem @click="folderSettings(folder.folder_id)">
                            <Settings class="mr-2 h-4 w-4" />
                            {{ t('folder.settings') }}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </template>
                </CollapsibleContent>
              </Collapsible>
            </div>
          </nav>

          <!-- Empty State -->
          <div v-if="mailStore.accounts.length === 0" class="px-2 py-8 text-center text-muted-foreground">
            <p class="text-sm">{{ t('account.noAccounts') }}</p>
            <p class="text-xs mt-1">{{ t('account.pleaseAdd') }}</p>
          </div>
        </div>

        <!-- Collapsed view -->
        <div v-else>
          <nav class="grid gap-1 px-2 justify-center">
            <template v-for="account in mailStore.accounts" :key="account.account_id">
              <Tooltip v-for="folder in mailStore.folders.filter(f => f.account_id === account.account_id)" :key="folder.folder_id" :delay-duration="0">
                <TooltipTrigger as-child>
                  <a
                    href="#"
                    :class="cn(
                      buttonVariants({
                        variant: (!mailStore.selectedVirtualFolder && mailStore.selectedFolderId === folder.folder_id) ? 'default' : 'ghost',
                        size: 'icon'
                      }),
                      'h-9 w-9'
                    )"
                    @click.prevent="selectFolder(folder.folder_id, account.account_id)"
                  >
                    <component :is="getFolderIcon(folder.type)" class="h-4 w-4" />
                    <span class="sr-only">{{ mailStore.getLocalizedName(folder) }}</span>
                  </a>
                </TooltipTrigger>
                <TooltipContent side="right" class="flex items-center gap-4">
                  {{ mailStore.getLocalizedName(folder) }}
                  <span v-if="folder.unread_count" class="ml-auto text-muted-foreground">
                    {{ folder.unread_count }}
                  </span>
                </TooltipContent>
              </Tooltip>
            </template>
          </nav>
        </div>
      </template>

      <!-- Sync Progress -->
      <div v-if="mailStore.isSyncing" class="px-2 mt-4">
        <div class="text-xs text-muted-foreground mb-1">{{ mailStore.syncMessage || t('sync.syncing') }}</div>
        <div class="w-full bg-secondary rounded-full h-1.5">
          <div
            class="bg-primary h-1.5 rounded-full transition-all duration-300"
            :style="{ width: `${mailStore.syncProgress}%` }"
          ></div>
        </div>
      </div>
    </div>
  </ScrollArea>
</template>
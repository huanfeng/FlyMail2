<script lang="ts" setup>
import { computed, ref, onMounted, inject } from 'vue'
import { User, Bell, Shield, Mail, Palette, MoreVertical, Trash2, Power, PowerOff, RefreshCw, Edit, Languages, Clock } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import DialogContentFast from '@/components/ui/dialog/DialogContentFast.vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { useSettingsStore } from '@/stores/settings'
import { useMailApiStore } from '@/stores/mailApi'
import AddAccountDialog from './AddAccountDialog.vue'
import EditAccountDialog from './EditAccountDialog.vue'
import ThemeCustomizer from '@/components/settings/ThemeCustomizer.vue'
import LanguageSelector from '@/components/settings/LanguageSelector.vue'
import AdminCredentialsForm from '@/components/settings/AdminCredentialsForm.vue'
import NotificationChannels from '@/components/settings/NotificationChannels.vue'
import TaskManager from '@/components/settings/TaskManager.vue'
import { accountsService } from '@/api'
import type { EmailAccount } from '@/api/types'
import { getErrorMessage } from '@/utils/error'
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'

const settingsStore = useSettingsStore()
const mailStore = useMailApiStore()
const activeSection = ref('account')
const { t } = useI18n()

// 注入状态管理器来处理设置对话框的开关
const stateManager = inject<UrlDrivenStateManager>('stateManager')!

// 创建计算属性来处理设置对话框的开关，确保通过URL状态管理器
const isSettingsOpen = computed({
  get: () => settingsStore.isSettingsOpen,
  set: (value: boolean) => {
    if (value) {
      stateManager.openSettings()
    } else {
      stateManager.closeSettings()
    }
  }
})

// 账户管理状态
const isLoadingAccounts = ref(false)
const accountsError = ref<string | null>(null)
const deleteDialogOpen = ref(false)
const accountToDelete = ref<EmailAccount | null>(null)
const isDeleting = ref(false)
const isToggling = ref<number | null>(null)
const toggleDialogOpen = ref(false)
const accountToToggle = ref<EmailAccount | null>(null)
const editDialogOpen = ref(false)
const accountToEdit = ref<EmailAccount | null>(null)



const sections = computed(() => [
  { id: 'account', label: t('settings.tabs.accounts'), icon: User },
  { id: 'language', label: t('settings.tabs.language'), icon: Languages },
  { id: 'theme', label: t('settings.tabs.theme'), icon: Palette },
  { id: 'security', label: t('settings.tabs.security'), icon: Shield },
  { id: 'notifications', label: t('settings.tabs.notifications'), icon: Bell },
  { id: 'tasks', label: t('settings.tabs.tasks'), icon: Clock },
])



// 账户管理功能
async function refreshAccounts() {
  isLoadingAccounts.value = true
  accountsError.value = null
  try {
    await mailStore.fetchAccounts()
  } catch (err) {
    accountsError.value = getErrorMessage(err) || t('settings.accounts.refreshFailed')
  } finally {
    isLoadingAccounts.value = false
  }
}

function handleAccountAdded() {
  // 刷新账户列表
  refreshAccounts()
}

function handleAccountUpdated() {
  // 刷新账户列表
  refreshAccounts()
  // 关闭编辑对话框
  editDialogOpen.value = false
  accountToEdit.value = null
}

function confirmToggleAccount(account: EmailAccount) {
  accountToToggle.value = account
  toggleDialogOpen.value = true
}

async function toggleAccountStatus() {
  if (!accountToToggle.value || isToggling.value === accountToToggle.value.account_id) return

  isToggling.value = accountToToggle.value.account_id
  try {
    const newStatus = !accountToToggle.value.is_active
    await accountsService.updateAccount(accountToToggle.value.account_id, {
      name: accountToToggle.value.name,
      email: accountToToggle.value.email,
      type: accountToToggle.value.type,
      is_active: newStatus
    } as any)
    await refreshAccounts()
    toggleDialogOpen.value = false
    accountToToggle.value = null
  } catch (err) {
    accountsError.value = getErrorMessage(err) || t('settings.accounts.toggleFailed')
  } finally {
    isToggling.value = null
  }
}

function confirmDeleteAccount(account: EmailAccount) {
  accountToDelete.value = account
  deleteDialogOpen.value = true
}

async function deleteAccount() {
  if (!accountToDelete.value || isDeleting.value) return

  isDeleting.value = true
  try {
    await accountsService.deleteAccount(accountToDelete.value.account_id)
    await refreshAccounts()
    deleteDialogOpen.value = false
    accountToDelete.value = null
  } catch (err) {
    accountsError.value = getErrorMessage(err) || t('settings.accounts.deleteFailed')
  } finally {
    isDeleting.value = false
  }
}

function getAccountStatusBadge(account: EmailAccount) {
  if (account.is_active) {
    return { text: t('common.enable'), variant: 'default' as const }
  } else {
    return { text: t('common.disable'), variant: 'secondary' as const }
  }
}

function formatLastSync(lastSync?: string) {
  if (!lastSync) return t('settings.accounts.neverSynced')

  const date = new Date(lastSync)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)

  if (diffMins < 1) return t('settings.accounts.justNow')
  if (diffMins < 60) return `${diffMins} ${t('settings.accounts.minutesAgo')}`

  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours} ${t('settings.accounts.hoursAgo')}`

  const diffDays = Math.floor(diffHours / 24)
  return `${diffDays} ${t('settings.accounts.daysAgo')}`
}

// 初始化时刷新账户列表
onMounted(() => {
  if (mailStore.accounts.length === 0) {
    refreshAccounts()
  }
})

</script>

<template>
  <Dialog v-model:open="isSettingsOpen">
    <DialogContentFast
      class="max-w-[95vw] sm:max-w-[95vw] md:max-w-[90vw] lg:max-w-[85vw] xl:max-w-[1200px] w-full h-[90vh] p-0 overflow-hidden">
      <div class="flex h-full">
        <!-- Left Sidebar -->
        <div
          class="min-w-[200px] max-w-[280px] w-1/4 lg:w-1/5 xl:w-[280px] border-r bg-muted/30 flex flex-col rounded-l-lg overflow-hidden">
          <DialogHeader class="p-4 pb-2 border-b bg-muted/30">
            <DialogTitle class="text-base">{{ t('settings.title') }}</DialogTitle>
            <DialogDescription class="sr-only">
              {{ t('settings.subtitle') }}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea class="flex-1">
            <div class="space-y-1 p-2">
              <Button v-for="section in sections" :key="section.id"
                :variant="activeSection === section.id ? 'default' : 'ghost'" class="w-full justify-start"
                @click="activeSection = section.id">
                <component :is="section.icon" class="mr-2 h-4 w-4" />
                {{ section.label }}
              </Button>
            </div>
          </ScrollArea>
        </div>

        <!-- Right Content -->
        <div class="flex-1 min-w-0 rounded-r-lg overflow-hidden">
          <ScrollArea class="h-full">
            <div class="p-8">
              <!-- Account Management -->
              <div v-show="activeSection === 'account'" class="space-y-6">
                <div class="flex items-center justify-between">
                  <div>
                    <h3 class="text-lg font-medium">{{ t('settings.tabs.accounts') }}</h3>
                    <p class="text-sm text-muted-foreground">{{ t('settings.accounts.title') }}</p>
                  </div>
                  <div class="flex gap-2">
                    <Button variant="outline" size="sm" @click="refreshAccounts" :disabled="isLoadingAccounts">
                      <RefreshCw :class="{ 'animate-spin': isLoadingAccounts }" class="h-4 w-4 mr-2" />
                      {{ t('common.refresh') }}
                    </Button>
                    <AddAccountDialog @account-added="handleAccountAdded" size="sm" />
                  </div>
                </div>
                <Separator />

                <!-- 错误提示 -->
                <Alert v-if="accountsError" variant="destructive">
                  <AlertDescription>{{ accountsError }}</AlertDescription>
                </Alert>

                <!-- 加载状态 -->
                <div v-if="isLoadingAccounts" class="space-y-3">
                  <Skeleton class="h-20 w-full" />
                  <Skeleton class="h-20 w-full" />
                  <Skeleton class="h-20 w-full" />
                </div>

                <!-- 账户列表 -->
                <div v-else-if="mailStore.accounts.length > 0" class="space-y-3">
                  <div v-for="account in mailStore.accounts" :key="account.account_id"
                    class="rounded-lg border p-4 hover:bg-accent/50 transition-colors">
                    <div class="flex items-start justify-between">
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2 mb-2">
                          <div class="font-medium truncate">{{ account.name }}</div>
                          <Badge :variant="getAccountStatusBadge(account).variant">
                            {{ getAccountStatusBadge(account).text }}
                          </Badge>
                        </div>
                        <div class="text-sm text-muted-foreground mb-1">{{ account.email }}</div>
                        <div class="text-xs text-muted-foreground">
                          {{ t('settings.accounts.type') }}: {{ account.type?.toUpperCase() || 'IMAP' }} |
                          {{ t('settings.accounts.lastSync') }}: {{ formatLastSync(account.last_sync) }}
                        </div>
                        <div v-if="account.imap_server" class="text-xs text-muted-foreground mt-1">
                          {{ t('settings.accounts.server') }}: {{ account.imap_server }}:{{ account.imap_port }}
                        </div>
                      </div>

                      <DropdownMenu>
                        <DropdownMenuTrigger as-child>
                          <Button variant="ghost" size="icon" class="h-8 w-8">
                            <MoreVertical class="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem @click="() => { accountToEdit = account; editDialogOpen = true }">
                            <Edit class="mr-2 h-4 w-4" />
                            {{ t('settings.accounts.edit') }}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem @click="confirmToggleAccount(account)"
                            :disabled="isToggling === account.account_id">
                            <Power v-if="!account.is_active" class="mr-2 h-4 w-4" />
                            <PowerOff v-else class="mr-2 h-4 w-4" />
                            {{ account.is_active ? t('common.disable') : t('common.enable') }}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem @click="confirmDeleteAccount(account)"
                            class="text-destructive focus:text-destructive">
                            <Trash2 class="mr-2 h-4 w-4" />
                            {{ t('settings.accounts.delete') }}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>
                </div>

                <!-- 空状态 -->
                <div v-else class="text-center py-12">
                  <Mail class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
                  <h3 class="text-lg font-medium mb-2"> {{ t('settings.accounts.noAccounts') }}</h3>
                  <p class="text-sm text-muted-foreground mb-4">
                    {{ t('settings.accounts.noAccountsDesc') }}
                  </p>
                  <AddAccountDialog @account-added="handleAccountAdded" size="sm" />
                </div>
              </div>

              <!-- Language Settings -->
              <div v-show="activeSection === 'language'" class="space-y-6">
                <div>
                  <h3 class="text-lg font-medium">{{ t('settings.tabs.language') }}</h3>
                  <p class="text-sm text-muted-foreground">{{ t('settings.language.title') }}</p>
                </div>
                <Separator />
                <div class="space-y-4">
                  <LanguageSelector />
                </div>
              </div>

              <!-- Theme Settings -->
              <div v-show="activeSection === 'theme'" class="space-y-6">
                <div>
                  <h3 class="text-lg font-medium">{{ t('settings.tabs.theme') }}</h3>
                  <p class="text-sm text-muted-foreground">{{ t('settings.theme.title') }}</p>
                </div>
                <Separator />
                <ThemeCustomizer />
              </div>


              <!-- Security Settings -->
              <div v-show="activeSection === 'security'" class="space-y-6">
                <div>
                  <h3 class="text-lg font-medium">{{ t('settings.tabs.security') }}</h3>
                  <p class="text-sm text-muted-foreground">{{ t('settings.security.title') }}</p>
                </div>
                <Separator />

                <!-- 管理员信息修改 -->
                <AdminCredentialsForm />

                <Separator />

                <!-- 其他安全设置 -->
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <div class="space-y-0.5">
                      <Label>{{ t('settings.security.twoFactorAuth') }}</Label>
                      <p class="text-sm text-muted-foreground">{{ t('settings.security.twoFactorAuthDesc') }}</p>
                    </div>
                    <Button variant="outline" size="sm">{{ t('common.enable') }}</Button>
                  </div>
                </div>
              </div>

              <!-- Notification Settings -->
              <div v-show="activeSection === 'notifications'" class="space-y-6">
                <NotificationChannels />
              </div>
              <!-- Task Management -->
              <div v-show="activeSection === 'tasks'" class="space-y-6">
                <TaskManager />
              </div>
            </div>
          </ScrollArea>
        </div>
      </div>
    </DialogContentFast>

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.accounts.confirmDelete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.accounts.confirmDeleteDesc', { email: accountToDelete?.name }) }}
            <br />
            <strong>{{ t('settings.accounts.confirmDeleteWarning') }}</strong>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="deleteAccount" :disabled="isDeleting"
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90">
            {{ isDeleting ? t('common.deleting') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- 禁用/启用确认对话框 -->
    <AlertDialog v-model:open="toggleDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ accountToToggle?.is_active ? t('settings.accounts.confirmDisable') :
            t('settings.accounts.confirmEnable') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ accountToToggle?.is_active ? t('settings.accounts.confirmDisableDesc') :
              t('settings.accounts.confirmEnableDesc') }}
            <br />
            <span v-if="accountToToggle?.is_active">
              {{ t('settings.accounts.disableWarning') }}
            </span>
            <span v-else>
              {{ t('settings.accounts.enableWarning') }}
            </span>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="toggleAccountStatus" :disabled="isToggling === accountToToggle?.account_id">
            {{ isToggling === accountToToggle?.account_id ? t('common.processing') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </Dialog>

  <!-- 编辑账户对话框（放在主对话框外面避免嵌套） -->
  <EditAccountDialog v-if="accountToEdit" :account="accountToEdit" @account-updated="handleAccountUpdated"
    :standalone="true" :open="editDialogOpen"
    @update:open="(val) => { editDialogOpen = val; if (!val) accountToEdit = null }" />
</template>